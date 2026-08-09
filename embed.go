package agentpass

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// EmbedService creates read-only hosted views for a connected user/app grant.
// Embed URLs are safe to return to the app frontend; OAuth tokens are not.
type EmbedService struct {
	client *Client
}

// EmbedTheme controls the hosted usage view appearance.
type EmbedTheme string

const (
	EmbedThemeSystem EmbedTheme = "system"
	EmbedThemeLight  EmbedTheme = "light"
	EmbedThemeDark   EmbedTheme = "dark"
)

// UsageEmbedSession contains a one-time URL for the hosted usage view. The URL
// must be opened before ExpiresAt and should not be persisted or logged.
type UsageEmbedSession struct {
	Object    string `json:"object"`
	Component string `json:"component"`
	EmbedURL  string `json:"embed_url"`
	ExpiresAt string `json:"expires_at"`
}

// CreateUsageView creates a one-time URL suitable for a WKWebView. Call this
// from the app backend with the connected user's AgentPass access token.
func (service *EmbedService) CreateUsageView(
	ctx context.Context,
	theme EmbedTheme,
) (*UsageEmbedSession, error) {
	if theme == "" {
		theme = EmbedThemeSystem
	}
	if theme != EmbedThemeSystem && theme != EmbedThemeLight && theme != EmbedThemeDark {
		return nil, errors.New("agentpass: embed theme must be system, light, or dark")
	}
	request, err := service.client.newJSONRequest(
		ctx,
		http.MethodPost,
		"/v1/embed/sessions",
		struct {
			Theme EmbedTheme `json:"theme"`
		}{Theme: theme},
	)
	if err != nil {
		return nil, err
	}
	if err := service.client.authorizeRequest(ctx, request); err != nil {
		return nil, err
	}

	session := &UsageEmbedSession{}
	if err := service.client.do(request, session); err != nil {
		return nil, err
	}
	if err := service.validateUsageSession(session); err != nil {
		return nil, err
	}
	return session, nil
}

func (service *EmbedService) validateUsageSession(session *UsageEmbedSession) error {
	if session.Object != "agentpass.embed_session" || session.Component != "usage" {
		return fmt.Errorf("%w: malformed embed identity", ErrInvalidResponse)
	}
	embedURL, err := url.Parse(session.EmbedURL)
	if err != nil || embedURL.Scheme == "" || embedURL.Host == "" {
		return fmt.Errorf("%w: malformed embed URL", ErrInvalidResponse)
	}
	expectedURL, _ := url.Parse(service.client.endpoint("/embed/usage"))
	if embedURL.Scheme != expectedURL.Scheme ||
		embedURL.Host != expectedURL.Host ||
		embedURL.Path != expectedURL.Path ||
		embedURL.User != nil ||
		!strings.HasPrefix(embedURL.Fragment, "code=embed_code_") {
		return fmt.Errorf("%w: malformed embed URL", ErrInvalidResponse)
	}
	if theme := embedURL.Query().Get("theme"); theme != "" && theme != "light" && theme != "dark" {
		return fmt.Errorf("%w: malformed embed theme", ErrInvalidResponse)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, session.ExpiresAt)
	if err != nil || !expiresAt.After(time.Now()) {
		return fmt.Errorf("%w: malformed embed expiration", ErrInvalidResponse)
	}
	return nil
}
