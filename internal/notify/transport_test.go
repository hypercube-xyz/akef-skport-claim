package notify

import "testing"

func TestTruncateUTF8(t *testing.T) {
	tests := []struct {
		name  string
		value string
		limit int
		want  string
	}{
		{"no truncation", "hello", 10, "hello"},
		{"truncate", "hello world", 8, "hello…"},
		{"exact limit", "hello", 5, "hello"},
		{"zero limit", "hello", 0, ""},
		{"small limit", "hello", 2, "he"},
		{"very small limit", "hello", 1, "h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateUTF8(tt.value, tt.limit)
			if got != tt.want {
				t.Errorf("truncateUTF8(%q, %d) = %q; want %q", tt.value, tt.limit, got, tt.want)
			}
		})
	}
}