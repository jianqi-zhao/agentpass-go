package agentpass

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// OAuthService exposes the confidential backend portion of AgentPass OAuth.
type OAuthService struct {
	client *Client
}

// AuthorizationURLParams describes the user consent request.
type AuthorizationURLParams struct {
	ClientID     string
	RedirectURI  string
	Capabilities []string
	MonthlyLimit int
	State        string
}

// AuthorizationURL creates the URL to which a backend should redirect a user.
func (service *OAuthService) AuthorizationURL(params AuthorizationURLParams) (string, error) {
	if strings.TrimSpace(params.ClientID) == "" {
		return "", errors.New("agentpass: OAuth client ID is required")
	}
	if strings.TrimSpace(params.RedirectURI) == "" {
		return "", errors.New("agentpass: OAuth redirect URI is required")
	}
	if len(params.Capabilities) == 0 {
		return "", errors.New("agentpass: at least one capability is required")
	}
	if params.MonthlyLimit <= 0 {
		return "", errors.New("agentpass: monthly limit must be positive")
	}

	query := url.Values{
		"response_type": {"code"},
		"client_id":     {params.ClientID},
		"redirect_uri":  {params.RedirectURI},
		"scope":         {strings.Join(params.Capabilities, " ")},
		"monthly_limit": {strconv.Itoa(params.MonthlyLimit)},
	}
	if params.State != "" {
		query.Set("state", params.State)
	}
	return service.client.endpoint("/oauth/authorize") + "?" + query.Encode(), nil
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
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	GrantID     string `json:"grant_id"`
}

// ExchangeAuthorizationCode exchanges a one-time code from the callback for an
// access token. Persist the token in application-owned encrypted storage.
func (service *OAuthService) ExchangeAuthorizationCode(
	ctx context.Context,
	params ExchangeCodeParams,
) (*Token, error) {
	if strings.TrimSpace(params.Code) == "" ||
		strings.TrimSpace(params.ClientID) == "" ||
		strings.TrimSpace(params.ClientSecret) == "" ||
		strings.TrimSpace(params.RedirectURI) == "" {
		return nil, errors.New("agentpass: code, client ID, client secret, and redirect URI are required")
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
	return token, nil
}
