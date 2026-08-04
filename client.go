// Package agentpass provides a backend client for the AgentPass OAuth and AI APIs.
package agentpass

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the public AgentPass API endpoint.
	DefaultBaseURL = "https://www.prsvrc.com/agentpass"
	// Version is the SDK version sent in the default User-Agent header.
	Version          = "0.5.0"
	maxResponseBytes = 4 << 20
)

// TokenSource returns an AgentPass access token for an outgoing request.
// Implementations may load or refresh tokens from application-owned storage
// and must be safe for concurrent use when shared by a Client.
type TokenSource interface {
	Token(context.Context) (string, error)
}

// OpenAIBaseURL returns the base URL for standard OpenAI SDK clients.
func OpenAIBaseURL(agentPassBaseURL string) (string, error) {
	baseURL, err := normalizeBaseURL(agentPassBaseURL)
	if err != nil {
		return "", err
	}
	return baseURL + "/openai/v1", nil
}

// AnthropicBaseURL returns the base URL for standard Anthropic SDK clients.
func AnthropicBaseURL(agentPassBaseURL string) (string, error) {
	baseURL, err := normalizeBaseURL(agentPassBaseURL)
	if err != nil {
		return "", err
	}
	return baseURL + "/anthropic", nil
}

// TokenSourceFunc adapts a function into a TokenSource.
type TokenSourceFunc func(context.Context) (string, error)

// Token implements TokenSource.
func (f TokenSourceFunc) Token(ctx context.Context) (string, error) {
	if f == nil {
		return "", errors.New("agentpass: token source function is nil")
	}
	return f(ctx)
}

// Option configures a Client.
type Option func(*clientOptions) error

type clientOptions struct {
	baseURL     string
	httpClient  *http.Client
	tokenSource TokenSource
	userAgent   string
}

// WithBaseURL points the client at an AgentPass deployment. A trailing slash is
// accepted and removed.
func WithBaseURL(baseURL string) Option {
	return func(options *clientOptions) error {
		options.baseURL = baseURL
		return nil
	}
}

// WithHTTPClient supplies the HTTP client used for all requests.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(options *clientOptions) error {
		if httpClient == nil {
			return errors.New("agentpass: HTTP client cannot be nil")
		}
		options.httpClient = httpClient
		return nil
	}
}

// WithAccessToken configures a fixed user-scoped access token. Do not use a
// developer client secret here.
func WithAccessToken(accessToken string) Option {
	return func(options *clientOptions) error {
		if accessToken == "" {
			return errors.New("agentpass: access token cannot be empty")
		}
		if strings.TrimSpace(accessToken) != accessToken {
			return errors.New("agentpass: access token cannot contain surrounding whitespace")
		}
		if !validHeaderValue(accessToken) {
			return errors.New("agentpass: access token contains invalid characters")
		}
		options.tokenSource = TokenSourceFunc(func(context.Context) (string, error) {
			return accessToken, nil
		})
		return nil
	}
}

// WithTokenSource configures dynamic access-token lookup for each AI request.
func WithTokenSource(tokenSource TokenSource) Option {
	return func(options *clientOptions) error {
		if tokenSource == nil {
			return errors.New("agentpass: token source cannot be nil")
		}
		options.tokenSource = tokenSource
		return nil
	}
}

// WithUserAgent overrides the SDK User-Agent header.
func WithUserAgent(userAgent string) Option {
	return func(options *clientOptions) error {
		userAgent = strings.TrimSpace(userAgent)
		if userAgent == "" {
			return errors.New("agentpass: user agent cannot be empty")
		}
		if !validHeaderValue(userAgent) {
			return errors.New("agentpass: user agent contains invalid characters")
		}
		options.userAgent = userAgent
		return nil
	}
}

