package config

import (
	"log/slog"
	"testing"
)

func TestMask(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"empty", "", "<empty>"},
		{"short", "abc", "***"},
		{"six chars", "abcdef", "******"},
		{"long", "my-secret-token-here", "my****re"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Mask(tt.value); got != tt.want {
				t.Errorf("Mask(%q) = %q; want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestSecret_Expose(t *testing.T) {
	s := NewSecret("plaintext")
	if got := s.Expose(); got != "plaintext" {
		t.Errorf("Expose() = %q; want 'plaintext'", got)
	}
}

func TestSecret_Empty(t *testing.T) {
	if !NewSecret("").Empty() {
		t.Error("Empty() = false; want true for empty secret")
	}
	if NewSecret("value").Empty() {
		t.Error("Empty() = true; want false for non-empty secret")
	}
}

func TestSecret_String(t *testing.T) {
	s := NewSecret("my-token")
	if got := s.String(); got == "my-token" {
		t.Error("String() exposed secret value; should be masked")
	}
}

func TestSecret_LogValue(t *testing.T) {
	s := NewSecret("my-token")
	val := s.LogValue()
	if val.Kind() != slog.KindString || val.String() == "my-token" {
		t.Error("LogValue() should mask the secret")
	}
}

func TestSecret_MarshalText(t *testing.T) {
	s := NewSecret("my-token")
	data, err := s.MarshalText()
	if err != nil || string(data) == "my-token" {
		t.Error("MarshalText() should redact the secret")
	}
}

func TestSecret_UnmarshalText(t *testing.T) {
	var s Secret
	if err := s.UnmarshalText([]byte("hello")); err != nil {
		t.Fatalf("UnmarshalText() error: %v", err)
	}
	if s.Expose() != "hello" {
		t.Errorf("UnmarshalText() = %q; want 'hello'", s.Expose())
	}
}

func TestSecret_UnmarshalTOML(t *testing.T) {
	var s Secret
	if err := s.UnmarshalTOML("token"); err != nil {
		t.Fatalf("UnmarshalTOML() error: %v", err)
	}
	if s.Expose() != "token" {
		t.Errorf("UnmarshalTOML() = %q; want 'token'", s.Expose())
	}
	if err := s.UnmarshalTOML(42); err == nil {
		t.Error("UnmarshalTOML(42) should fail for non-string")
	}
}