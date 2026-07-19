package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/BurntSushi/toml"
)

const (
	AppDir         = "akef-skport-claim"
	MaxRandomDelay = 15 * time.Minute
)

type Duration struct{ time.Duration }

func (d *Duration) UnmarshalText(text []byte) error {
	value, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	if value < 0 {
		return errors.New("duration must not be negative")
	}
	d.Duration = value
	return nil
}

func (d Duration) String() string { return d.Duration.String() }

type Config struct {
	Version       int           `toml:"version"`
	App           AppConfig     `toml:"app"`
	Run           RunConfig     `toml:"run"`
	Accounts      []Account     `toml:"accounts"`
	Notifications Notifications `toml:"notifications"`
	Path          string        `toml:"-"`
}

type AppConfig struct {
	Language string `toml:"language"`
	LogLevel string `toml:"log_level"`
}

type RunConfig struct {
	RandomDelay               Duration `toml:"random_delay"`
	AccountDelay              Duration `toml:"account_delay"`
	RequestTimeout            Duration `toml:"request_timeout"`
	NotificationErrorCooldown Duration `toml:"notification_error_cooldown"`
}

type Account struct {
	Name       string `toml:"name"`
	Enabled    bool   `toml:"enabled"`
	Credential Secret `toml:"cred"`
	GameRole   Secret `toml:"game_role"`
	Language   string `toml:"language"`
}

type Notifications struct {
	Aggregate bool                 `toml:"aggregate"`
	Targets   []NotificationTarget `toml:"targets"`
}

type NotificationTarget struct {
	Name     string   `toml:"name"`
	Type     string   `toml:"type"`
	Enabled  bool     `toml:"enabled"`
	Webhook  Secret   `toml:"webhook"`
	BotToken Secret   `toml:"bot_token"`
	ChatID   Secret   `toml:"chat_id"`
	Server   string   `toml:"server"`
	Topic    string   `toml:"topic"`
	Token    Secret   `toml:"token"`
	Events   []string `toml:"events"`
}

func defaults() Config {
	return Config{
		Version: 0,
		App:     AppConfig{Language: "en", LogLevel: "info"},
		Run: RunConfig{
			RandomDelay: Duration{15 * time.Minute}, AccountDelay: Duration{3 * time.Second},
			RequestTimeout: Duration{20 * time.Second}, NotificationErrorCooldown: Duration{24 * time.Hour},
		},
		Notifications: Notifications{Aggregate: true},
	}
}

func Load(path string) (*Config, error) {
	resolved, err := ResolvePath(path)
	if err != nil {
		return nil, err
	}
	cfg := defaults()
	if err := CheckPermissions(resolved); err != nil {
		return nil, err
	}
	metadata, err := toml.DecodeFile(resolved, &cfg)
	if err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, key := range undecoded {
			keys[i] = key.String()
		}
		return nil, fmt.Errorf("unknown configuration keys: %s", strings.Join(keys, ", "))
	}
	cfg.Path = resolved
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func ResolvePath(path string) (string, error) {
	if path != "" {
		return filepath.Abs(path)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(base, AppDir, "config.toml"), nil
}

func CacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache directory: %w", err)
	}
	return filepath.Join(base, AppDir), nil
}

var languagePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var topicPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func (c *Config) Validate() error {
	if c.Version != 1 {
		return fmt.Errorf("unsupported config version %d", c.Version)
	}
	if !languagePattern.MatchString(c.App.Language) {
		return errors.New("app.language must contain only ASCII letters, digits, '-' or '_'")
	}
	if !slices.Contains([]string{"debug", "info", "warn", "error"}, c.App.LogLevel) {
		return errors.New("app.log_level must be debug, info, warn, or error")
	}
	if c.Run.RequestTimeout.Duration <= 0 {
		return errors.New("run.request_timeout must be greater than zero")
	}
	if c.Run.RandomDelay.Duration > MaxRandomDelay {
		return fmt.Errorf("run.random_delay must not exceed %s", MaxRandomDelay)
	}
	seenAccounts := map[string]bool{}
	enabled := 0
	for i := range c.Accounts {
		account := &c.Accounts[i]
		account.Name = strings.TrimSpace(account.Name)
		account.Credential.trimSpace()
		account.GameRole.trimSpace()
		if err := validateDisplayName(account.Name); err != nil {
			return fmt.Errorf("accounts[%d].name %w", i, err)
		}
		if seenAccounts[account.Name] {
			return fmt.Errorf("duplicate account name %q", account.Name)
		}
		seenAccounts[account.Name] = true
		if account.Language == "" {
			account.Language = c.App.Language
		}
		if !languagePattern.MatchString(account.Language) {
			return fmt.Errorf("account %q has invalid language", account.Name)
		}
		if account.Credential.Empty() || account.GameRole.Empty() {
			return fmt.Errorf("account %q requires cred and game_role", account.Name)
		}
		if err := validateRequestHeaderValue("cred", account.Credential.Expose()); err != nil {
			return fmt.Errorf("account %q: %w", account.Name, err)
		}
		if err := validateRequestHeaderValue("game_role", account.GameRole.Expose()); err != nil {
			return fmt.Errorf("account %q: %w", account.Name, err)
		}
		if account.Enabled {
			enabled++
			if isPlaceholder(account.Credential.Expose()) || isPlaceholder(account.GameRole.Expose()) {
				return fmt.Errorf("account %q still contains placeholder credentials", account.Name)
			}
		}
	}
	if enabled == 0 {
		return errors.New("at least one account must be enabled")
	}
	seenTargets := map[string]bool{}
	for i := range c.Notifications.Targets {
		target := &c.Notifications.Targets[i]
		target.Name = strings.TrimSpace(target.Name)
		if err := validateDisplayName(target.Name); err != nil {
			return fmt.Errorf("notifications.targets[%d].name %w", i, err)
		}
		if seenTargets[target.Name] {
			return fmt.Errorf("duplicate notification target name %q", target.Name)
		}
		seenTargets[target.Name] = true
		if err := ValidateTarget(*target); err != nil {
			return fmt.Errorf("notification target %q: %w", target.Name, err)
		}
	}
	return nil
}

func validateDisplayName(value string) error {
	if value == "" {
		return errors.New("must not be empty")
	}
	for _, item := range value {
		if unicode.IsControl(item) {
			return errors.New("must not contain control characters")
		}
	}
	return nil
}

func validateRequestHeaderValue(name, value string) error {
	for _, item := range value {
		if item <= 0x1f || item == 0x7f {
			return fmt.Errorf("%s contains an invalid HTTP header control character", name)
		}
	}
	return nil
}

func isPlaceholder(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	return normalized == "replace-me" || normalized == "your-cred" || normalized == "your-game-role"
}

func ValidateTarget(target NotificationTarget) error {
	validEvent := func(event string) bool {
		return slices.Contains([]string{"claimed", "already_claimed", "unavailable", "auth_expired", "error"}, event)
	}
	for _, event := range target.Events {
		if !validEvent(event) {
			return fmt.Errorf("invalid event %q", event)
		}
	}
	switch target.Type {
	case "discord":
		u, err := url.Parse(target.Webhook.Expose())
		if err != nil || u.Scheme != "https" || !slices.Contains([]string{"discord.com", "discordapp.com", "canary.discord.com", "ptb.discord.com"}, strings.ToLower(u.Hostname())) || !strings.HasPrefix(u.Path, "/api/webhooks/") {
			return errors.New("webhook must be an official HTTPS Discord webhook URL")
		}
		if target.Enabled && pathContainsPlaceholder(u.Path) {
			return errors.New("webhook still contains a placeholder")
		}
	case "telegram":
		if target.BotToken.Empty() || target.ChatID.Empty() {
			return errors.New("bot_token and chat_id are required")
		}
		if target.Enabled && (isPlaceholder(target.BotToken.Expose()) || isPlaceholder(target.ChatID.Expose())) {
			return errors.New("bot_token and chat_id placeholders must be replaced")
		}
	case "ntfy":
		u, err := url.Parse(target.Server)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return errors.New("server must be an HTTPS URL")
		}
		if !topicPattern.MatchString(target.Topic) {
			return errors.New("topic must contain only ASCII letters, digits, '-' or '_'")
		}
		if target.Enabled && isPlaceholder(target.Topic) {
			return errors.New("topic placeholder must be replaced")
		}
	default:
		return fmt.Errorf("unsupported type %q", target.Type)
	}
	return nil
}

func pathContainsPlaceholder(value string) bool {
	for segment := range strings.SplitSeq(strings.Trim(value, "/"), "/") {
		if isPlaceholder(segment) {
			return true
		}
	}
	return false
}

func EnabledAccounts(c *Config, selected string) ([]Account, error) {
	accounts := make([]Account, 0, len(c.Accounts))
	for _, account := range c.Accounts {
		if account.Enabled && (selected == "" || account.Name == selected) {
			accounts = append(accounts, account)
		}
	}
	if len(accounts) == 0 {
		if selected != "" {
			return nil, fmt.Errorf("enabled account %q not found", selected)
		}
		return nil, errors.New("no enabled accounts")
	}
	return accounts, nil
}
