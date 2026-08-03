# AgentPass Go SDK

The AgentPass Go SDK is a dependency-free backend client for:

- building user authorization URLs;
- confidential authorization-code exchange;
- authenticated AI requests;
- stable idempotency keys; and
- typed, settled metering receipts.

The SDK does not meter locally. AgentPass reserves and settles credits in its
transactional ledger, then returns the authoritative receipt.

## Install

```bash
go get github.com/jianqi-zhao/agentpass-go@latest
```

## Call the AI gateway

```go
client, err := agentpass.NewClient(
    agentpass.WithAccessToken(userAccessToken),
)
if err != nil {
    return err
}

result, err := client.Responses.Create(ctx, agentpass.CreateResponseParams{
    Capability:     "text.fast",
    Input:          "Draft a launch announcement.",
    MaxCredits:     30,
    IdempotencyKey: jobID,
})
if err != nil {
    var apiError *agentpass.APIError
    switch {
    case errors.As(err, &apiError):
        // Branch on apiError.Code; retry only documented transient errors.
    case errors.Is(err, agentpass.ErrInvalidResponse):
        // Alert: the successful HTTP payload violated the protocol contract.
    }
    return err
}

fmt.Println(result.OutputText)
fmt.Println(result.AgentPass.Receipt.CreditsUsed)
fmt.Println(result.Usage.TotalTokens)
```

Use a stable application job or request ID as `IdempotencyKey` whenever work
may be retried after a process restart. When omitted, the SDK creates a secure
random key for the current call.

## OAuth

Use `client.OAuth.AuthorizationURL` to create the consent URL and
`client.OAuth.ExchangeAuthorizationCode` from the backend callback. Never send
the client secret to a browser or mobile application. Store the returned opaque
access token encrypted in application-owned storage.

AgentPass access tokens currently expire after one hour and do not have a
refresh-token flow. Send the user through authorization again after expiry.
The default HTTP timeout is five minutes; use `WithHTTPClient` when your
backend needs a different deadline.

The client refuses unexpected HTTP redirects and rejects malformed successful
responses before returning them to application code. API rejections are typed
as `*agentpass.APIError`; malformed success payloads wrap
`agentpass.ErrInvalidResponse`. Retry only safe failures with the same
idempotency key and identical request body.

See `examples/backend` for a direct request and `examples/oauth-backend` for a
complete local authorization, callback, inference, and receipt flow.

## Verify

```bash
go test ./...
go vet ./...
```

Build production applications with a currently supported, fully patched Go
release. Go embeds its standard library in your binary, so upgrading this module
alone cannot fix vulnerabilities in an outdated Go toolchain.
