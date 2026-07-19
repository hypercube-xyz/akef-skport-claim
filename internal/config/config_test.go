package config

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadStrictConfigAndDefaults(t *testing.T) {
	path := writeConfig(t, `version=1
[[accounts]]
name="main"
enabled=true
cred="example-credential-secret"
game_role="example-role-secret"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Run.RequestTimeout.String() != "20s" || cfg.Accounts[0].Language != "en" {
		t.Fatalf("defaults not applied: %#v", cfg)
	}
}

func TestRejectsMissingVersionUnknownKeysAndDuplicates(t *testing.T) {
	tests := []string{
		`[[accounts]]
name="main"
enabled=true
cred="example-x"
game_role="example-y"`,
		`version=1
unknown=true
[[accounts]]
name="main"
enabled=true
cred="example-x"
game_role="example-y"`,
		`version=1
[[accounts]]
name="same"
enabled=true
cred="example-x"
game_role="example-y"
[[accounts]]
name="same"
enabled=true
cred="example-x"
game_role="example-y"`,
	}
	for i, text := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if _, err := Load(writeConfig(t, text)); err == nil {
				t.Fatal("expected validation failure")
			}
		})
	}
}

func TestSecretNeverFormatsPlaintext(t *testing.T) {
	secret := NewSecret("abcdef123456")
	if got := fmt.Sprintf("%s %v %+v %q %x %#v", secret, secret, secret, secret, secret, secret); strings.Contains(got, "abcdef123456") || !strings.Contains(got, "ab****56") {
		t.Fatalf("unsafe formatting: %s", got)
	}
	if marshaled, err := secret.MarshalText(); err != nil || string(marshaled) != "<redacted>" {
		t.Fatalf("unsafe marshaling: %q, %v", marshaled, err)
	}
	var buffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buffer, nil))
	logger.Info("secret", "value", secret)
	if strings.Contains(buffer.String(), "abcdef123456") {
		t.Fatalf("slog leaked secret: %s", buffer.String())
	}
}

func TestValidateLanguage(t *testing.T) {
	cfg := defaults()
	cfg.Version = 1
	cfg.Accounts = []Account{{Name: "main", Enabled: true, Credential: NewSecret("x"), GameRole: NewSecret("y"), Language: "bad language"}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected language validation failure")
	}
}

func TestValidateRejectsWhitespaceOnlySecrets(t *testing.T) {
	cfg := defaults()
	cfg.Version = 1
	cfg.Accounts = []Account{{Name: "main", Enabled: true, Credential: NewSecret(" \t "), GameRole: NewSecret("role")}}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "requires cred") {
		t.Fatalf("expected empty credential error, got %v", err)
	}
}

func TestValidateTrimsCopiedHeaderValuesAndRejectsHeaderInjection(t *testing.T) {
	cfg := defaults()
	cfg.Version = 1
	cfg.Accounts = []Account{{
		Name:       "main",
		Enabled:    true,
		Credential: NewSecret("  credential-secret  "),
		GameRole:   NewSecret("\trole-secret\t"),
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.Accounts[0].Credential.Expose() != "credential-secret" || cfg.Accounts[0].GameRole.Expose() != "role-secret" {
		t.Fatalf("copied header values were not normalized: cred=%q role=%q", cfg.Accounts[0].Credential.Expose(), cfg.Accounts[0].GameRole.Expose())
	}

	cfg.Accounts[0].Credential = NewSecret("credential\r\nInjected: value")
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "control character") {
		t.Fatalf("expected header control-character rejection, got %v", err)
	}
}

func TestRejectsDuplicateNotificationNames(t *testing.T) {
	cfg := defaults()
	cfg.Version = 1
	cfg.Accounts = []Account{{Name: "main", Enabled: true, Credential: NewSecret("x"), GameRole: NewSecret("y")}}
	cfg.Notifications.Targets = []NotificationTarget{{Name: "same", Type: "telegram", BotToken: NewSecret("x"), ChatID: NewSecret("y")}, {Name: "same", Type: "ntfy", Server: "https://ntfy.sh", Topic: "safe"}}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate notification") {
		t.Fatalf("expected duplicate target error, got %v", err)
	}
}

func TestInitRefusesExistingAndForceReplacesAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	if _, err := Init(path, false); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(path, false); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected existing-file error, got %v", err)
	}
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(path, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != Example {
		t.Fatalf("force init did not replace contents: err=%v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("config mode: info=%v err=%v", info, err)
		}
	}
}

func TestDefaultConfigDirUsesNativePerUserLocations(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	localAppData := filepath.Join(home, "AppData", "Local")
	xdgConfig := filepath.Join(home, "xdg-config")
	tests := []struct {
		name     string
		goos     string
		env      map[string]string
		expected string
	}{
		{name: "windows", goos: "windows", env: map[string]string{"LOCALAPPDATA": localAppData}, expected: localAppData},
		{name: "linux-xdg", goos: "linux", env: map[string]string{"XDG_CONFIG_HOME": xdgConfig}, expected: xdgConfig},
		{name: "linux-default", goos: "linux", expected: filepath.Join(home, ".config")},
		{name: "macos", goos: "darwin", expected: filepath.Join(home, "Library", "Application Support")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := defaultConfigDir(test.goos, func(key string) string { return test.env[key] }, func() (string, error) { return home, nil })
			if err != nil || got != test.expected {
				t.Fatalf("default config directory: got=%q expected=%q err=%v", got, test.expected, err)
			}
		})
	}
}

func TestDefaultConfigDirRejectsMissingOrRelativeEnvironmentPaths(t *testing.T) {
	home := func() (string, error) { return filepath.Join(t.TempDir(), "home"), nil }
	if _, err := defaultConfigDir("windows", func(string) string { return "" }, home); err == nil {
		t.Fatal("missing LOCALAPPDATA was accepted")
	}
	if _, err := defaultConfigDir("linux", func(key string) string {
		if key == "XDG_CONFIG_HOME" {
			return "relative"
		}
		return ""
	}, home); err == nil {
		t.Fatal("relative XDG_CONFIG_HOME was accepted")
	}
}

func TestEmbeddedExampleMatchesRepositoryFile(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "config.example.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != Example {
		t.Fatal("config.Example and config.example.toml have drifted")
	}
}

func TestValidateRejectsControlCharactersAndExcessiveRandomDelay(t *testing.T) {
	cfg := defaults()
	cfg.Version = 1
	cfg.Accounts = []Account{{Name: "main\nforged", Enabled: true, Credential: NewSecret("x"), GameRole: NewSecret("y")}}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "control") {
		t.Fatalf("expected control-character error, got %v", err)
	}
	cfg.Accounts[0].Name = "main"
	cfg.Run.RandomDelay.Duration = MaxRandomDelay + 1
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "random_delay") {
		t.Fatalf("expected random delay error, got %v", err)
	}
}

func TestDecodeErrorsDoNotEchoSecretValues(t *testing.T) {
	for _, test := range []struct {
		text    string
		secrets []string
	}{
		{`version=1
[[accounts]]
name="main"
enabled=true
cred=123456789012345
game_role="example-role-secret-value"
`, []string{"123456789012345", "example-role-secret-value"}},
		{`version=1
[[accounts]]
name="main"
enabled=true
cred="unterminated-secret-value
`, []string{"unterminated-secret-value"}},
	} {
		_, err := Load(writeConfig(t, test.text))
		if err == nil {
			t.Fatal("expected decode error")
		}
		for _, secret := range test.secrets {
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("decode error exposed config value: %v", err)
			}
		}
	}
}

func TestEnabledNotificationTargetsRejectPlaceholders(t *testing.T) {
	for _, target := range []NotificationTarget{
		{Name: "discord", Type: "discord", Enabled: true, Webhook: NewSecret("https://discord.com/api/webhooks/REPLACE_ME")},
		{Name: "telegram", Type: "telegram", Enabled: true, BotToken: NewSecret("replace-me"), ChatID: NewSecret("chat")},
		{Name: "ntfy", Type: "ntfy", Enabled: true, Server: "https://ntfy.sh", Topic: "replace_me"},
	} {
		if err := ValidateTarget(target); err == nil || !strings.Contains(err.Error(), "placeholder") {
			t.Fatalf("%s placeholder was accepted: %v", target.Type, err)
		}
		target.Enabled = false
		if err := ValidateTarget(target); err != nil {
			t.Fatalf("disabled %s placeholder should remain a valid template: %v", target.Type, err)
		}
	}
}
