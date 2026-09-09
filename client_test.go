package agentpass_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	agentpass "github.com/jianqi-zhao/agentpass-go"
)

func newTestClient(t *testing.T, handler http.HandlerFunc, options ...agentpass.Option) *agentpass.Client {
	t.Helper()
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder.Result(), nil
	})}
	allOptions := []agentpass.Option{
		agentpass.WithBaseURL("https://agentpass.example/agentpass/"),
		agentpass.WithHTTPClient(httpClient),
	}
	allOptions = append(allOptions, options...)
	client, err := agentpass.NewClient(allOptions...)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestAuthorizationURL(t *testing.T) {
	client, err := agentpass.NewClient(agentpass.WithBaseURL("https://example.test/agentpass/"))
	if err != nil {
		t.Fatal(err)
	}
	authorizationURL, err := client.OAuth.AuthorizationURL(agentpass.AuthorizationURLParams{
		ClientID:     "client_123",
		RedirectURI:  "https://app.example/callback",
		Capabilities: []string{"ai.inference", "identity.basic"},
		Models:       []string{"openai:gpt-5.6-sol"},
		DefaultModel: "openai:gpt-5.6-sol",
		MonthlyLimit: 10,
		WeeklyLimit:  2.5,
		State:        "signed-state",
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(authorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/agentpass/oauth/authorize" {
		t.Fatalf("unexpected authorization path: %s", parsed.Path)
	}
	if parsed.Query().Get("scope") != "ai.inference identity.basic" || parsed.Query().Get("state") != "signed-state" {
		t.Fatalf("unexpected authorization query: %s", parsed.RawQuery)
	}
	if parsed.Query().Get("model") != "openai:gpt-5.6-sol" || parsed.Query().Get("default_model") != "openai:gpt-5.6-sol" {
		t.Fatalf("unexpected model query: %s", parsed.RawQuery)
	}
	if parsed.Query().Has("weekly_limit") || parsed.Query().Has("monthly_limit") {
		t.Fatalf("retired app limits must not be sent: %s", parsed.RawQuery)
	}
}

func TestCurrentUsage(t *testing.T) {
	client := newTestClient(t, func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/agentpass/v1/usage" || request.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer ap_test" {
			t.Errorf("unexpected authorization: %s", request.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(response, `{
			"object":"agentpass.usage","scope":"user","grant_id":"grant_123",
			"app":{"client_id":"client_123","name":"Enjoy Life"},
			"week":{"starts_at":"2026-08-03T00:00:00Z","ends_at":"2026-08-10T00:00:00Z","resets_at":"2026-08-10T00:00:00Z","limit":500,"used":170,"reserved":20,"remaining":310,"remaining_percent":62,"can_start_new_work":true},
			"updated_at":"2026-08-04T12:00:00Z"
		}`)
	}, agentpass.WithAccessToken("ap_test"))

	usage, err := client.Usage.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if usage.App.Name != "Enjoy Life" || usage.Week.Limit != 500 || usage.Week.RemainingPercent != 62 || !usage.Week.CanStartNewWork || usage.Week.Remaining != 310 {
		t.Fatalf("unexpected usage summary: %+v", usage)
	}
}

func TestUserInfo(t *testing.T) {
	client := newTestClient(t, func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/agentpass/oauth/userinfo" || request.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer ap_identity" {
			t.Errorf("unexpected authorization: %s", request.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(response, `{"sub":"user_123","name":"Ada","email":"ada@example.com","email_verified":true}`)
	}, agentpass.WithAccessToken("ap_identity"))

	profile, err := client.Identity.UserInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if profile.Subject != "user_123" || profile.Email != "ada@example.com" || !profile.EmailVerified {
		t.Fatalf("unexpected identity: %+v", profile)
	}
}

func TestCreateUsageEmbedForWKWebView(t *testing.T) {
	client := newTestClient(t, func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/agentpass/v1/embed/sessions" || request.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer ap_test" {
			t.Errorf("unexpected authorization: %s", request.Header.Get("Authorization"))
		}
		payload := map[string]string{}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["theme"] != "dark" {
			t.Fatalf("unexpected embed payload: %v", payload)
		}
		_, _ = io.WriteString(response, `{
			"object":"agentpass.embed_session",
			"component":"usage",
			"embed_url":"https://agentpass.example/agentpass/embed/usage?theme=dark#code=embed_code_one_time_value",
			"expires_at":"`+time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)+`"
		}`)
	}, agentpass.WithAccessToken("ap_test"))

	session, err := client.Embed.CreateUsageView(
		context.Background(),
		agentpass.EmbedThemeDark,
	)
	if err != nil {
		t.Fatal(err)
	}
	if session.Component != "usage" || !strings.Contains(session.EmbedURL, "#code=embed_code_") {
		t.Fatalf("unexpected embed session: %+v", session)
	}
}

func TestExchangeAuthorizationCode(t *testing.T) {
	client := newTestClient(t, func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/agentpass/oauth/token" {
			t.Errorf("unexpected path: %s", request.URL.Path)
		}
		if request.Header.Get("User-Agent") != "agentpass-go/"+agentpass.Version {
			t.Errorf("unexpected user agent: %s", request.Header.Get("User-Agent"))
		}
		if err := request.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if request.Form.Get("client_secret") != "server-secret" || request.Form.Get("code") != "code_123" {
			t.Errorf("unexpected form: %v", request.Form)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, `{"access_token":"ap_token","refresh_token":"refresh_token","token_type":"Bearer","expires_in":3600,"grant_id":"grant_123"}`)
	})

	token, err := client.OAuth.ExchangeAuthorizationCode(context.Background(), agentpass.ExchangeCodeParams{
		Code:         "code_123",
		ClientID:     "client_123",
		ClientSecret: "server-secret",
		RedirectURI:  "https://app.example/agentpass/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "ap_token" || token.RefreshToken != "refresh_token" || token.GrantID != "grant_123" {
		t.Fatalf("unexpected token: %+v", token)
	}
}

func TestRefreshAccessToken(t *testing.T) {
	client := newTestClient(t, func(response http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if request.Form.Get("grant_type") != "refresh_token" || request.Form.Get("refresh_token") != "refresh_old" {
			t.Fatalf("unexpected refresh form: %v", request.Form)
		}
		if request.Header.Get("Idempotency-Key") != "refresh-attempt-123" {
			t.Fatalf("unexpected refresh idempotency key: %q", request.Header.Get("Idempotency-Key"))
		}
		_, _ = io.WriteString(response, `{"access_token":"ap_new","refresh_token":"refresh_new","token_type":"Bearer","expires_in":3600,"grant_id":"grant_123"}`)
	})
	token, err := client.OAuth.RefreshAccessToken(context.Background(), agentpass.RefreshTokenParams{
		RefreshToken:   "refresh_old",
		ClientID:       "client_123",
		ClientSecret:   "server-secret",
		IdempotencyKey: "refresh-attempt-123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "ap_new" || token.RefreshToken != "refresh_new" {
		t.Fatalf("unexpected refreshed token: %+v", token)
	}
}

func TestRefreshAccessTokenRequiresDurableIdempotencyKey(t *testing.T) {
	for _, key := range []string{"", " ", " leading", "trailing ", "bad\nkey", "bad\x7fkey", strings.Repeat("a", 201)} {
		t.Run(fmt.Sprintf("key_%q", key), func(t *testing.T) {
			client := newTestClient(t, func(response http.ResponseWriter, request *http.Request) {
				t.Fatal("invalid refresh key must fail before any HTTP request")
			})
			_, err := client.OAuth.RefreshAccessToken(context.Background(), agentpass.RefreshTokenParams{
				RefreshToken: "refresh_old", ClientID: "client_123", ClientSecret: "server-secret",
				IdempotencyKey: key,
			})
			if err == nil || !strings.Contains(err.Error(), "idempotency key") {
				t.Fatalf("expected local key validation, got %v", err)
			}
			var apiError *agentpass.APIError
			if errors.As(err, &apiError) {
				t.Fatal("local validation must not pretend the server rejected a token")
			}
		})
	}
}

func TestProviderBaseURLs(t *testing.T) {
	openAI, err := agentpass.OpenAIBaseURL("https://example.test/agentpass/")
	if err != nil {
		t.Fatal(err)
	}
	anthropic, err := agentpass.AnthropicBaseURL("https://example.test/agentpass/")
	if err != nil {
		t.Fatal(err)
	}
	if openAI != "https://example.test/agentpass/openai/v1" || anthropic != "https://example.test/agentpass/anthropic" {
		t.Fatalf("unexpected provider base URLs: %s %s", openAI, anthropic)
	}
}

func TestNewIdempotencyKey(t *testing.T) {
	keys := make(map[string]bool)
	for range 100 {
		key, err := agentpass.NewIdempotencyKey()
		if err != nil {
			t.Fatal(err)
		}
		if len(key) != 36 || key[14] != '4' || !strings.ContainsRune("89ab", rune(key[19])) || keys[key] {
			t.Fatalf("invalid or repeated idempotency key: %q", key)
		}
		keys[key] = true
	}
}

func TestAPIErrorIsTyped(t *testing.T) {
	client := newTestClient(t, func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(response, `{"error":{"code":"grant_revoked","message":"The grant was revoked.","details":{"grant_id":"grant_123"}}}`)
	}, agentpass.WithAccessToken("ap_revoked"))

	_, err := client.Usage.Current(context.Background())
	var apiError *agentpass.APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiError.StatusCode != http.StatusUnauthorized || apiError.Code != "grant_revoked" || !strings.Contains(string(apiError.Details), "grant_123") {
		t.Fatalf("unexpected API error: %+v", apiError)
	}
}
