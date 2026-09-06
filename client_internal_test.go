package agentpass

import "testing"

func TestDefaultClientUsesDocumentedProviderSafeTimeout(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	if client.httpClient.Timeout != DefaultHTTPTimeout {
		t.Fatalf("default timeout = %v, want %v", client.httpClient.Timeout, DefaultHTTPTimeout)
	}
}
