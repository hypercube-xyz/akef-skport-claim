package config

import (
	"errors"
	"log/slog"
	"strings"
)

type Secret struct {
	value string
}

func NewSecret(value string) Secret { return Secret{value: value} }

func (s *Secret) UnmarshalText(text []byte) error {
	s.value = string(text)
	return nil
}

func (s *Secret) UnmarshalTOML(value any) error {
	text, ok := value.(string)
	if !ok {
		return errors.New("secret value must be a TOML string")
	}
	s.value = text
	return nil
}

func (s Secret) Expose() string { return s.value }

func (s Secret) Empty() bool { return strings.TrimSpace(s.value) == "" }

func (s *Secret) trimSpace() { s.value = strings.TrimSpace(s.value) }

func (s Secret) String() string { return Mask(s.value) }

func (s Secret) GoString() string { return Mask(s.value) }

func (s Secret) LogValue() slog.Value { return slog.StringValue(Mask(s.value)) }

func (s Secret) MarshalText() ([]byte, error) { return []byte("<redacted>"), nil }

func Mask(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return "<empty>"
	}
	if len(runes) <= 6 {
		return string(makeMask(len(runes)))
	}
	return string(runes[:2]) + "****" + string(runes[len(runes)-2:])
}

func makeMask(length int) []rune {
	mask := make([]rune, length)
	for i := range mask {
		mask[i] = '*'
	}
	return mask
}
