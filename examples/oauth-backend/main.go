package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	agentpass "github.com/jianqi-zhao/agentpass-go"
)

const (
	stateCookie   = "agentpass_example_oauth_state"
	sessionCookie = "agentpass_example_session"
)

type session struct {
	accessToken  string
	refreshToken string
	expiresAt    time.Time
}

type application struct {
	client       *agentpass.Client
	clientID     string
	clientSecret string
	redirectURI  string
	providerURL  string
	secureCookie bool
	sessions     map[string]session
	mutex        sync.RWMutex
}

var page = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>AgentPass Go example</title></head>
<body><main style="max-width:760px;margin:48px auto;font:16px/1.5 system-ui">
<h1>AgentPass Go OAuth example</h1>
{{if .Connected}}
  <p>AgentPass is connected. The access token remains in this backend process.</p>
  <form method="post" action="/generate"><label>Prompt<br><textarea name="input" rows="5" style="width:100%" required>{{.Input}}</textarea></label><br><button>Generate</button></form>
{{else}}
  <p><a href="/connect">Connect AgentPass</a></p>
{{end}}
{{if .Output}}<h2>Response</h2><pre style="white-space:pre-wrap">{{.Output}}</pre>{{end}}
{{if .Receipt}}<h2>Receipt</h2><pre>{{.Receipt}}</pre>{{end}}
{{if .Error}}<h2>Error</h2><pre>{{.Error}}</pre>{{end}}
</main></body></html>`))

func requiredEnvironment(name string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		log.Fatalf("%s is required", name)
	}
	return value
}

func randomToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (app *application) setCookie(response http.ResponseWriter, cookie *http.Cookie) {
	cookie.HttpOnly = true
	cookie.SameSite = http.SameSiteLaxMode
	cookie.Secure = app.secureCookie
	http.SetCookie(response, cookie)
}

func (app *application) currentSession(request *http.Request) (session, bool) {
	cookie, err := request.Cookie(sessionCookie)
	if err != nil {
		return session{}, false
	}
	app.mutex.RLock()
	current, exists := app.sessions[cookie.Value]
	app.mutex.RUnlock()
	return current, exists && current.expiresAt.After(time.Now())
}

func render(response http.ResponseWriter, values map[string]any) {
	response.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := page.Execute(response, values); err != nil {
		log.Printf("render page: %v", err)
	}
}

func (app *application) home(response http.ResponseWriter, request *http.Request) {
	_, connected := app.currentSession(request)
	render(response, map[string]any{"Connected": connected})
}

func (app *application) connect(response http.ResponseWriter, request *http.Request) {
	state, err := randomToken()
	if err != nil {
		http.Error(response, "could not create OAuth state", http.StatusInternalServerError)
		return
	}
	app.setCookie(response, &http.Cookie{
		Name: stateCookie, Value: state, Path: "/callback", MaxAge: 600,
	})
	authorizationURL, err := app.client.OAuth.AuthorizationURL(agentpass.AuthorizationURLParams{
		ClientID: app.clientID, RedirectURI: app.redirectURI,
		Capabilities: []string{"ai.inference"},
		Models:       []string{"openai:gpt-5.6-sol"},
		DefaultModel: "openai:gpt-5.6-sol", MonthlyLimit: 1_000,
		WeeklyLimit: 250, State: state,
	})
	if err != nil {
		http.Error(response, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(response, request, authorizationURL, http.StatusSeeOther)
}

func (app *application) callback(response http.ResponseWriter, request *http.Request) {
	state, err := request.Cookie(stateCookie)
	receivedState := request.URL.Query().Get("state")
	if err != nil || receivedState == "" || subtle.ConstantTimeCompare([]byte(state.Value), []byte(receivedState)) != 1 {
		http.Error(response, "invalid OAuth state", http.StatusBadRequest)
		return
	}
	app.setCookie(response, &http.Cookie{Name: stateCookie, Path: "/callback", MaxAge: -1})
	if oauthError := request.URL.Query().Get("error"); oauthError != "" {
		render(response, map[string]any{"Error": "AgentPass authorization failed: " + oauthError})
		return
	}

	ctx, cancel := context.WithTimeout(request.Context(), 30*time.Second)
	defer cancel()
	token, err := app.client.OAuth.ExchangeAuthorizationCode(ctx, agentpass.ExchangeCodeParams{
		Code: request.URL.Query().Get("code"), ClientID: app.clientID,
		ClientSecret: app.clientSecret, RedirectURI: app.redirectURI,
	})
	if err != nil {
		render(response, map[string]any{"Error": err.Error()})
		return
	}
	sessionToken, err := randomToken()
	if err != nil {
		http.Error(response, "could not create application session", http.StatusInternalServerError)
		return
	}
	app.mutex.Lock()
	app.sessions[sessionToken] = session{
		accessToken: token.AccessToken, refreshToken: token.RefreshToken,
		expiresAt: time.Now().Add(time.Duration(token.ExpiresIn) * time.Second),
	}
	app.mutex.Unlock()
	app.setCookie(response, &http.Cookie{
		Name: sessionCookie, Value: sessionToken, Path: "/", MaxAge: token.ExpiresIn,
	})
	http.Redirect(response, request, "/", http.StatusSeeOther)
}

func (app *application) generate(response http.ResponseWriter, request *http.Request) {
	current, connected := app.currentSession(request)
	if !connected {
		http.Redirect(response, request, "/connect", http.StatusSeeOther)
		return
	}
	if err := request.ParseForm(); err != nil {
		http.Error(response, "invalid form", http.StatusBadRequest)
		return
	}
	input := strings.TrimSpace(request.Form.Get("input"))
	if input == "" || len(input) > 10_000 {
		http.Error(response, "input must contain 1 to 10,000 characters", http.StatusBadRequest)
		return
	}
	idempotencyKey, err := randomToken()
	if err != nil {
		http.Error(response, "could not create request ID", http.StatusInternalServerError)
		return
	}
	payload, err := json.Marshal(map[string]any{
		"model":             "gpt-5.6-sol",
		"input":             input,
		"reasoning":         map[string]string{"effort": "medium"},
		"max_output_tokens": 600,
	})
	if err != nil {
		http.Error(response, "could not encode request", http.StatusInternalServerError)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 5*time.Minute)
	defer cancel()
	providerRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		app.providerURL+"/responses",
		bytes.NewReader(payload),
	)
	if err != nil {
		render(response, map[string]any{"Connected": true, "Input": input, "Error": err.Error()})
		return
	}
	providerRequest.Header.Set("Authorization", "Bearer "+current.accessToken)
	providerRequest.Header.Set("Content-Type", "application/json")
	providerRequest.Header.Set("Idempotency-Key", idempotencyKey)
	providerRequest.Header.Set("X-AgentPass-Max-Credits", "40")
	providerResponse, err := http.DefaultClient.Do(providerRequest)
	if err != nil {
		render(response, map[string]any{"Connected": true, "Input": input, "Error": err.Error()})
		return
	}
	defer providerResponse.Body.Close()
	body, err := io.ReadAll(io.LimitReader(providerResponse.Body, 4<<20))
	if err != nil {
		render(response, map[string]any{"Connected": true, "Input": input, "Error": err.Error()})
		return
	}
	if providerResponse.StatusCode != http.StatusOK {
		render(response, map[string]any{
			"Connected": true, "Input": input,
			"Error": fmt.Sprintf("AgentPass returned HTTP %d: %s", providerResponse.StatusCode, body),
		})
		return
	}
	var result struct {
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		render(response, map[string]any{"Connected": true, "Input": input, "Error": err.Error()})
		return
	}
	var output strings.Builder
	for _, item := range result.Output {
		for _, content := range item.Content {
			if item.Type == "message" && content.Type == "output_text" {
				output.WriteString(content.Text)
			}
		}
	}
	render(response, map[string]any{
		"Connected": true, "Input": input, "Output": output.String(),
		"Receipt": fmt.Sprintf(
			"request=%s\ncredits=%s\nremaining=%s",
			providerResponse.Header.Get("X-AgentPass-Request-Id"),
			providerResponse.Header.Get("X-AgentPass-Credits-Used"),
			providerResponse.Header.Get("X-AgentPass-Credits-Remaining"),
		),
	})
}

func main() {
	baseURL := strings.TrimSpace(os.Getenv("AGENTPASS_BASE_URL"))
	if baseURL == "" {
		baseURL = agentpass.DefaultBaseURL
	}
	client, err := agentpass.NewClient(agentpass.WithBaseURL(baseURL))
	if err != nil {
		log.Fatal(err)
	}
	providerURL, err := agentpass.OpenAIBaseURL(baseURL)
	if err != nil {
		log.Fatal(err)
	}
	redirectURI := requiredEnvironment("AGENTPASS_REDIRECT_URI")
	app := &application{
		client: client, clientID: requiredEnvironment("AGENTPASS_CLIENT_ID"),
		clientSecret: requiredEnvironment("AGENTPASS_CLIENT_SECRET"),
		redirectURI:  redirectURI, providerURL: providerURL,
		secureCookie: strings.HasPrefix(redirectURI, "https://"),
		sessions:     make(map[string]session),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", app.home)
	mux.HandleFunc("GET /connect", app.connect)
	mux.HandleFunc("GET /callback", app.callback)
	mux.HandleFunc("POST /generate", app.generate)
	address := ":" + strings.TrimSpace(os.Getenv("PORT"))
	if address == ":" {
		address = ":8080"
	}
	log.Printf("OAuth example listening on http://127.0.0.1%s", address)
	log.Fatal(http.ListenAndServe(address, mux))
}