// Client is safe for concurrent use when its configured TokenSource and HTTP
// transport are also safe for concurrent use.
type Client struct {
	baseURL     string
	httpClient  *http.Client
	tokenSource TokenSource
	userAgent   string

	OAuth     *OAuthService
	Responses *ResponsesService
	Usage     *UsageService
}

// NewClient constructs an AgentPass client. The default endpoint is the live
// AgentPass deployment and the default request timeout is five minutes.
func NewClient(options ...Option) (*Client, error) {
	configuration := clientOptions{
		baseURL: DefaultBaseURL,
		httpClient: &http.Client{
			Timeout:       5 * time.Minute,
			CheckRedirect: rejectRedirect,
		},
		userAgent: "agentpass-go/" + Version,
	}
	for _, option := range options {
		if option == nil {
			return nil, errors.New("agentpass: client option cannot be nil")
		}
		if err := option(&configuration); err != nil {
			return nil, err
		}
	}

	baseURL, err := normalizeBaseURL(configuration.baseURL)
	if err != nil {
		return nil, err
	}
	httpClient := *configuration.httpClient
	if httpClient.CheckRedirect == nil {
		httpClient.CheckRedirect = rejectRedirect
	}
	client := &Client{
		baseURL:     baseURL,
		httpClient:  &httpClient,
		tokenSource: configuration.tokenSource,
		userAgent:   configuration.userAgent,
	}
	client.OAuth = &OAuthService{client: client}
	client.Responses = &ResponsesService{client: client}
	client.Usage = &UsageService{client: client}
	return client, nil
}

func rejectRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

func normalizeBaseURL(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("agentpass: base URL must be an absolute HTTP URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("agentpass: base URL scheme must be http or https")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("agentpass: base URL cannot contain user credentials")
	}
	if parsed.Scheme == "http" && !isLoopbackHost(parsed.Hostname()) {
		return "", fmt.Errorf("agentpass: HTTP base URLs are allowed only for localhost")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("agentpass: base URL cannot contain a query or fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func validHeaderValue(value string) bool {
	return !strings.ContainsFunc(value, func(character rune) bool {
		return character < 0x20 || character == 0x7f
	})
}

func (client *Client) endpoint(path string) string {
	return client.baseURL + "/" + strings.TrimLeft(path, "/")
}

func (client *Client) newRequest(
	ctx context.Context,
	method string,
	path string,
	body io.Reader,
) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, client.endpoint(path), body)
	if err != nil {
		return nil, fmt.Errorf("agentpass: create request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", client.userAgent)
	return request, nil
}

func (client *Client) newJSONRequest(
	ctx context.Context,
	method string,
	path string,
	input any,
) (*http.Request, error) {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(input); err != nil {
		return nil, fmt.Errorf("agentpass: encode request: %w", err)
	}
	request, err := client.newRequest(ctx, method, path, &body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	return request, nil
}

func (client *Client) authorizeRequest(ctx context.Context, request *http.Request) error {
	if client.tokenSource == nil {
		return errors.New("agentpass: access token is required; configure WithAccessToken or WithTokenSource")
	}
	accessToken, err := client.tokenSource.Token(ctx)
	if err != nil {
		return fmt.Errorf("agentpass: load access token: %w", err)
	}
	if accessToken == "" {
		return errors.New("agentpass: token source returned an empty access token")
	}
	if strings.TrimSpace(accessToken) != accessToken {
		return errors.New("agentpass: token source returned an access token with surrounding whitespace")
	}
	if !validHeaderValue(accessToken) {
		return errors.New("agentpass: token source returned invalid characters")
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	return nil
}

func (client *Client) do(request *http.Request, output any) error {
	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("agentpass: send request: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("agentpass: read response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return fmt.Errorf("%w: response exceeded 4 MiB", ErrInvalidResponse)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return decodeAPIError(response.StatusCode, body)
	}
	if output == nil || len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, output); err != nil {
		return fmt.Errorf("%w: decode JSON: %v", ErrInvalidResponse, err)
	}
	return nil
}
