package config

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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
	data, err := os.ReadFile(path) // #nosec G304 -- path is inside t.TempDir.
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
		{Name: "discord", Type: "discord", Enabled: true, Webhook: NewSecret("https://discord.com/api/webhooks/REPLACE_ME"), Events: []string{"claimed"}},
		{Name: "telegram", Type: "telegram", Enabled: true, BotToken: NewSecret("replace-me"), ChatID: NewSecret("chat"), Events: []string{"claimed"}},
		{Name: "ntfy", Type: "ntfy", Enabled: true, Server: "https://ntfy.sh", Topic: "replace_me", Events: []string{"claimed"}},
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

func TestEnabledNotificationTargetsRequireUniqueEvents(t *testing.T) {
	target := NotificationTarget{
		Name: "ntfy", Type: "ntfy", Enabled: true,
		Server: "https://ntfy.sh", Topic: "alerts",
	}
	if err := ValidateTarget(target); err == nil || !strings.Contains(err.Error(), "at least one event") {
		t.Fatalf("enabled target without events was accepted: %v", err)
	}
	target.Events = []string{"error", "error"}
	if err := ValidateTarget(target); err == nil || !strings.Contains(err.Error(), "duplicate event") {
		t.Fatalf("duplicate target event was accepted: %v", err)
	}
}

func validConfig() Config {
	cfg := defaults()
	cfg.Version = 1
	cfg.Accounts = []Account{{Name: "main", Enabled: true, Credential: NewSecret("credential"), GameRole: NewSecret("role")}}
	return cfg
}

func TestDurationRejectsMalformedAndNegativeValues(t *testing.T) {
	tests := []struct {
		value string
		want  time.Duration
		err   bool
	}{{"not-a-duration", 0, true}, {"-1s", 0, true}, {"250ms", 250 * time.Millisecond, false}}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			var duration Duration
			err := duration.UnmarshalText([]byte(test.value))
			if (err != nil) != test.err || (!test.err && duration.Duration != test.want) {
				t.Fatalf("UnmarshalText(%q) duration=%s err=%v", test.value, duration.Duration, err)
			}
		})
	}
}

func TestValidateRejectsInvalidCoreAndAccountFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"version", func(c *Config) { c.Version = 2 }, "unsupported config version"},
		{"app language", func(c *Config) { c.App.Language = "en US" }, "app.language"},
		{"log level", func(c *Config) { c.App.LogLevel = "trace" }, "app.log_level"},
		{"request timeout", func(c *Config) { c.Run.RequestTimeout.Duration = 0 }, "request_timeout"},
		{"empty name", func(c *Config) { c.Accounts[0].Name = " " }, "must not be empty"},
		{"account language", func(c *Config) { c.Accounts[0].Language = "bad language" }, "invalid language"},
		{"game role header", func(c *Config) { c.Accounts[0].GameRole = NewSecret("role\nheader") }, "control character"},
		{"placeholder credential", func(c *Config) { c.Accounts[0].Credential = NewSecret("YOUR_CRED") }, "placeholder"},
		{"no enabled account", func(c *Config) { c.Accounts[0].Enabled = false }, "at least one account"},
		{"empty target name", func(c *Config) {
			c.Notifications.Targets = []NotificationTarget{{Type: "ntfy", Server: "https://ntfy.sh", Topic: "alerts"}}
		}, "must not be empty"},
		{"invalid target", func(c *Config) { c.Notifications.Targets = []NotificationTarget{{Name: "bad", Type: "email"}} }, "unsupported type"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validConfig()
			test.mutate(&cfg)
			if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q, got %v", test.want, err)
			}
		})
	}
}

