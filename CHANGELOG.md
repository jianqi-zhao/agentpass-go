# Changelog

All notable SDK changes are recorded here. The project follows semantic
versioning while its public API remains pre-1.0.

## v0.8.0 - 2026-09-05

- Publish and scan with patched Go 1.27.1; test both Go 1.26.8 and Go 1.27.1.
- Breaking: remove the retired `Client.Responses.Create` inference API, its
  request/result/receipt types, and `ResponseRequestError`. Use a provider-native
  client with explicit models; see the README migration guide.
- Preserve OAuth, identity, usage, embed, provider URL and HTTP-client helpers,
  and `NewIdempotencyKey`. The server's old endpoint now returns HTTP 410.

- Keep typed API error strings single-line and bounded even when an upstream
  error envelope is malformed.
- Keep redirect refusal as an SDK credential-safety invariant even when callers
  supply a permissive `http.Client`; expose the same cloned, redirect-refusing
  client configuration for official provider SDKs and examples so AgentPass
  tokens and request bodies cannot be forwarded to redirect-selected hosts.
  Also accept IPv6 loopback OAuth callbacks and reject unexpectedly large OAuth
  bearer values.
- Require provider-native examples to validate the OpenAI response object and
  the syntax and ordering of all settlement credit headers before accepting a
  billable result.
- Make both runnable examples share one bounded OpenAI response and settlement-
  receipt parser, reject bodies that exceed the 4 MiB limit by one byte, and use
  structured validation in tests.
- Match the server's fail-closed OAuth contract by rejecting state values over
  512 characters.
- Document that native email verification requires confirmation-time password
  proof sent directly from the mobile app to AgentPass; the Go backend never
  receives that password.
- Increase the default client and example request deadline from five to
  sixteen minutes so it exceeds AgentPass's default synchronous provider wait.
- Make both provider-native OpenAI examples poll only `queued` and
  `in_progress`, require settlement headers for terminal `completed` and
  `incomplete` responses, and report terminal `failed` responses as no-charge
  work that needs a new logical attempt and idempotency key. Provider error
  details are decoded, bounded, and stripped of control characters before
  display.

## v0.7.0 - 2026-08-09

- Add `Client.Identity.UserInfo` so approved native-sign-in integrations can
  resolve the verified AgentPass account after confidential code exchange.
- Document the direct mobile registration, verification, recovery, and native
  Google authorization flow while keeping client secrets and tokens backend-only.

## v0.6.0 - 2026-08-04

- Add `Client.Embed.CreateUsageView` for backend-issued, one-time hosted usage
  URLs suitable for an iOS `WKWebView`.

## v0.5.0 - 2026-08-04

- Use the single public credit denomination for OAuth limits, usage summaries,
  request ceilings, and receipts, with up to two decimal places.
- Add reserved credits to the typed legacy settlement receipt.

## v0.4.0 - 2026-08-04

- Add `Client.Usage.Current` for grant-scoped calendar-week and calendar-month
  credit usage.
- Add an optional weekly limit to OAuth authorization URLs.

## v0.3.0 - 2026-08-04

- Add provider-native OpenAI and Anthropic base URL helpers.
- Add per-model OAuth consent parameters and default-model selection.
- Return rotating refresh tokens and add confidential refresh-token exchange.
- Update both examples to use the OpenAI-compatible Responses façade and
  AgentPass settlement headers.
- Retain the earlier typed capability client as a compatibility API.

## v0.2.1 - 2026-08-03

- License the public SDK under Apache-2.0.
- Reject insecure remote HTTP endpoints and credential-bearing base URLs.
- Validate successful OAuth tokens, AI responses, receipts, token usage, and
  settlement timestamps before returning them to application code.
- Reject malformed credential headers, idempotency keys, redirect URIs,
  capabilities, request limits, and nil token-source functions.
- Verify OAuth state before handling authorization denial in the runnable
  example and integration guide.
- Make release workflow retries safe after an immutable tag has been created.
- Pin GitHub Actions by commit and add automated action update checks.
- Test both supported Go release lines, scan with `govulncheck`, and add
  adversarial protocol tests and fuzz targets.

## v0.2.0 - 2026-08-03

- Publish the SDK as `github.com/jianqi-zhao/agentpass-go`.
- Add normalized token usage and complete OAuth backend example.
- Add release automation and public Go module verification.

## v0.1.0 - 2026-08-03

- Initial backend SDK with OAuth code exchange, metered response creation,
  idempotency keys, typed API errors, and settled receipts.
