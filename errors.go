package agentpass

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrInvalidResponse identifies a successful HTTP response that does not
// satisfy the AgentPass protocol contract.
var ErrInvalidResponse = errors.New("agentpass: invalid API response")

// APIError is returned when AgentPass rejects an API request.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Details    json.RawMessage
}

// Error implements error.
func (err *APIError) Error() string {
	if err.Code == "" {
		return fmt.Sprintf("agentpass: API request failed with HTTP %d", err.StatusCode)
	}
	return fmt.Sprintf("agentpass: %s (HTTP %d): %s", err.Code, err.StatusCode, err.Message)
}

func decodeAPIError(statusCode int, body []byte) error {
	envelope := struct {
		Error struct {
			Code    string          `json:"code"`
			Message string          `json:"message"`
			Details json.RawMessage `json:"details"`
		} `json:"error"`
	}{}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Error.Code == "" {
		return &APIError{StatusCode: statusCode}
	}
	return &APIError{
		StatusCode: statusCode,
		Code:       envelope.Error.Code,
		Message:    envelope.Error.Message,
		Details:    envelope.Error.Details,
	}
}
