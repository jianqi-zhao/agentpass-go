package main

import (
	"context"
	"encoding/json"
	"fmt"
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

	options := []agentpass.Option{agentpass.WithAccessToken(accessToken)}
	if baseURL := os.Getenv("AGENTPASS_BASE_URL"); baseURL != "" {
		options = append(options, agentpass.WithBaseURL(baseURL))
	}
	client, err := agentpass.NewClient(options...)
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
	result, err := client.Responses.Create(ctx, agentpass.CreateResponseParams{
		Capability:     "text.fast",
		Input:          input,
		MaxCredits:     30,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		fail(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