func TestValidateTargetFailuresAndSupportedTargets(t *testing.T) {
	failures := []struct {
		name   string
		target NotificationTarget
	}{
		{"event", NotificationTarget{Type: "ntfy", Server: "https://ntfy.sh", Topic: "alerts", Events: []string{"unknown"}}},
		{"discord scheme", NotificationTarget{Type: "discord", Webhook: NewSecret("http://discord.com/api/webhooks/1/x")}},
		{"discord host", NotificationTarget{Type: "discord", Webhook: NewSecret("https://example.com/api/webhooks/1/x")}},
		{"telegram token", NotificationTarget{Type: "telegram", ChatID: NewSecret("chat")}},
		{"ntfy scheme", NotificationTarget{Type: "ntfy", Server: "http://ntfy.sh", Topic: "alerts"}},
		{"ntfy topic", NotificationTarget{Type: "ntfy", Server: "https://ntfy.sh", Topic: "bad topic"}},
		{"unsupported", NotificationTarget{Type: "email"}},
	}
	for _, test := range failures {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateTarget(test.target); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	for _, target := range []NotificationTarget{
		{Type: "discord", Webhook: NewSecret("https://canary.discord.com/api/webhooks/1/token")},
		{Type: "telegram", BotToken: NewSecret("token"), ChatID: NewSecret("chat")},
		{Type: "ntfy", Server: "https://ntfy.sh", Topic: "alerts"},
	} {
		if err := ValidateTarget(target); err != nil {
			t.Fatalf("valid %s target: %v", target.Type, err)
		}
	}
}

func TestPathResolutionAndAccountSelectionFailures(t *testing.T) {
	relative := filepath.Join("relative", "config.toml")
	resolved, err := ResolvePath(relative)
	if err != nil || !filepath.IsAbs(resolved) {
		t.Fatalf("ResolvePath()=%q, %v", resolved, err)
	}
	homeErr := errors.New("home unavailable")
	for _, goos := range []string{"linux", "darwin"} {
		if _, err := defaultConfigDir(goos, func(string) string { return "" }, func() (string, error) { return "", homeErr }); !errors.Is(err, homeErr) {
			t.Fatalf("%s did not return home error: %v", goos, err)
		}
	}
	cfg := validConfig()
	cfg.Accounts = append(cfg.Accounts, Account{Name: "disabled"})
	accounts, err := EnabledAccounts(&cfg, "main")
	if err != nil || len(accounts) != 1 || accounts[0].Name != "main" {
		t.Fatalf("selected accounts=%v err=%v", accounts, err)
	}
	for _, selected := range []string{"missing", "disabled"} {
		if _, err := EnabledAccounts(&cfg, selected); err == nil {
			t.Fatalf("selected account %q should fail", selected)
		}
	}
	cfg.Accounts[0].Enabled = false
	if _, err := EnabledAccounts(&cfg, ""); err == nil {
		t.Fatal("empty enabled account set should fail")
	}
}

func TestDefaultPathAndCacheDirectory(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	t.Setenv("XDG_CACHE_HOME", base)
	t.Setenv("LOCALAPPDATA", base)
	t.Setenv("HOME", base)
	path, err := ResolvePath("")
	if err != nil || filepath.Base(path) != "config.toml" || !strings.Contains(path, AppDir) {
		t.Fatalf("default config path=%q err=%v", path, err)
	}
	cache, err := CacheDir()
	if err != nil || !strings.Contains(cache, AppDir) {
		t.Fatalf("cache directory=%q err=%v", cache, err)
	}
	if pathContainsPlaceholder("safe/path") {
		t.Fatal("safe path was treated as a placeholder")
	}
}

func TestLoadAndInitFilesystemFailures(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.toml")); err == nil {
		t.Fatal("missing configuration should fail")
	}
	dir := t.TempDir()
	if _, err := Load(dir); err == nil {
		t.Fatal("directory configuration should fail")
	}
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(filepath.Join(blocker, "config.toml"), false); err == nil {
		t.Fatal("initialization below a regular file should fail")
	}
}

func TestSecretTextAndMaskBoundaries(t *testing.T) {
	var secret Secret
	if err := secret.UnmarshalText([]byte(" value ")); err != nil || secret.Expose() != " value " {
		t.Fatalf("UnmarshalText()=%q, %v", secret.Expose(), err)
	}
	if err := secret.UnmarshalTOML(42); err == nil {
		t.Fatal("non-string TOML secret should fail")
	}
	for value, want := range map[string]string{"": "<empty>", "abc": "***", "abcdef": "******", "abcdefg": "ab****fg"} {
		if got := Mask(value); got != want {
			t.Fatalf("Mask(%q)=%q want %q", value, got, want)
		}
	}
}
