# AgentPass Go SDK

The AgentPass Go SDK is a dependency-free backend helper for:

- building user authorization URLs;
- confidential authorization-code exchange and refresh-token rotation;
- verified AgentPass identity lookup for `identity.basic` grants;
- provider-native OpenAI and Anthropic base URLs;
- grant-scoped weekly and monthly usage summaries;
- backend-issued, one-time hosted usage URLs for iOS `WKWebView`;
- redirect-safe provider HTTP clients and stable idempotency keys.

The SDK does not meter locally. AgentPass reserves and settles credits in its
transactional ledger, then returns the authoritative receipt.
Credits use one public denomination throughout AgentPass and support up to two
decimal places. For example, `0.25` means one quarter of one credit; SDK values
are never scaled or converted to another unit.

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
providerHTTPClient := agentpass.ProviderHTTPClient(http.DefaultClient)
// Configure the official OpenAI Go client with baseURL and the connected
// user's AgentPass access token, and use providerHTTPClient as its HTTP client.
// Its Responses request/response types remain unchanged. Use a stable
// Idempotency-Key and X-AgentPass-Max-Credits header.
```

For Anthropic clients, use `agentpass.AnthropicBaseURL(...)`. The returned roots
are `.../openai/v1` and `.../anthropic`, respectively. AgentPass preserves
provider-native request and response bodies and returns settlement metadata in
`X-AgentPass-*` headers.

The currently metered provider-native contract accepts body-bounded text and
local function/custom tools. It rejects media, files, provider-hosted tools,
externally referenced prompt/conversation/context objects, and separately
priced service or reasoning modes before reserving credits or dispatching
provider work. OpenAI `previous_response_id` remains supported only when it is
an AgentPass response ID owned by the same grant; never pass an upstream ID.
Native JSON bodies may be at most 2 MiB. Set OpenAI `max_output_tokens` or
Anthropic `max_tokens` between 1 and 8,192; OpenAI uses 4,096 when omitted.
Log-probability output and hosted-result `include` selectors are outside this
bounded response contract.

Treat only `queued` and `in_progress` OpenAI responses as pollable, and poll
their response ID instead of submitting another POST. `completed` and
`incomplete` are terminal, billable responses and must include all four
AgentPass settlement headers. `failed` is terminal and no-charge; surface its
bounded OpenAI error details, then use a new idempotency key only if the user or
application starts a new logical attempt. Do not loop on a failed response or
reuse its key for a different attempt.

## OAuth

Use `client.OAuth.AuthorizationURL` to request `ai.inference` plus explicit
provider-qualified model keys, create the consent URL, and then use
`client.OAuth.ExchangeAuthorizationCode` from the backend callback. Never send
the client secret to a browser or mobile application. Store the returned opaque
access and refresh tokens encrypted in application-owned storage.

AgentPass access tokens expire after one hour. Store refresh tokens encrypted
and rotate them with `client.OAuth.RefreshAccessToken`. Before sending, persist
a unique 1–200 character `RefreshTokenParams.IdempotencyKey` with the old token.
If the response is lost, retry the exact old token and key within five minutes;
AgentPass returns the already-committed pair. A different key, an expired
recovery window, or reuse after the replacement was itself used is treated as
compromise and revokes that user's application connection. Discard the token
pair and ask the user to reconnect; reconnecting does not reset the user's
weekly or monthly usage for your application.

For an operator-approved native account flow, the mobile app submits account
credentials directly to AgentPass and gives your backend only the returned
one-time code and state. After confidential code exchange, configure a client
with the access token and call `client.Identity.UserInfo(ctx)`. Key your local
account by `UserInfo.Subject`, then issue your own opaque application session.
Never proxy the user's AgentPass password through the backend or return the
AgentPass token pair to mobile code. During native registration, the app asks
the user to re-enter their registration password after the email link returns,
then sends that password and the client-bound verification token directly to
AgentPass. The Go backend is not involved in that password-proof step. Treat
registration acceptance as non-enumerating; it does not prove that the email is
new, pending, or eligible.

Operator approval is a password-handling trust decision: native UI code can see
what the user types and must never retain, log, or proxy it. Verification and
recovery links carry their action and client-bound bearer token in the URL
fragment, not the query. Parse the fragment locally, remove it from the visible
URL immediately, and never send it to the Go backend, analytics, or crash logs.

Use `client.Usage.Current(ctx)` with the connected user's access token to read
the app grant's calendar-week and calendar-month limits, settled usage,
in-flight reservations, and remaining credits. Return only the display fields
your frontend needs; never send the AgentPass access token to frontend code.

To use AgentPass's ready-made mobile UI, call
`client.Embed.CreateUsageView(ctx, agentpass.EmbedThemeSystem)` from the
backend and return only its `EmbedURL` to the currently signed-in iOS user.
Load that URL directly in a `WKWebView`. The URL code expires after two minutes,
works once, and opens a read-only fifteen-minute session bound to the same app
grant. Do not persist, log, prefetch, or send analytics containing the URL.

For AI calls, use the official provider SDK and replace only its base URL
and credential. `agentpass.OpenAIBaseURL(agentpass.DefaultBaseURL)` returns the
OpenAI-compatible root, while `agentpass.AnthropicBaseURL(...)` returns the
Anthropic-compatible root. Provider request and response bodies remain native;
AgentPass settlement is returned in `X-AgentPass-*` headers.
The default HTTP timeout is sixteen minutes, slightly longer than AgentPass's
default synchronous provider window. Use `WithHTTPClient` or a context deadline
when your backend needs a different limit.

Configure official OpenAI or Anthropic clients with
`agentpass.ProviderHTTPClient(yourHTTPClient)`. It clones the supplied client,
preserves its transport and timeout, and refuses redirects so an AgentPass user
token and provider request body cannot be forwarded to a redirect-selected
host. This safeguard applies to provider-native clients; `NewClient` already
enforces the same redirect policy for AgentPass SDK methods.

The client refuses unexpected HTTP redirects and rejects malformed successful
responses before returning them to application code. API rejections are typed
as `*agentpass.APIError`; malformed success payloads wrap
`agentpass.ErrInvalidResponse`.

See `examples/backend` for a native OpenAI Responses HTTP request and
`examples/oauth-backend` for a complete local authorization, callback,
provider-native inference, and settlement-header flow.

## Migrating to v0.8.0

The unused capability-based inference API has been retired. This is a breaking
pre-1.0 SDK release: `Client.Responses`, `ResponsesService`,
`CreateResponseParams`, `Response`, `Usage`, `Receipt`, and
`ResponseRequestError` have been removed. OAuth, identity, usage summaries,
embed sessions, base URLs, and safe transport helpers remain supported.

The old `POST /v1/responses` endpoint returns HTTP 410
`legacy_inference_removed` without dispatching work or charging credits.
Replace `client.Responses.Create` with a provider-native OpenAI Responses
client configured with `OpenAIBaseURL` and the user's AgentPass access token.
Request the `ai.inference` OAuth scope and choose an explicit model from the
catalog instead of `text.fast` or `text.smart`. Send the request credit ceiling
in `X-AgentPass-Max-Credits`, and read all four `X-AgentPass-*` settlement
headers as shown in the examples.

Generate an idempotency key with `NewIdempotencyKey` and persist it **before**
dispatching a logical AI job. After an ambiguous timeout, reuse that key and
the identical request; do not create a new key for the same work. The SDK no
longer generates a hidden key inside an inference call.

Historical ledger records and settlement receipts are retained unchanged.

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
