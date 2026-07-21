package version

import "testing"

func TestString(t *testing.T) {
	Version, Commit, Date = "1.0.0", "abc1234", "2025-01-15"
	got := String()
	if got != "1.0.0 (commit abc1234, built 2025-01-15)" {
		t.Errorf("String() = %q; want value with commit and date", got)
	}
}