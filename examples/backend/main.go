package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	agentpass "github.com/jianqi-zhao/agentpass-go"
)

func main() {
	accessToken := os.Getenv("AGENTPASS_ACCESS_TOKEN")
	if accessToken == "" {
		fmt.Fprintln(os.Stderr, "AGENTPASS_ACCESS_TOKEN is required")
		os.Exit(2)
	}

	baseURL := os.Getenv("AGENTPASS_BASE_URL")
	if baseURL == "" {
		baseURL = agentpass.DefaultBaseURL
	}
	providerURL, err := agentpass.OpenAIBaseURL(baseURL)
	if err != nil {
		fail(err)
	}

	idempotencyKey := os.Getenv("AGENTPASS_IDEMPOTENCY_KEY")
	if idempotencyKey == "" {
		idempotencyKey, err = agentpass.NewIdempotencyKey()
		if err != nil {
			fail(err)
		}
	}
	input := os.Getenv("AGENTPASS_INPUT")
	if input == "" {
		input = "Explain portable AI access in one sentence."
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	payload, err := json.Marshal(map[string]any{
		"model":             "gpt-5.6-sol",
		"input":             input,
		"reasoning":         map[string]string{"effort": "medium"},
		"max_output_tokens": 600,
	})
	if err != nil {
		fail(err)
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		providerURL+"/responses",
		bytes.NewReader(payload),
	)
	if err != nil {
		fail(err)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", idempotencyKey)
	request.Header.Set("X-AgentPass-Max-Credits", "40")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		fail(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		fail(err)
	}
	if response.StatusCode != http.StatusOK {
		fail(fmt.Errorf("AgentPass returned HTTP %d: %s", response.StatusCode, body))
	}
	var result any
	if err := json.Unmarshal(body, &result); err != nil {
		fail(err)
	}
	output := map[string]any{
		"response":          result,
		"request_id":        response.Header.Get("X-AgentPass-Request-Id"),
		"credits_used":      response.Header.Get("X-AgentPass-Credits-Used"),
		"credits_remaining": response.Header.Get("X-AgentPass-Credits-Remaining"),
	}
	if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
