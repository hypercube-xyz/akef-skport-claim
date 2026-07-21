package skport

import "testing"

func TestHeaderJSON(t *testing.T) {
	got := HeaderJSON("3", "1712345678", "1.0.0")
	if got != `{"platform":"3","timestamp":"1712345678","dId":"","vName":"1.0.0"}` {
		t.Errorf("HeaderJSON() = %q; want JSON with platform/timestamp/vName", got)
	}
}

func TestGenerateSign(t *testing.T) {
	// Deterministic: same inputs produce same output.
	a := GenerateSign("/path", "{}", "1712345678", "test-token", "3", "1.0.0")
	b := GenerateSign("/path", "{}", "1712345678", "test-token", "3", "1.0.0")
	if a != b {
		t.Errorf("GenerateSign() not deterministic: %q vs %q", a, b)
	}
	if len(a) != 32 {
		t.Errorf("GenerateSign() = %q; want 32-char hex (MD5)", a)
	}

	// Different input produces different output.
	c := GenerateSign("/other", "{}", "1712345678", "test-token", "3", "1.0.0")
	if a == c {
		t.Error("GenerateSign() with different path should produce different signature")
	}
}