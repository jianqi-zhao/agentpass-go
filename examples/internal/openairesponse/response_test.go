package openairesponse

import (
	"net/http"
	"strings"
	"testing"
)

func TestClassifyStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status      string
		disposition Disposition
	}{
		{status: "queued", disposition: DispositionPollable},
		{status: "in_progress", disposition: DispositionPollable},
		{status: "completed", disposition: DispositionBillable},
		{status: "incomplete", disposition: DispositionBillable},
		{status: "failed", disposition: DispositionFailed},
	}
	for _, test := range tests {
		test := test
		t.Run(test.status, func(t *testing.T) {
			t.Parallel()
			disposition, err := ClassifyStatus(test.status)
			if err != nil {
				t.Fatalf("classify status: %v", err)
			}
			if disposition != test.disposition {
				t.Fatalf("got disposition %v, want %v", disposition, test.disposition)
			}
		})
	}
	if _, err := ClassifyStatus("cancelled"); err == nil {
		t.Fatal("expected unsupported status error")
	}
}

func TestDescribeErrorSanitizesProviderText(t *testing.T) {
	t.Parallel()
	description := DescribeError(&Error{
		Code:    "provider\n\x1b[31merror",
		Message: strings.Repeat("x", 600) + "\x00secret",
	})
	if strings.ContainsAny(description, "\n\r\x00\x1b") {
		t.Fatalf("description contains a control character: %q", description)
	}
	if !strings.HasSuffix(description, "…") {
		t.Fatalf("description was not bounded: %q", description)
	}
}

func TestDescribeHTTPErrorDoesNotEchoUnexpectedBodies(t *testing.T) {
	t.Parallel()
	description := DescribeHTTPError(502, []byte("upstream secret response"))
	if description != "AgentPass returned HTTP 502" {
		t.Fatalf("unexpected description: %q", description)
	}

	description = DescribeHTTPError(
		504,
		[]byte(`{"error":{"code":"provider_timeout","message":"try later"}}`),
	)
	if description != "AgentPass returned HTTP 504: provider_timeout: try later" {
		t.Fatalf("unexpected decoded description: %q", description)
	}
}

func TestReadBodyRejectsOnlyResponsesAboveTheLimit(t *testing.T) {
	t.Parallel()
	exact := strings.Repeat("x", int(maximumResponseBytes))
	body, err := ReadBody(strings.NewReader(exact))
	if err != nil {
		t.Fatalf("read exact-limit response: %v", err)
	}
	if len(body) != len(exact) {
		t.Fatalf("read %d bytes, want %d", len(body), len(exact))
	}
	if _, err := ReadBody(strings.NewReader(exact + "x")); err == nil {
		t.Fatal("expected oversized response to be rejected")
	}
}

func TestParseValidatesSharedResponseIdentity(t *testing.T) {
	t.Parallel()
	response, err := Parse([]byte(`{
		"id":"resp_123","object":"response","status":"completed",
		"output":[{"type":"message","content":[{"type":"output_text","text":"done"}]}]
	}`))
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if response.ID != "resp_123" || response.Output[0].Content[0].Text != "done" {
		t.Fatalf("unexpected response: %+v", response)
	}
	for _, body := range []string{
		`{"object":"response","status":"completed"}`,
		`{"id":"resp_123","object":"response"}`,
		`{"id":"resp_123","object":"not_response","status":"completed"}`,
		`{"id":"resp_\u000a123","object":"response","status":"completed"}`,
		`{"id":"resp_\u200b123","object":"response","status":"completed"}`,
		`{"id":"` + strings.Repeat("r", 513) + `","object":"response","status":"completed"}`,
		`{"id":"resp_123","object":"response","status":" completed"}`,
		`{"id":"resp_123","object":"response","status":"` + strings.Repeat("s", 51) + `"}`,
		`{"id":`,
	} {
		if _, err := Parse([]byte(body)); err == nil {
			t.Fatalf("expected malformed response %q to be rejected", body)
		}
	}
}

func TestParseSettlementReceiptRequiresCompleteMatchingHeaders(t *testing.T) {
	t.Parallel()
	headers := http.Header{
		"X-Agentpass-Request-Id":        {"resp_123"},
		"X-Agentpass-Credits-Reserved":  {"0.40"},
		"X-Agentpass-Credits-Used":      {"0.07"},
		"X-Agentpass-Credits-Remaining": {"9.93"},
	}
	receipt, err := ParseSettlementReceipt(headers, "resp_123")
	if err != nil {
		t.Fatalf("parse settlement receipt: %v", err)
	}
	if receipt.CreditsReserved != "0.40" || receipt.CreditsRemaining != "9.93" {
		t.Fatalf("unexpected receipt: %+v", receipt)
	}

	missing := headers.Clone()
	missing.Del("X-AgentPass-Credits-Used")
	if _, err := ParseSettlementReceipt(missing, "resp_123"); err == nil {
		t.Fatal("expected incomplete receipt to be rejected")
	}
	if _, err := ParseSettlementReceipt(headers, "resp_other"); err == nil {
		t.Fatal("expected mismatched receipt to be rejected")
	}
	for _, values := range [][3]string{
		{"not-credit", "0.07", "9.93"},
		{"0.40", "-0.01", "9.93"},
		{"0.40", "0.001", "9.93"},
		{"0.05", "0.07", "9.93"},
		{"0.40", "0.07", "NaN"},
	} {
		malformed := headers.Clone()
		malformed.Set("X-AgentPass-Credits-Reserved", values[0])
		malformed.Set("X-AgentPass-Credits-Used", values[1])
		malformed.Set("X-AgentPass-Credits-Remaining", values[2])
		if _, err := ParseSettlementReceipt(malformed, "resp_123"); err == nil {
			t.Fatalf("expected malformed receipt values %v to be rejected", values)
		}
	}
}
