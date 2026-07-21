package config

import (
	"testing"
	"time"
)

func TestDuration_UnmarshalText(t *testing.T) {
	var d Duration
	if err := d.UnmarshalText([]byte("5m")); err != nil {
		t.Fatalf("UnmarshalText('5m') error: %v", err)
	}
	if d.Duration != 5*time.Minute {
		t.Errorf("UnmarshalText('5m') = %v; want 5m", d.Duration)
	}
}

func TestDuration_UnmarshalText_Negative(t *testing.T) {
	var d Duration
	if err := d.UnmarshalText([]byte("-1s")); err == nil {
		t.Error("UnmarshalText('-1s') should fail")
	}
}

func TestDefaults(t *testing.T) {
	cfg := defaults()
	if cfg.Version != 0 {
		t.Errorf("defaults().Version = %d; want 0", cfg.Version)
	}
	if cfg.App.Language != "en" {
		t.Errorf("defaults().App.Language = %q; want 'en'", cfg.App.Language)
	}
	if cfg.Notifications.Aggregate != true {
		t.Error("defaults().Notifications.Aggregate = false; want true")
	}
	if cfg.Run.RandomDelay.Duration != 15*time.Minute {
		t.Errorf("defaults().Run.RandomDelay = %v; want 15m", cfg.Run.RandomDelay.Duration)
	}
}

func TestValidate_Version(t *testing.T) {
	cfg := defaults()
	cfg.Version = 2
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail for unsupported version")
	}
}

func TestValidate_LogLevel(t *testing.T) {
	cfg := validConfig()
	cfg.App.LogLevel = "trace"
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail for invalid log level")
	}
}

func TestValidate_DuplicateAccount(t *testing.T) {
	cfg := validConfig()
	cfg.Accounts = []Account{
		{Name: "dup", Enabled: true, Credential: NewSecret("c1"), GameRole: NewSecret("g1"), Language: "en"},
		{Name: "dup", Enabled: true, Credential: NewSecret("c2"), GameRole: NewSecret("g2"), Language: "en"},
	}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail for duplicate account names")
	}
}

func TestValidate_NoEnabledAccounts(t *testing.T) {
	cfg := validConfig()
	cfg.Accounts = []Account{{Name: "disabled", Enabled: false, Credential: NewSecret("c"), GameRole: NewSecret("g"), Language: "en"}}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail when no accounts are enabled")
	}
}

func TestValidate_Placeholder(t *testing.T) {
	cfg := validConfig()
	cfg.Accounts = []Account{{Name: "main", Enabled: true, Credential: NewSecret("replace-me"), GameRole: NewSecret("real"), Language: "en"}}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail for placeholder cred")
	}
}

func TestValidateTarget_InvalidType(t *testing.T) {
	target := NotificationTarget{Name: "test", Type: "unknown", Enabled: false}
	if err := ValidateTarget(target); err == nil {
		t.Error("ValidateTarget() should fail for unsupported type")
	}
}

func TestValidateTarget_InvalidEvent(t *testing.T) {
	target := NotificationTarget{Name: "test", Type: "discord", Enabled: true, Webhook: NewSecret("https://discord.com/api/webhooks/123/abc"), Events: []string{"invalid"}}
	if err := ValidateTarget(target); err == nil {
		t.Error("ValidateTarget() should fail for invalid event")
	}
}

func TestValidateTarget_NoEvents(t *testing.T) {
	target := NotificationTarget{Name: "test", Type: "discord", Enabled: true, Webhook: NewSecret("https://discord.com/api/webhooks/123/abc"), Events: nil}
	if err := ValidateTarget(target); err == nil {
		t.Error("ValidateTarget() should fail when enabled with no events")
	}
}

func TestValidateTarget_DuplicateEvent(t *testing.T) {
	target := NotificationTarget{Name: "test", Type: "ntfy", Enabled: false, Server: "https://ntfy.sh", Topic: "test", Events: []string{"claimed", "claimed"}}
	if err := ValidateTarget(target); err == nil {
		t.Error("ValidateTarget() should fail for duplicate events")
	}
}

func TestValidateTarget_DiscordBadURL(t *testing.T) {
	target := NotificationTarget{Name: "test", Type: "discord", Enabled: false, Webhook: NewSecret("http://bad.com"), Events: []string{"claimed"}}
	if err := ValidateTarget(target); err == nil {
		t.Error("ValidateTarget() should fail for non-HTTPS discord webhook")
	}
}

