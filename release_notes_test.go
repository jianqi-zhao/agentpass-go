package agentpass_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseNotesRequireReviewedChangelogEntry(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "CHANGELOG.md")
	if err := os.WriteFile(fixture, []byte("# Changelog\n\n## v0.8.1 - 2026-09-06\n\n- Reviewed change.\n\n## v0.8.0 - 2026-09-05\n\n- Older change.\n\n## v0.7.0 - 2026-08-09\n\n"), 0600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("sh", "scripts/release-notes.sh", "v0.8.1", fixture).CombinedOutput()
	if err != nil || !strings.Contains(string(output), "Reviewed change.") || strings.Contains(string(output), "Older change.") {
		t.Fatalf("release notes did not isolate the reviewed version: %v: %s", err, output)
	}
	for _, version := range []string{"v0.9.0", "v0.7.0", "v0.8.1;echo unsafe", ""} {
		if err := exec.Command("sh", "scripts/release-notes.sh", version, fixture).Run(); err == nil {
			t.Fatalf("missing, empty, or invalid version %q must block publication", version)
		}
	}
}

func TestRefreshBreakingChangeAppearsInReleaseNotes(t *testing.T) {
	for _, version := range []string{"v0.8.0", "v0.8.1"} {
		output, err := exec.Command("sh", "scripts/release-notes.sh", version).CombinedOutput()
		if err != nil || !strings.Contains(string(output), "RefreshTokenParams.IdempotencyKey") || !strings.Contains(string(output), "v0.7.0") {
			t.Fatalf("%s must explain the required refresh upgrade: %v: %s", version, err, output)
		}
	}
}
