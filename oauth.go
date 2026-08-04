package agentpass

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var modelKeyPattern = regexp.MustCompile(`^(openai|anthropic):[A-Za-z0-9._:/-]+$`)

// OAuthService exposes the confidential backend portion of AgentPass OAuth.
type OAuthService struct {
	client *Client
}

// AuthorizationURLParams describes the user consent request.
type AuthorizationURLParams struct {
	ClientID     string
	RedirectURI  string
	Capabilities []string
	Models       []string
	DefaultModel string
	MonthlyLimit int
	State        string
}

// AuthorizationURL creates the URL to which a backend should redirect a user.
func (service *OAuthService) AuthorizationURL(params AuthorizationURLParams) (string, error) {
	if strings.TrimSpace(params.ClientID) == "" || strings.TrimSpace(params.ClientID) != params.ClientID {
		return "", errors.New("agentpass: OAuth client ID is required")
	}
	if strings.TrimSpace(params.RedirectURI) == "" {
		return "", errors.New("agentpass: OAuth redirect URI is required")
	}
	if err := validateRedirectURI(params.RedirectURI); err != nil {
		return "", err
	}
	if len(params.Capabilities) == 0 {
		return "", errors.New("agentpass: at least one capability is required")
	}
	seenCapabilities := make(map[string]struct{}, len(params.Capabilities))
	for _, capability := range params.Capabilities {
		if capability == "" || strings.TrimSpace(capability) != capability || strings.ContainsAny(capability, " \t\r\n") {
			return "", errors.New("agentpass: capabilities must be non-empty values without whitespace")
		}
		if _, exists := seenCapabilities[capability]; exists {
			return "", errors.New("agentpass: capabilities cannot contain duplicates")
		}
		seenCapabilities[capability] = struct{}{}
	}
	seenModels := make(map[string]struct{}, len(params.Models))
	for _, model := range params.Models {
		if !modelKeyPattern.MatchString(model) {
			return "", errors.New("agentpass: models must be provider-qualified openai:model or anthropic:model values")
		}
		if _, exists := seenModels[model]; exists {
			return "", errors.New("agentpass: models cannot contain duplicates")
		}
		seenModels[model] = struct{}{}
	}
	if params.DefaultModel != "" {
		if _, allowed := seenModels[params.DefaultModel]; !allowed {
			return "", errors.New("agentpass: default model must be one of the requested models")
		}
	}
	if params.MonthlyLimit <= 0 {
		return "", errors.New("agentpass: monthly limit must be positive")
	}
	if params.State == "" {
		return "", errors.New("agentpass: OAuth state is required")
	}
	if strings.TrimSpace(params.State) != params.State || !validHeaderValue(params.State) {
		return "", errors.New("agentpass: OAuth state contains invalid characters")
	}
	if len(params.State) > 1024 {
		return "", errors.New("agentpass: OAuth state cannot exceed 1024 characters")
	}

	query := url.Values{
		"response_type": {"code"},
		"client_id":     {params.ClientID},
		"redirect_uri":  {params.RedirectURI},
		"scope":         {strings.Join(params.Capabilities, " ")},
		"monthly_limit": {strconv.Itoa(params.MonthlyLimit)},
	}
	query.Set("state", params.State)
	for _, model := range params.Models {
		query.Add("model", model)
	}
	if params.DefaultModel != "" {
		query.Set("default_model", params.DefaultModel)
	}
	return service.client.endpoint("/oauth/authorize") + "?" + query.Encode(), nil
}

func validateRedirectURI(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("agentpass: OAuth redirect URI must be an absolute URL")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return errors.New("agentpass: OAuth redirect URI cannot contain credentials or a fragment")
	}
	loopbackRedirect := parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1"
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopbackRedirect) {
		return errors.New("agentpass: OAuth redirect URI must use HTTPS except on localhost")
	}
	return nil
}

