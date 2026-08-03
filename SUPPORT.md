# Support

Use GitHub issues for reproducible SDK defects and documentation gaps. Include
the SDK version, Go version, expected behavior, actual behavior, and a minimal
reproduction that contains no credentials or user data.

AgentPass API incidents, account access, billing, and private security reports
should not be filed as public SDK issues. Follow the live AgentPass support and
security guidance for those cases.

The SDK is source-compatible with the Go version declared by `go.mod`. Security
support covers the two current Go release lines at their latest patch levels;
older toolchains may contain standard-library vulnerabilities even when the SDK
still compiles. The public API remains pre-1.0: patch releases are intended to
be backward-compatible, while a minor release may contain a documented
breaking change when necessary.
