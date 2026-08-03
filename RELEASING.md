# Releasing

The public module is `github.com/jianqi-zhao/agentpass-go`. Every release is an
immutable semantic-version tag in the dedicated public repository.

1. Update `Version` in `client.go` and the documentation.
2. Run `go mod tidy`, `go test -race ./...`, and `go vet ./...`.
3. Sync the SDK repository with `make sdk/publish SDK_VERSION=v0.x.y` from the
   AgentPass platform workspace.
4. The sync command pushes `main` and dispatches the `Publish Go SDK` workflow.
5. The workflow re-runs verification, creates the tag and GitHub release, and
   confirms that the version resolves through `proxy.golang.org`.

Tags are never moved or overwritten. Fixes always receive a new version.
