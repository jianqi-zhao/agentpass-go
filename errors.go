package agentpass

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

const (
	maxAPIErrorCodeBytes    = 128
	maxAPIErrorMessageRunes = 512
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
	if err == nil {
		return "agentpass: API request failed"
	}
	code := safeAPIErrorCode(err.Code)
	if code == "" {
		return fmt.Sprintf("agentpass: API request failed with HTTP %d", err.StatusCode)
	}
	message := safeAPIErrorMessage(err.Message)
	if message == "" {
		return fmt.Sprintf("agentpass: %s (HTTP %d)", code, err.StatusCode)
	}
	return fmt.Sprintf("agentpass: %s (HTTP %d): %s", code, err.StatusCode, message)
}

func safeAPIErrorCode(value string) string {
	if value == "" || len(value) > maxAPIErrorCodeBytes {
		return ""
	}
	for _, character := range value {
		if !((character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '_' || character == '-' || character == '.') {
			return ""
		}
	}
	return value
}

func safeAPIErrorMessage(value string) string {
	value = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return ' '
		}
		return character
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	characters := []rune(value)
	if len(characters) > maxAPIErrorMessageRunes {
		value = string(characters[:maxAPIErrorMessageRunes])
	}
	return value
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
		Code:       safeAPIErrorCode(envelope.Error.Code),
		Message:    safeAPIErrorMessage(envelope.Error.Message),
		Details:    envelope.Error.Details,
	}
}
