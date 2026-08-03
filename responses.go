package agentpass

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
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
	if strings.TrimSpace(params.Capability) == "" {
		return nil, errors.New("agentpass: capability is required")
	}
	if strings.TrimSpace(params.Input) == "" {
		return nil, errors.New("agentpass: input is required")
	}
	if len(params.IdempotencyKey) > 200 {
		return nil, errors.New("agentpass: idempotency key cannot exceed 200 characters")
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
	return response, nil
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
