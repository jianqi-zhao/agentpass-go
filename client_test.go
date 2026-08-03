package agentpass_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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
		Capabilities: []string{"text.fast", "text.smart"},
		MonthlyLimit: 250,
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
	if parsed.Query().Get("scope") != "text.fast text.smart" || parsed.Query().Get("state") != "signed-state" {
		t.Fatalf("unexpected authorization query: %s", parsed.RawQuery)
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
		_, _ = io.WriteString(response, `{"access_token":"ap_token","token_type":"Bearer","expires_in":3600,"grant_id":"grant_123"}`)
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
	if token.AccessToken != "ap_token" || token.GrantID != "grant_123" {
		t.Fatalf("unexpected token: %+v", token)
	}
}

func TestCreateResponseSendsAuthenticationAndIdempotency(t *testing.T) {
	client := newTestClient(t, func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/agentpass/v1/responses" {
			t.Errorf("unexpected path: %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer ap_test" {
			t.Errorf("unexpected authorization: %s", request.Header.Get("Authorization"))
		}
		if request.Header.Get("Idempotency-Key") != "job-42" {
			t.Errorf("unexpected idempotency key: %s", request.Header.Get("Idempotency-Key"))
		}
		payload := map[string]any{}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["capability"] != "text.fast" || payload["max_credits"] != float64(30) {
			t.Errorf("unexpected payload: %v", payload)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, `{
			"id":"request_123","object":"agentpass.response","model":"fast-model",
			"output_text":"done","usage":{"inputTokens":12,"cachedInputTokens":3,"outputTokens":4,"reasoningTokens":2,"totalTokens":16},
			"agentpass":{"receipt":{"request_id":"request_123","app":"Draftly","capability":"text.fast","credits_used":3,"remaining_credits":997,"settled_at":"2026-08-03T00:00:00.000Z"}}
		}`)
	}, agentpass.WithAccessToken("ap_test"))

	result, err := client.Responses.Create(context.Background(), agentpass.CreateResponseParams{
		Capability:     "text.fast",
		Input:          "Write a title",
		MaxCredits:     30,
		IdempotencyKey: "job-42",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.OutputText != "done" || result.AgentPass.Receipt.CreditsUsed != 3 || result.Usage.TotalTokens != 16 {
		t.Fatalf("unexpected response: %+v", result)
	}
}

func TestCreateResponseGeneratesIdempotencyKey(t *testing.T) {
	client := newTestClient(t, func(response http.ResponseWriter, request *http.Request) {
		if len(request.Header.Get("Idempotency-Key")) != 36 {
			t.Errorf("expected generated UUID, got %q", request.Header.Get("Idempotency-Key"))
		}
		_, _ = io.WriteString(response, `{
			"id":"request_generated","object":"agentpass.response","model":"fast-model",
			"output_text":"done","usage":{"inputTokens":1,"outputTokens":1,"totalTokens":2},
			"agentpass":{"receipt":{"request_id":"request_generated","app":"Draftly","capability":"text.fast","credits_used":1,"remaining_credits":99,"settled_at":"2026-08-03T00:00:00Z"}}
		}`)
	}, agentpass.WithAccessToken("ap_test"))
	if _, err := client.Responses.Create(context.Background(), agentpass.CreateResponseParams{
		Capability: "text.fast",
		Input:      "Hello",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAPIErrorIsTyped(t *testing.T) {
	client := newTestClient(t, func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(response, `{"error":{"code":"grant_revoked","message":"The grant was revoked.","details":{"grant_id":"grant_123"}}}`)
	}, agentpass.WithAccessToken("ap_revoked"))

	_, err := client.Responses.Create(context.Background(), agentpass.CreateResponseParams{
		Capability: "text.fast",
		Input:      "Hello",
	})
	var apiError *agentpass.APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiError.StatusCode != http.StatusUnauthorized || apiError.Code != "grant_revoked" || !strings.Contains(string(apiError.Details), "grant_123") {
		t.Fatalf("unexpected API error: %+v", apiError)
	}
}