// ExchangeCodeParams contains confidential authorization-code exchange input.
// ClientSecret must only be loaded by a trusted backend.
type ExchangeCodeParams struct {
	Code         string
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

// Token is an opaque user-scoped AgentPass access token.
type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	GrantID      string `json:"grant_id"`
}

// RefreshTokenParams contains confidential refresh-token exchange input.
type RefreshTokenParams struct {
	RefreshToken string
	ClientID     string
	ClientSecret string
}

// ExchangeAuthorizationCode exchanges a one-time code from the callback for an
// access token. Persist the token in application-owned encrypted storage.
func (service *OAuthService) ExchangeAuthorizationCode(
	ctx context.Context,
	params ExchangeCodeParams,
) (*Token, error) {
	if strings.TrimSpace(params.Code) == "" || strings.TrimSpace(params.Code) != params.Code ||
		strings.TrimSpace(params.ClientID) == "" || strings.TrimSpace(params.ClientID) != params.ClientID ||
		strings.TrimSpace(params.ClientSecret) == "" || strings.TrimSpace(params.ClientSecret) != params.ClientSecret ||
		strings.TrimSpace(params.RedirectURI) == "" || strings.TrimSpace(params.RedirectURI) != params.RedirectURI {
		return nil, errors.New("agentpass: code, client ID, client secret, and redirect URI are required")
	}
	if err := validateRedirectURI(params.RedirectURI); err != nil {
		return nil, err
	}

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {params.Code},
		"client_id":     {params.ClientID},
		"client_secret": {params.ClientSecret},
		"redirect_uri":  {params.RedirectURI},
	}
	request, err := service.client.newRequest(
		ctx,
		http.MethodPost,
		"/oauth/token",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	token := &Token{}
	if err := service.client.do(request, token); err != nil {
		return nil, err
	}
	if err := validateToken(token); err != nil {
		return nil, err
	}
	return token, nil
}

// RefreshAccessToken rotates a refresh token and returns a new access/refresh
// token pair. Persist the replacement before discarding the previous value.
func (service *OAuthService) RefreshAccessToken(
	ctx context.Context,
	params RefreshTokenParams,
) (*Token, error) {
	if strings.TrimSpace(params.RefreshToken) == "" ||
		strings.TrimSpace(params.RefreshToken) != params.RefreshToken ||
		strings.TrimSpace(params.ClientID) == "" ||
		strings.TrimSpace(params.ClientID) != params.ClientID ||
		strings.TrimSpace(params.ClientSecret) == "" ||
		strings.TrimSpace(params.ClientSecret) != params.ClientSecret {
		return nil, errors.New("agentpass: refresh token, client ID, and client secret are required")
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {params.RefreshToken},
		"client_id":     {params.ClientID},
		"client_secret": {params.ClientSecret},
	}
	request, err := service.client.newRequest(
		ctx,
		http.MethodPost,
		"/oauth/token",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	token := &Token{}
	if err := service.client.do(request, token); err != nil {
		return nil, err
	}
	if err := validateToken(token); err != nil {
		return nil, err
	}
	return token, nil
}

func validateToken(token *Token) error {
	if strings.TrimSpace(token.AccessToken) == "" ||
		strings.TrimSpace(token.AccessToken) != token.AccessToken ||
		!validHeaderValue(token.AccessToken) ||
		strings.TrimSpace(token.RefreshToken) == "" ||
		strings.TrimSpace(token.RefreshToken) != token.RefreshToken ||
		!validHeaderValue(token.RefreshToken) ||
		!strings.EqualFold(token.TokenType, "Bearer") ||
		token.ExpiresIn <= 0 ||
		strings.TrimSpace(token.GrantID) == "" ||
		strings.TrimSpace(token.GrantID) != token.GrantID ||
		!validHeaderValue(token.GrantID) {
		return fmt.Errorf("%w: malformed OAuth token payload", ErrInvalidResponse)
	}
	return nil
}
