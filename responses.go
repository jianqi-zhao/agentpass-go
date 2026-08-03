package agentpass

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ResponsesService calls AgentPass's metered AI gateway.
type ResponsesService struct {
	client *Client
}

// CreateResponseParams describes an AI request. MaxCredits is omitted when it
// is zero. Set IdempotencyKey yourself when a job may be retried after restart.
type CreateResponseParams struct {
	Capability     string `json:"capability"`
	Input          string `json:"input"`
	MaxCredits     int    `json:"max_credits,omitempty"`
	IdempotencyKey string `json:"-"`
}

// Usage reports provider-normalized token usage. Character fields remain for
// compatibility with local test providers and may be zero in live responses.
type Usage struct {
	InputTokens       int `json:"inputTokens"`
	CachedInputTokens int `json:"cachedInputTokens"`
	OutputTokens      int `json:"outputTokens"`
	ReasoningTokens   int `json:"reasoningTokens"`
	TotalTokens       int `json:"totalTokens"`
	InputCharacters   int `json:"inputCharacters,omitempty"`
	OutputCharacters  int `json:"outputCharacters,omitempty"`
}

// Receipt is AgentPass's authoritative settled metering record.
type Receipt struct {
	RequestID        string `json:"request_id"`
	App              string `json:"app"`
	Capability       string `json:"capability"`
	CreditsUsed      int    `json:"credits_used"`
	RemainingCredits int    `json:"remaining_credits"`
	SettledAt        string `json:"settled_at"`
}

// Response is a normalized AI result with its settled AgentPass receipt.
type Response struct {
	ID         string `json:"id"`
	Object     string `json:"object"`
	Model      string `json:"model"`
	OutputText string `json:"output_text"`
	Usage      Usage  `json:"usage"`
	AgentPass  struct {
		Receipt Receipt `json:"receipt"`
	} `json:"agentpass"`
}

// Create makes one authenticated, metered AI request. If IdempotencyKey is
// empty, the SDK generates a cryptographically random key for this call.
func (service *ResponsesService) Create(
	ctx context.Context,
	params CreateResponseParams,
) (*Response, error) {
	if strings.TrimSpace(params.Capability) == "" ||
		strings.TrimSpace(params.Capability) != params.Capability ||
		strings.ContainsAny(params.Capability, " \t\r\n") {
		return nil, errors.New("agentpass: capability is required")
	}
	if strings.TrimSpace(params.Input) == "" {
		return nil, errors.New("agentpass: input is required")
	}
	if params.MaxCredits < 0 {
		return nil, errors.New("agentpass: max credits cannot be negative")
	}
	if len(params.IdempotencyKey) > 200 {
		return nil, errors.New("agentpass: idempotency key cannot exceed 200 characters")
	}
	if params.IdempotencyKey != "" &&
		(strings.TrimSpace(params.IdempotencyKey) != params.IdempotencyKey ||
			!validHeaderValue(params.IdempotencyKey)) {
		return nil, errors.New("agentpass: idempotency key contains invalid characters")
	}

	idempotencyKey := params.IdempotencyKey
	if idempotencyKey == "" {
		var err error
		idempotencyKey, err = NewIdempotencyKey()
		if err != nil {
			return nil, err
		}
	}

	request, err := service.client.newJSONRequest(ctx, http.MethodPost, "/v1/responses", params)
	if err != nil {
		return nil, err
	}
	if err := service.client.authorizeRequest(ctx, request); err != nil {
		return nil, err
	}
	request.Header.Set("Idempotency-Key", idempotencyKey)

	response := &Response{}
	if err := service.client.do(request, response); err != nil {
		return nil, err
	}
	if err := validateResponse(response, params.Capability); err != nil {
		return nil, err
	}
	return response, nil
}

func validateResponse(response *Response, capability string) error {
	if strings.TrimSpace(response.ID) == "" ||
		response.Object != "agentpass.response" ||
		strings.TrimSpace(response.Model) == "" ||
		strings.TrimSpace(response.OutputText) == "" {
		return fmt.Errorf("%w: missing response identity or output", ErrInvalidResponse)
	}
	receipt := response.AgentPass.Receipt
	if receipt.RequestID != response.ID ||
		strings.TrimSpace(receipt.App) == "" ||
		receipt.Capability != capability ||
		receipt.CreditsUsed <= 0 ||
		receipt.RemainingCredits < 0 ||
		strings.TrimSpace(receipt.SettledAt) == "" {
		return fmt.Errorf("%w: malformed settlement receipt", ErrInvalidResponse)
	}
	if _, err := time.Parse(time.RFC3339Nano, receipt.SettledAt); err != nil {
		return fmt.Errorf("%w: malformed settlement timestamp", ErrInvalidResponse)
	}
	usage := response.Usage
	if usage.InputTokens <= 0 || usage.CachedInputTokens < 0 ||
		usage.OutputTokens <= 0 || usage.ReasoningTokens < 0 ||
		usage.TotalTokens <= 0 || usage.InputCharacters < 0 || usage.OutputCharacters < 0 ||
		usage.CachedInputTokens > usage.InputTokens || usage.ReasoningTokens > usage.OutputTokens ||
		usage.TotalTokens < usage.InputTokens+usage.OutputTokens {
		return fmt.Errorf("%w: malformed token usage", ErrInvalidResponse)
	}
	return nil
}

// NewIdempotencyKey returns a UUID-shaped cryptographically random request key.
func NewIdempotencyKey() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", errors.New("agentpass: generate idempotency key: " + err.Error())
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80

	encoded := make([]byte, 36)
	hex.Encode(encoded[0:8], value[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], value[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], value[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], value[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], value[10:16])
	return string(encoded), nil
}
