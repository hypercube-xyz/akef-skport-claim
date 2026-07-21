package config

import (
	"fmt"
	"os"
)

func Init(path string, force bool) (string, error) {
	resolved, err := ResolvePath(path)
	if err != nil {
		return "", err
	}
	if force {
		if err := os.WriteFile(resolved, []byte(Example), 0o600); err != nil {
			return "", fmt.Errorf("initialize config: %w", err)
		}
		return resolved, nil
	}

	if _, err := os.Stat(resolved); err == nil {
		return "", fmt.Errorf("config already exists at %s (use --force to replace it)", resolved)
	}
	if err := os.WriteFile(resolved, []byte(Example), 0o600); err != nil {
		return "", fmt.Errorf("initialize config: %w", err)
	}
	return resolved, nil
}

const Example = `version = 1

[app]
language = "en"
log_level = "info"

[run]
random_delay = "15m"
account_delay = "3s"
request_timeout = "20s"
notification_error_cooldown = "24h"

[[accounts]]
name = "main"
enabled = true
# Copy only the Cred request-header value.
cred = "replace-me"
# Copy only the Sk-Game-Role request-header value.
game_role = "replace-me"
language = "en"

[[accounts]]
name = "secondary"
enabled = false
cred = "replace-me"
game_role = "replace-me"
language = "en"

[notifications]
aggregate = true

[[notifications.targets]]
name = "discord-home"
type = "discord"
enabled = false
webhook = "https://discord.com/api/webhooks/REPLACE_ME"
events = ["claimed", "auth_expired", "error"]

[[notifications.targets]]
name = "telegram-admin"
type = "telegram"
enabled = false
bot_token = "replace-me"
chat_id = "replace-me"
events = ["auth_expired", "error"]

[[notifications.targets]]
name = "ntfy-phone"
type = "ntfy"
enabled = false
server = "https://ntfy.sh"
topic = "replace-me"
token = ""
events = ["claimed", "auth_expired", "error"]
`