func TestValidateTarget_TelegramMissing(t *testing.T) {
	target := NotificationTarget{Name: "test", Type: "telegram", Enabled: false, Events: []string{"claimed"}}
	if err := ValidateTarget(target); err == nil {
		t.Error("ValidateTarget() should fail for telegram without bot_token")
	}
}

func TestValidateTarget_NtfyBadURL(t *testing.T) {
	target := NotificationTarget{Name: "test", Type: "ntfy", Enabled: false, Server: "http://ntfy.sh", Topic: "test", Events: []string{"claimed"}}
	if err := ValidateTarget(target); err == nil {
		t.Error("ValidateTarget() should fail for non-HTTPS ntfy server")
	}
}

func TestIsPlaceholder(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"replace-me", true},
		{"REPLACE-ME", true},
		{"replace_me", true},
		{"your-cred", true},
		{"your-game-role", true},
		{"real-token", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isPlaceholder(tt.value); got != tt.want {
			t.Errorf("isPlaceholder(%q) = %v; want %v", tt.value, got, tt.want)
		}
	}
}

func TestDefaultConfigDir(t *testing.T) {
	// Darwin (macOS).
	home := func() (string, error) { return "/Users/test", nil }
	got, err := defaultConfigDir("darwin", nil, home)
	if err != nil {
		t.Fatalf("defaultConfigDir(darwin) error: %v", err)
	}
	if got != "/Users/test/Library/Application Support" {
		t.Errorf("defaultConfigDir(darwin) = %q; want .../Library/Application Support", got)
	}

	// Linux with XDG_CONFIG_HOME.
	getenv := func(key string) string {
		if key == "XDG_CONFIG_HOME" {
			return "/custom/config"
		}
		return ""
	}
	got, err = defaultConfigDir("linux", getenv, home)
	if err != nil {
		t.Fatalf("defaultConfigDir(linux) error: %v", err)
	}
	if got != "/custom/config" {
		t.Errorf("defaultConfigDir(linux) = %q; want /custom/config", got)
	}

	// Linux without XDG_CONFIG_HOME.
	got, err = defaultConfigDir("linux", func(string) string { return "" }, home)
	if err != nil {
		t.Fatalf("defaultConfigDir(linux, no XDG) error: %v", err)
	}
	if got != "/Users/test/.config" {
		t.Errorf("defaultConfigDir(linux, no XDG) = %q; want .../.config", got)
	}
}

func TestEnabledAccounts(t *testing.T) {
	cfg := &Config{
		Accounts: []Account{
			{Name: "main", Enabled: true},
			{Name: "alt", Enabled: true},
			{Name: "off", Enabled: false},
		},
	}
	all, err := EnabledAccounts(cfg, "")
	if err != nil || len(all) != 2 {
		t.Errorf("EnabledAccounts(all) = %d, %v; want 2", len(all), err)
	}
	one, err := EnabledAccounts(cfg, "main")
	if err != nil || len(one) != 1 || one[0].Name != "main" {
		t.Errorf("EnabledAccounts(main) = %+v, %v; want 1", one, err)
	}
	_, err = EnabledAccounts(cfg, "missing")
	if err == nil {
		t.Error("EnabledAccounts(missing) should fail")
	}
}

func TestValidateDisplayName(t *testing.T) {
	if err := validateDisplayName(""); err == nil {
		t.Error("validateDisplayName('') should fail")
	}
	if err := validateDisplayName("ok"); err != nil {
		t.Errorf("validateDisplayName('ok') error: %v", err)
	}
	if err := validateDisplayName("bad\x00char"); err == nil {
		t.Error("validateDisplayName with control char should fail")
	}
}

func validConfig() Config {
	cfg := defaults()
	cfg.Version = 1
	cfg.Accounts = []Account{{Name: "main", Enabled: true, Credential: NewSecret("cred-value"), GameRole: NewSecret("role-value"), Language: "en"}}
	return cfg
}

func TestLoad_Fixture(t *testing.T) {
	cfg, err := Load("testdata/valid.toml")
	if err != nil {
		t.Fatalf("Load(valid.toml) error: %v", err)
	}
	if cfg.Version != 1 || cfg.App.Language != "en" {
		t.Errorf("Version=%d Language=%q; want 1/en", cfg.Version, cfg.App.Language)
	}
	accounts, err := EnabledAccounts(cfg, "")
	if err != nil || len(accounts) != 1 || accounts[0].Name != "main" {
		t.Errorf("enabled accounts = %d/%v; want 1/main", len(accounts), err)
	}
}