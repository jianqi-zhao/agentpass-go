# AgentPass Go SDK

The AgentPass Go SDK is a dependency-free backend helper for:

- building user authorization URLs;
- confidential authorization-code exchange and refresh-token rotation;
- provider-native OpenAI and Anthropic base URLs;
- grant-scoped weekly and monthly usage summaries;
- the legacy AgentPass response API during migration;
- stable idempotency keys; and
- typed, settled metering receipts.

The SDK does not meter locally. AgentPass reserves and settles credits in its
transactional ledger, then returns the authoritative receipt.

## Install

```bash
go get github.com/jianqi-zhao/agentpass-go@latest
```

## Call the provider-native gateway

```go
baseURL, err := agentpass.OpenAIBaseURL(agentpass.DefaultBaseURL)
if err != nil {
    return err
}
// Configure the official OpenAI Go client with baseURL and the connected
// user's AgentPass access token. Its Responses request/response types remain
// unchanged. Use a stable Idempotency-Key and X-AgentPass-Max-Credits header.
```

For Anthropic clients, use `agentpass.AnthropicBaseURL(...)`. The returned roots
are `.../openai/v1` and `.../anthropic`, respectively. AgentPass preserves
provider-native request and response bodies and returns settlement metadata in
`X-AgentPass-*` headers.

## OAuth

Use `client.OAuth.AuthorizationURL` to request `ai.inference` plus explicit
provider-qualified model keys, create the consent URL, and then use
`client.OAuth.ExchangeAuthorizationCode` from the backend callback. Never send
the client secret to a browser or mobile application. Store the returned opaque
access and refresh tokens encrypted in application-owned storage.

AgentPass access tokens expire after one hour. Store refresh tokens encrypted
and rotate them with `client.OAuth.RefreshAccessToken`; refresh-token reuse
revokes the entire token family.

Use `client.Usage.Current(ctx)` with the connected user's access token to read
the app grant's calendar-week and calendar-month limits, settled usage,
in-flight reservations, and remaining credits. Return only the display fields
your frontend needs; never send the AgentPass access token to frontend code.

For AI calls, use the official provider SDK and replace only its base URL
and credential. `agentpass.OpenAIBaseURL(agentpass.DefaultBaseURL)` returns the
OpenAI-compatible root, while `agentpass.AnthropicBaseURL(...)` returns the
Anthropic-compatible root. Provider request and response bodies remain native;
AgentPass settlement is returned in `X-AgentPass-*` headers.
The default HTTP timeout is five minutes; use `WithHTTPClient` when your
backend needs a different deadline.

The client refuses unexpected HTTP redirects and rejects malformed successful
responses before returning them to application code. API rejections are typed
as `*agentpass.APIError`; malformed success payloads wrap
`agentpass.ErrInvalidResponse`. Retry only safe failures with the same
idempotency key and identical request body.

See `examples/backend` for a native OpenAI Responses HTTP request and
`examples/oauth-backend` for a complete local authorization, callback,
provider-native inference, and settlement-header flow. The legacy typed
`client.Responses` API remains available for applications migrating from
AgentPass's earlier `capability/input` contract.

## Verify

```bash
go test ./...
go vet ./...
```

Build production applications with a currently supported, fully patched Go
release. Go embeds its standard library in your binary, so upgrading this module
alone cannot fix vulnerabilities in an outdated Go toolchain.

## License

AgentPass Go SDK is licensed under the Apache License 2.0.
