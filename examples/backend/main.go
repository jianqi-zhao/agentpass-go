package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"

	agentpass "github.com/jianqi-zhao/agentpass-go"
	"github.com/jianqi-zhao/agentpass-go/examples/internal/openairesponse"
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

	ctx, cancel := context.WithTimeout(context.Background(), agentpass.DefaultHTTPTimeout)
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
	request.Header.Set("X-AgentPass-Max-Credits", "0.4")
	response, err := agentpass.ProviderHTTPClient(http.DefaultClient).Do(request)
	if err != nil {
		fail(err)
	}
	defer response.Body.Close()
	body, err := openairesponse.ReadBody(response.Body)
	if err != nil {
		fail(err)
	}
	if response.StatusCode != http.StatusOK {
		fail(fmt.Errorf("%s", openairesponse.DescribeHTTPError(response.StatusCode, body)))
	}
	metadata, err := openairesponse.Parse(body)
	if err != nil {
		fail(err)
	}
	disposition, err := openairesponse.ClassifyStatus(metadata.Status)
	if err != nil {
		fail(err)
	}
	switch disposition {
	case openairesponse.DispositionPollable:
		fail(fmt.Errorf(
			"OpenAI response is %s; poll GET /openai/v1/responses/%s and do not submit another POST",
			metadata.Status,
			url.PathEscape(metadata.ID),
		))
	case openairesponse.DispositionFailed:
		fail(fmt.Errorf(
			"OpenAI response failed: %s. AgentPass did not charge this attempt; start a new logical attempt with a new Idempotency-Key",
			openairesponse.DescribeError(metadata.Error),
		))
	case openairesponse.DispositionBillable:
		// Both completed and incomplete are terminal and can contain verified usage.
	}
	// The shared parser requires X-AgentPass-Request-Id,
	// X-AgentPass-Credits-Reserved, X-AgentPass-Credits-Used, and
	// X-AgentPass-Credits-Remaining on every settled response.
	receipt, err := openairesponse.ParseSettlementReceipt(response.Header, metadata.ID)
	if err != nil {
		fail(err)
	}
	var result any
	if err := json.Unmarshal(body, &result); err != nil {
		fail(err)
	}
	output := map[string]any{
		"response":          result,
		"request_id":        receipt.RequestID,
		"credits_reserved":  receipt.CreditsReserved,
		"credits_used":      receipt.CreditsUsed,
		"credits_remaining": receipt.CreditsRemaining,
	}
	if err := json.NewEncoder(os.Stdout).Encode(output); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
