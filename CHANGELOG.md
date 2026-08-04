# Changelog

All notable SDK changes are recorded here. The project follows semantic
versioning while its public API remains pre-1.0.

## Unreleased

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
