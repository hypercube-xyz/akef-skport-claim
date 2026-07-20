package version

import "testing"

func TestStringIncludesBuildMetadata(t *testing.T) {
	oldVersion, oldCommit, oldDate := Version, Commit, Date
	t.Cleanup(func() { Version, Commit, Date = oldVersion, oldCommit, oldDate })
	Version, Commit, Date = "v1.0.0", "abc123", "2026-07-20"
	if got, want := String(), "v1.0.0 (commit abc123, built 2026-07-20)"; got != want {
		t.Fatalf("String()=%q want %q", got, want)
	}
}
