package agentpass_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	agentpass "github.com/jianqi-zhao/agentpass-go"
)

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
	_, err := client.Usage.Current(context.Background())
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
	_, err := client.Usage.Current(context.Background())
	if err == nil || !strings.Contains(err.Error(), "surrounding whitespace") {
		t.Fatalf("expected unsafe token error, got %v", err)
	}
}

func TestClientDoesNotFollowUnexpectedRedirects(t *testing.T) {
	t.Parallel()

	requests := 0
	client := newTestClient(t, func(response http.ResponseWriter, request *http.Request) {
		requests++
		if request.URL.Path == "/agentpass/v1/usage" {
			response.Header().Set("Location", "https://redirect.example/collect")
			response.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		t.Fatalf("followed unexpected redirect to %s", request.URL)
	}, agentpass.WithAccessToken("token"))
	_, err := client.Usage.Current(context.Background())
	var apiError *agentpass.APIError
	if !errors.As(err, &apiError) || apiError.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("expected typed redirect response, got %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected one request, got %d", requests)
	}
}

func TestSuppliedHTTPClientCannotEnableCredentialRedirects(t *testing.T) {
	t.Parallel()
	requests := 0
	redirectCallbacks := 0
	httpClient := &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests++
			return &http.Response{
				StatusCode: http.StatusTemporaryRedirect,
				Header: http.Header{
					"Location": {"https://redirect.example/collect"},
				},
				Body:    io.NopCloser(strings.NewReader("")),
				Request: request,
			}, nil
		}),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			redirectCallbacks++
			return nil
		},
	}
	client, err := agentpass.NewClient(
		agentpass.WithBaseURL("https://agentpass.example/agentpass"),
		agentpass.WithHTTPClient(httpClient),
		agentpass.WithAccessToken("token"),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Usage.Current(context.Background())
	var apiError *agentpass.APIError
	if !errors.As(err, &apiError) || apiError.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("expected typed redirect response, got %v", err)
	}
	if requests != 1 || redirectCallbacks != 0 {
		t.Fatalf("redirect policy was weakened: requests=%d callbacks=%d", requests, redirectCallbacks)
	}
}

func TestProviderHTTPClientCannotForwardProviderCredentialsOnRedirect(t *testing.T) {
	t.Parallel()
	requests := 0
	redirectCallbacks := 0
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{
			StatusCode: http.StatusTemporaryRedirect,
			Header: http.Header{
				"Location": {"https://redirect.example/collect"},
			},
			Body:    io.NopCloser(strings.NewReader("")),
			Request: request,
		}, nil
	})
	original := &http.Client{
		Transport: transport,
		Timeout:   17 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			redirectCallbacks++
			return nil
		},
	}
	protected := agentpass.ProviderHTTPClient(original)
	if protected == original || protected.Timeout != original.Timeout {
		t.Fatal("provider client was not cloned with its transport and timeout")
	}
	response, err := protected.Get("https://agentpass.example/openai/v1/responses")
	if err != nil {
		t.Fatalf("get redirect response: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusTemporaryRedirect || requests != 1 ||
		redirectCallbacks != 0 {
		t.Fatalf(
			"redirect policy was weakened: status=%d requests=%d callbacks=%d",
			response.StatusCode,
			requests,
			redirectCallbacks,
		)
	}
	if original.CheckRedirect == nil {
		t.Fatal("ProviderHTTPClient mutated the caller's HTTP client")
	}
}

func TestMalformedJSONIsInvalidResponse(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(response http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(response, `{"id":`)
	}, agentpass.WithAccessToken("token"))
	_, err := client.Usage.Current(context.Background())
	if !errors.Is(err, agentpass.ErrInvalidResponse) {
		t.Fatalf("expected ErrInvalidResponse, got %v", err)
	}
}

func TestOversizedPayloadIsInvalidResponse(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(response http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(response, strings.Repeat("x", (4<<20)+1))
	}, agentpass.WithAccessToken("token"))
	_, err := client.Usage.Current(context.Background())
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
		Capabilities: []string{"ai.inference"},
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
			params.Capabilities = []string{"ai.inference", "ai.inference"}
		}},
		{"capability whitespace", func(params *agentpass.AuthorizationURLParams) { params.Capabilities = []string{"ai.inference smart"} }},
		{"unqualified model", func(params *agentpass.AuthorizationURLParams) { params.Models = []string{"gpt-5.6-sol"} }},
		{"unknown model provider", func(params *agentpass.AuthorizationURLParams) { params.Models = []string{"other:model"} }},
		{"default outside models", func(params *agentpass.AuthorizationURLParams) {
			params.Models = []string{"openai:gpt-5.6-sol"}
			params.DefaultModel = "anthropic:claude"
		}},
		{"missing state", func(params *agentpass.AuthorizationURLParams) { params.State = "" }},
		{"oversized state", func(params *agentpass.AuthorizationURLParams) { params.State = strings.Repeat("a", 513) }},
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

func TestAuthorizationURLAcceptsIPv6LoopbackRedirect(t *testing.T) {
	t.Parallel()
	client, err := agentpass.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.OAuth.AuthorizationURL(agentpass.AuthorizationURLParams{
		ClientID:     "client_123",
		RedirectURI:  "http://[::1]:8080/callback",
		Capabilities: []string{"ai.inference"},
		State:        "state",
	})
	if err != nil {
		t.Fatalf("expected IPv6 loopback redirect to be accepted: %v", err)
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
		`{"access_token":"` + strings.Repeat("a", 4097) + `","refresh_token":"refresh","token_type":"Bearer","expires_in":3600,"grant_id":"grant_123"}`,
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

func TestAPIErrorTextIsBoundedAndSafeForSingleLineLogs(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		body        string
		wantCode    string
		wantGeneric bool
	}{
		{
			name:        "unsafe code",
			body:        `{"error":{"code":"bad\nforged","message":"provider secret"}}`,
			wantGeneric: true,
		},
		{
			name:     "control and oversized message",
			body:     `{"error":{"code":"provider_unavailable","message":"` + strings.Repeat("x", 700) + `\nforged"}}`,
			wantCode: "provider_unavailable",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t, func(response http.ResponseWriter, _ *http.Request) {
				response.WriteHeader(http.StatusBadGateway)
				_, _ = io.WriteString(response, test.body)
			}, agentpass.WithAccessToken("token"))
			_, err := client.Usage.Current(context.Background())
			var apiError *agentpass.APIError
			if !errors.As(err, &apiError) {
				t.Fatalf("expected APIError, got %T: %v", err, err)
			}
			if strings.ContainsAny(err.Error(), "\r\n") || len([]rune(apiError.Message)) > 512 {
				t.Fatalf("unsafe API error text: %q", err.Error())
			}
			if test.wantGeneric {
				if apiError.Code != "" || strings.Contains(err.Error(), "provider secret") {
					t.Fatalf("malformed code was exposed: %+v", apiError)
				}
			} else if apiError.Code != test.wantCode {
				t.Fatalf("expected code %q, got %q", test.wantCode, apiError.Code)
			}
		})
	}
}
