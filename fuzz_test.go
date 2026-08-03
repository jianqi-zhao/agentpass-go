package agentpass

import "testing"

func FuzzNormalizeBaseURL(fuzzer *testing.F) {
	for _, seed := range []string{
		DefaultBaseURL,
		"http://localhost:8080/agentpass",
		"https://user:password@example.test/path",
		"not a URL",
		"https://[::1]/path",
	} {
		fuzzer.Add(seed)
	}
	fuzzer.Fuzz(func(t *testing.T, raw string) {
		_, _ = normalizeBaseURL(raw)
	})
}

func FuzzValidateRedirectURI(fuzzer *testing.F) {
	for _, seed := range []string{
		"https://app.example/callback",
		"http://localhost:8080/callback",
		"https://user@app.example/callback#fragment",
		"/callback",
	} {
		fuzzer.Add(seed)
	}
	fuzzer.Fuzz(func(t *testing.T, raw string) {
		_ = validateRedirectURI(raw)
	})
}

func FuzzDecodeAPIError(fuzzer *testing.F) {
	fuzzer.Add(uint16(400), []byte(`{"error":{"code":"invalid_input","message":"bad input"}}`))
	fuzzer.Add(uint16(502), []byte(`not-json`))
	fuzzer.Fuzz(func(t *testing.T, status uint16, body []byte) {
		if err := decodeAPIError(int(status), body); err == nil {
			t.Fatal("decodeAPIError returned nil")
		}
	})
}
