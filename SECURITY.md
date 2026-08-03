# Security policy

## Reporting a vulnerability

Please do not open a public issue for a suspected vulnerability. Use GitHub's
private vulnerability reporting for `jianqi-zhao/agentpass-go` instead:

1. Open the repository's **Security** tab.
2. Select **Report a vulnerability**.
3. Include the affected SDK version, impact, reproduction steps, and any known
   mitigation.

Do not include real AgentPass access tokens, OAuth codes, client secrets,
prompts, or user data in the report. We will acknowledge a complete report
within three business days and coordinate disclosure after a fix is available.

## Supported versions

Security fixes are made on the latest released minor line. Because the SDK is
currently pre-1.0, applications should stay on the newest patch release and pin
the selected version in `go.mod`.

The module remains source-compatible with Go 1.22, but Go embeds its standard
library into the application binary. Production applications must build with a
currently supported, fully patched Go release. SDK CI tests the latest patch of
both supported Go release lines and runs `govulncheck` on the newest line.
