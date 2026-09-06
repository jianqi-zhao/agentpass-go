// Package openairesponse contains the response-state handling shared by the
// provider-native examples. It is internal so it does not become SDK API.
package openairesponse

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode"
)

const maximumResponseBytes int64 = 4 << 20

// Error is the safe subset of an OpenAI response error used by the examples.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Content is the output subset rendered by the interactive example.
type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Output is the response output subset rendered by the interactive example.
type Output struct {
	Type    string    `json:"type"`
	Content []Content `json:"content"`
}

// Response contains the fields both provider-native examples must interpret.
type Response struct {
	ID     string   `json:"id"`
	Object string   `json:"object"`
	Status string   `json:"status"`
	Error  *Error   `json:"error"`
	Output []Output `json:"output"`
}

// SettlementReceipt contains AgentPass's authoritative native response
// headers. The values remain strings so examples preserve the public decimal
// representation exactly as it appeared on the wire.
type SettlementReceipt struct {
	RequestID        string
	CreditsReserved  string
	CreditsUsed      string
	CreditsRemaining string
}

// ReadBody reads a provider-native response while enforcing the same 4 MiB
// boundary as the public SDK client. Reading one additional byte distinguishes
// an exactly-at-limit response from a truncated oversized response.
func ReadBody(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maximumResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read AgentPass response: %w", err)
	}
	if int64(len(body)) > maximumResponseBytes {
		return nil, fmt.Errorf("AgentPass response exceeded 4 MiB")
	}
	return body, nil
}

// Parse validates the response identity fields shared by both examples.
func Parse(body []byte) (*Response, error) {
	var response Response
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode AgentPass OpenAI response: %w", err)
	}
	if strings.TrimSpace(response.ID) == "" ||
		len(response.ID) > 512 ||
		strings.TrimSpace(response.ID) != response.ID ||
		strings.ContainsFunc(response.ID, func(character rune) bool {
			return unicode.IsControl(character) || unicode.Is(unicode.Cf, character)
		}) ||
		response.Object != "response" ||
		strings.TrimSpace(response.Status) == "" ||
		len(response.Status) > 50 ||
		strings.TrimSpace(response.Status) != response.Status ||
		strings.ContainsFunc(response.Status, func(character rune) bool {
			return unicode.IsControl(character) || unicode.Is(unicode.Cf, character)
		}) {
		return nil, fmt.Errorf("AgentPass returned a malformed OpenAI response")
	}
	return &response, nil
}

// ParseSettlementReceipt requires the complete settlement header set and binds
// it to the response body that the application is about to accept.
func ParseSettlementReceipt(headers http.Header, responseID string) (SettlementReceipt, error) {
	receipt := SettlementReceipt{
		RequestID:        strings.TrimSpace(headers.Get("X-AgentPass-Request-Id")),
		CreditsReserved:  strings.TrimSpace(headers.Get("X-AgentPass-Credits-Reserved")),
		CreditsUsed:      strings.TrimSpace(headers.Get("X-AgentPass-Credits-Used")),
		CreditsRemaining: strings.TrimSpace(headers.Get("X-AgentPass-Credits-Remaining")),
	}
	for _, required := range []struct {
		name  string
		value string
	}{
		{name: "X-AgentPass-Request-Id", value: receipt.RequestID},
		{name: "X-AgentPass-Credits-Reserved", value: receipt.CreditsReserved},
		{name: "X-AgentPass-Credits-Used", value: receipt.CreditsUsed},
		{name: "X-AgentPass-Credits-Remaining", value: receipt.CreditsRemaining},
	} {
		if required.value == "" {
			return SettlementReceipt{}, fmt.Errorf(
				"AgentPass returned a settled response without %s",
				required.name,
			)
		}
	}
	if receipt.RequestID != responseID {
		return SettlementReceipt{}, fmt.Errorf(
			"AgentPass returned a settlement receipt for a different response",
		)
	}
	reserved, reservedOK := parseCreditCents(receipt.CreditsReserved)
	used, usedOK := parseCreditCents(receipt.CreditsUsed)
	_, remainingOK := parseCreditCents(receipt.CreditsRemaining)
	if !reservedOK || !usedOK || !remainingOK || reserved == 0 || used > reserved {
		return SettlementReceipt{}, fmt.Errorf(
			"AgentPass returned malformed settlement credit values",
		)
	}
	return receipt, nil
}

func parseCreditCents(value string) (uint64, bool) {
	if value == "" || len(value) > 18 || strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		return 0, false
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" || (len(parts) == 2 && (len(parts[1]) == 0 || len(parts[1]) > 2)) {
		return 0, false
	}
	for _, part := range parts {
		if strings.ContainsFunc(part, func(character rune) bool {
			return character < '0' || character > '9'
		}) {
			return 0, false
		}
	}
	whole, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return 0, false
	}
	fraction := uint64(0)
	if len(parts) == 2 {
		fraction, err = strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			return 0, false
		}
		if len(parts[1]) == 1 {
			fraction *= 10
		}
	}
	if whole > (^uint64(0)-fraction)/100 {
		return 0, false
	}
	return whole*100 + fraction, true
}

// Disposition describes what an application may do with a response status.
type Disposition uint8

const (
	DispositionPollable Disposition = iota + 1
	DispositionBillable
	DispositionFailed
)

// ClassifyStatus separates pollable work from terminal provider responses.
func ClassifyStatus(status string) (Disposition, error) {
	switch status {
	case "queued", "in_progress":
		return DispositionPollable, nil
	case "completed", "incomplete":
		return DispositionBillable, nil
	case "failed":
		return DispositionFailed, nil
	default:
		return 0, fmt.Errorf("OpenAI returned an unsupported response status")
	}
}

// DescribeError strips control characters and bounds untrusted provider text
// before an example writes it to a terminal or HTML template.
func DescribeError(providerError *Error) string {
	if providerError == nil {
		return "OpenAI did not provide error details"
	}
	code := safeText(providerError.Code, 80)
	message := safeText(providerError.Message, 500)
	switch {
	case code != "" && message != "":
		return code + ": " + message
	case code != "":
		return code
	case message != "":
		return message
	default:
		return "OpenAI did not provide error details"
	}
}

// DescribeHTTPError decodes only the documented OpenAI error fields. Invalid
// or unexpected response bodies are never copied into user-visible output.
func DescribeHTTPError(statusCode int, body []byte) string {
	var envelope struct {
		Error *Error `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Error == nil {
		return fmt.Sprintf("AgentPass returned HTTP %d", statusCode)
	}
	return fmt.Sprintf(
		"AgentPass returned HTTP %d: %s",
		statusCode,
		DescribeError(envelope.Error),
	)
}

func safeText(value string, maximumRunes int) string {
	value = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) || unicode.Is(unicode.Cf, character) {
			return ' '
		}
		return character
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	characters := []rune(value)
	if len(characters) <= maximumRunes {
		return value
	}
	return string(characters[:maximumRunes]) + "…"
}
