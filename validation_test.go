package agentpass_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	agentpass "github.com/jianqi-zhao/agentpass-go"
)

const validResponseJSON = `{
	"id":"request_123","object":"agentpass.response","model":"fast-model",
	"output_text":"done","usage":{"inputTokens":12,"cachedInputTokens":3,"outputTokens":4,"reasoningTokens":2,"totalTokens":16},
	"agentpass":{"receipt":{"request_id":"request_123","app":"Draftly","capability":"text.fast","credits_used":3,"remaining_credits":997,"settled_at":"2026-08-03T00:00:00Z"}}
}`

func TestBaseURLTransportRules(t *testing.T) {
	t.Parallel()

	for _, baseURL := range []string{
		"http://api.example.test/agentpass",
		"https://user:password@api.example.test/agentpass",
	} {
		if _, err := agentpass.NewClient(agentpass.WithBaseURL(baseURL)); err == nil {
			t.Fatalf("expected %q to be rejected", baseURL)
		}
	}

	for _, baseURL := range []string{
		"https://api.example.test/agentpass",
		"http://localhost:8080/agentpass",
		"http://127.0.0.1:8080/agentpass",
		"http://[::1]:8080/agentpass",
	} {
		if _, err := agentpass.NewClient(agentpass.WithBaseURL(baseURL)); err != nil {
			t.Fatalf("expected %q to be accepted: %v", baseURL, err)
		}
	}
}

func TestRejectsUnsafeCredentialHeaders(t *testing.T) {
	t.Parallel()

	for _, accessToken := range []string{"", " token", "token ", "token\nforged", "token\tforged"} {
		if _, err := agentpass.NewClient(agentpass.WithAccessToken(accessToken)); err == nil {
			t.Fatalf("expected access token %q to be rejected", accessToken)
		}
	}

	for _, userAgent := range []string{"", "agentpass\nforged", "agentpass\tforged"} {
		if _, err := agentpass.NewClient(agentpass.WithUserAgent(userAgent)); err == nil {
			t.Fatalf("expected user agent %q to be rejected", userAgent)
		}
	}
}

func TestNilTokenSourceFunctionReturnsError(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("request should not be sent")
	}, agentpass.WithTokenSource(agentpass.TokenSourceFunc(nil)))
	_, err := client.Responses.Create(context.Background(), agentpass.CreateResponseParams{
		Capability: "text.fast",
		Input:      "Hello",
	})
	if err == nil || !strings.Contains(err.Error(), "token source function is nil") {
		t.Fatalf("expected nil token source error, got %v", err)
	}
}

func TestDynamicTokenRejectsSurroundingWhitespace(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(http.ResponseWriter, *http.Request) {
		t.Fatal("request should not be sent")
	}, agentpass.WithTokenSource(agentpass.TokenSourceFunc(func(context.Context) (string, error) {
		return " token", nil
	})))
	_, err := client.Responses.Create(context.Background(), agentpass.CreateResponseParams{
		Capability: "text.fast",
		Input:      "Hello",
	})
	if err == nil || !strings.Contains(err.Error(), "surrounding whitespace") {
		t.Fatalf("expected unsafe token error, got %v", err)
	}
}

func TestClientDoesNotFollowUnexpectedRedirects(t *testing.T) {
	t.Parallel()

	requests := 0
	client := newTestClient(t, func(response http.ResponseWriter, request *http.Request) {
		requests++
		if request.URL.Path == "/agentpass/v1/responses" {
			response.Header().Set("Location", "https://redirect.example/collect")
			response.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		t.Fatalf("followed unexpected redirect to %s", request.URL)
	}, agentpass.WithAccessToken("token"))
	_, err := client.Responses.Create(context.Background(), agentpass.CreateResponseParams{
		Capability: "text.fast",
		Input:      "Hello",
	})
	var apiError *agentpass.APIError
	if !errors.As(err, &apiError) || apiError.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("expected typed redirect response, got %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected one request, got %d", requests)
	}
}

func TestMalformedJSONIsInvalidResponse(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(response http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(response, `{"id":`)
	}, agentpass.WithAccessToken("token"))
	_, err := client.Responses.Create(context.Background(), agentpass.CreateResponseParams{
		Capability: "text.fast",
		Input:      "Hello",
	})
	if !errors.Is(err, agentpass.ErrInvalidResponse) {
		t.Fatalf("expected ErrInvalidResponse, got %v", err)
	}
}

func TestOversizedPayloadIsInvalidResponse(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(response http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(response, strings.Repeat("x", (4<<20)+1))
	}, agentpass.WithAccessToken("token"))
	_, err := client.Responses.Create(context.Background(), agentpass.CreateResponseParams{
		Capability: "text.fast",
		Input:      "Hello",
	})
	if !errors.Is(err, agentpass.ErrInvalidResponse) {
		t.Fatalf("expected ErrInvalidResponse, got %v", err)
	}
}

func TestAuthorizationURLRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	client, err := agentpass.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	valid := agentpass.AuthorizationURLParams{
		ClientID:     "client_123",
		RedirectURI:  "https://app.example/callback",
		Capabilities: []string{"text.fast"},
		MonthlyLimit: 100,
		State:        "state",
	}
	tests := []struct {
		name   string
		mutate func(*agentpass.AuthorizationURLParams)
	}{
		{"relative redirect", func(params *agentpass.AuthorizationURLParams) { params.RedirectURI = "/callback" }},
		{"insecure remote redirect", func(params *agentpass.AuthorizationURLParams) { params.RedirectURI = "http://app.example/callback" }},
		{"redirect credentials", func(params *agentpass.AuthorizationURLParams) {
			params.RedirectURI = "https://user@app.example/callback"
		}},
		{"redirect fragment", func(params *agentpass.AuthorizationURLParams) {
			params.RedirectURI = "https://app.example/callback#fragment"
		}},
		{"duplicate capability", func(params *agentpass.AuthorizationURLParams) {
			params.Capabilities = []string{"text.fast", "text.fast"}
		}},
		{"capability whitespace", func(params *agentpass.AuthorizationURLParams) { params.Capabilities = []string{"text.fast smart"} }},
		{"unqualified model", func(params *agentpass.AuthorizationURLParams) { params.Models = []string{"gpt-5.6-sol"} }},
		{"unknown model provider", func(params *agentpass.AuthorizationURLParams) { params.Models = []string{"other:model"} }},
		{"default outside models", func(params *agentpass.AuthorizationURLParams) {
			params.Models = []string{"openai:gpt-5.6-sol"}
			params.DefaultModel = "anthropic:claude"
		}},
		{"missing state", func(params *agentpass.AuthorizationURLParams) { params.State = "" }},
		{"oversized state", func(params *agentpass.AuthorizationURLParams) { params.State = strings.Repeat("a", 1025) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := valid
			test.mutate(&params)
			if _, err := client.OAuth.AuthorizationURL(params); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestExchangeRejectsMalformedSuccessPayload(t *testing.T) {
	t.Parallel()

	for _, payload := range []string{
		`{"token_type":"Bearer","expires_in":3600,"grant_id":"grant_123"}`,
		`{"access_token":"token","token_type":"Basic","expires_in":3600,"grant_id":"grant_123"}`,
		`{"access_token":"token","token_type":"Bearer","expires_in":0,"grant_id":"grant_123"}`,
		`{"access_token":"token","token_type":"Bearer","expires_in":3600}`,
		`{"access_token":"token\nforged","token_type":"Bearer","expires_in":3600,"grant_id":"grant_123"}`,
	} {
		payload := payload
		t.Run(payload, func(t *testing.T) {
			client := newTestClient(t, func(response http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(response, payload)
			})
			_, err := client.OAuth.ExchangeAuthorizationCode(context.Background(), agentpass.ExchangeCodeParams{
				Code:         "code_123",
				ClientID:     "client_123",
				ClientSecret: "secret_123",
				RedirectURI:  "https://app.example/callback",
			})
			if !errors.Is(err, agentpass.ErrInvalidResponse) {
				t.Fatalf("expected ErrInvalidResponse, got %v", err)
			}
		})
	}
}

func TestCreateRejectsInvalidRequestLimitsAndKeys(t *testing.T) {
	t.Parallel()

	client, err := agentpass.NewClient(agentpass.WithAccessToken("token"))
	if err != nil {
		t.Fatal(err)
	}
	for _, params := range []agentpass.CreateResponseParams{
		{Capability: " text.fast", Input: "Hello"},
		{Capability: "text.fast", Input: "Hello", MaxCredits: -1},
		{Capability: "text.fast", Input: "Hello", IdempotencyKey: " key"},
		{Capability: "text.fast", Input: "Hello", IdempotencyKey: "key\tforged"},
	} {
		if _, err := client.Responses.Create(context.Background(), params); err == nil {
			t.Fatalf("expected validation error for %+v", params)
		}
	}
}

func TestCreateRejectsMalformedSuccessPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		payload string
	}{
		{"missing identity", strings.Replace(validResponseJSON, `"id":"request_123"`, `"id":""`, 1)},
		{"wrong object", strings.Replace(validResponseJSON, `"object":"agentpass.response"`, `"object":"response"`, 1)},
		{"mismatched receipt", strings.Replace(validResponseJSON, `"request_id":"request_123"`, `"request_id":"request_other"`, 1)},
		{"wrong capability", strings.Replace(validResponseJSON, `"capability":"text.fast"`, `"capability":"text.smart"`, 1)},
		{"negative usage", strings.Replace(validResponseJSON, `"totalTokens":16`, `"totalTokens":-1`, 1)},
		{"missing usage", strings.Replace(validResponseJSON, `"inputTokens":12,"cachedInputTokens":3,"outputTokens":4,"reasoningTokens":2,"totalTokens":16`, `"inputTokens":0,"outputTokens":0,"totalTokens":0`, 1)},
		{"excess reasoning usage", strings.Replace(validResponseJSON, `"reasoningTokens":2`, `"reasoningTokens":5`, 1)},
		{"inconsistent usage", strings.Replace(validResponseJSON, `"totalTokens":16`, `"totalTokens":2`, 1)},
		{"invalid settled timestamp", strings.Replace(validResponseJSON, `"2026-08-03T00:00:00Z"`, `"not-a-date"`, 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t, func(response http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(response, test.payload)
			}, agentpass.WithAccessToken("token"))
			_, err := client.Responses.Create(context.Background(), agentpass.CreateResponseParams{
				Capability: "text.fast",
				Input:      "Hello",
			})
			if !errors.Is(err, agentpass.ErrInvalidResponse) {
				t.Fatalf("expected ErrInvalidResponse, got %v", err)
			}
		})
	}
}
