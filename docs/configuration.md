# Configuration

The only configuration sources are the default TOML file and the global `--config PATH` flag. Unknown keys, duplicate names, invalid durations, unsupported notification types, placeholder credentials, and an empty enabled-account set are rejected.

Default paths:

- Windows config: `%LOCALAPPDATA%\akef-skport-claim\config.toml`
- Linux config: `${XDG_CONFIG_HOME:-$HOME/.config}/akef-skport-claim/config.toml`
- macOS config: `$HOME/Library/Application Support/akef-skport-claim/config.toml`

Cache and scheduled logs use the operating system's per-user cache directory.
The Windows installer copies an existing `%APPDATA%\akef-skport-claim\config.toml`
to the non-roaming location and removes the legacy copy only after the new
scheduler installation succeeds.

The application subdirectory is always `akef-skport-claim`. Run `akef-claim config path` to see the exact config path.

Use [config.example.toml](../config.example.toml) as the schema reference. Account names and notification target names must be unique. Account language overrides the application language and may contain only ASCII letters, digits, `-`, and `_`.

The config path must resolve to a regular file. On Unix, it must not be group/world readable; `0600` is recommended. The generated file uses `0600` and its parent directory uses `0700`. On Windows the CLI performs only portable best-effort file checks, so users must also ensure through Windows security settings that only their account can read it.

Never commit the real file. Secret values are redacted in formatting and logs, but filesystem privacy remains essential.

## Obtaining account values

For your own account, sign in at `https://game.skport.com/endfield/sign-in`, inspect the browser Network request to `/web/v1/game/endfield/attendance`, copy only the `Cred` request-header value into `accounts[].cred`, and copy only the `Sk-Game-Role` value into `accounts[].game_role`. Header names are case-insensitive, so browser capitalization may differ. The application generates `Sign` and `Timestamp`; do not copy them into the config.

Do not copy the full request as cURL, export a HAR file, or share screenshots/full headers. Those forms may expose additional session material. If either value is disclosed, log out, sign in again, and replace the stored values. See the README for step-by-step instructions.

Scheduler time is an installer concern rather than an application configuration key. Use `powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\install.ps1 -Time HH:mm` on Windows or `./scripts/install.sh --time HH:MM` on Linux/macOS; a `[schedule]` table is not part of the TOML schema.
