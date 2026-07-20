# Arknights: Endfield SKPORT Claim

[![CI](https://github.com/hypercube-xyz/akef-skport-claim/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/hypercube-xyz/akef-skport-claim/actions/workflows/ci.yml)
[![Go version](https://img.shields.io/github/go-mod/go-version/hypercube-xyz/akef-skport-claim)](https://go.dev/dl/)
[![Go Reference](https://pkg.go.dev/badge/github.com/hypercube-xyz/akef-skport-claim.svg)](https://pkg.go.dev/github.com/hypercube-xyz/akef-skport-claim)
[![Latest release](https://img.shields.io/github/v/release/hypercube-xyz/akef-skport-claim)](https://github.com/hypercube-xyz/akef-skport-claim/releases)
[![License](https://img.shields.io/github/license/hypercube-xyz/akef-skport-claim)](LICENSE)
[![Renovate enabled](https://img.shields.io/badge/renovate-enabled-brightgreen.svg?logo=renovatebot)](https://docs.renovatebot.com/)

`akef-claim` is an unofficial, local-only command-line tool for checking and claiming Arknights: Endfield SKPORT daily attendance. It is not affiliated with Hypergryph or Gryphline. Automated use may carry account risk; use it sparingly and at your own risk.

The tool runs on your computer, stores no credentials remotely, exposes no server, and includes no captcha solving, browser automation, anti-bot bypass, fingerprint spoofing, proxy rotation, or cloud claim workflow.

## Security warning

`cred` and `game_role` are session secrets. Keep `config.toml` private. Never paste the file, cookies, request headers, bot tokens, chat IDs, webhook URLs, or unredacted logs into an issue. If a secret is exposed, log out or rotate the session and remove it from any Git history.

## Install and first-time setup

Download and extract the archive for your operating system. Every release archive contains the platform executable together with the installer scripts and documentation:

- Windows: `akef-claim.exe`
- Linux/macOS: `akef-claim`

Before installation, compare the archive's SHA-256 digest with the matching entry in the release's `checksums.txt`. For example:

```bash
sha256sum -c checksums.txt --ignore-missing
```

On Windows PowerShell, compare `Get-FileHash .\akef-claim_*.zip -Algorithm SHA256` with the published checksum.

On Windows, use the native PowerShell installer:

```powershell
./scripts/install.ps1
```

If PowerShell blocks local scripts because of its execution policy, use:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\install.ps1
```

On Linux and macOS, use the Bash installer:

```bash
./scripts/install.sh
```

The first installer invocation installs the executable, creates `config.toml`, and prints its path. Replace the placeholders, then invoke the installer again; the scheduler is not installed until the configuration is valid.

Manual source builds require Go 1.26.5:

```bash
make build
```

The executable is written under `bin/`. The equivalent direct command is:

```bash
go build -trimpath ./cmd/akef-claim
```

### Obtain the account header values

Use only a session belonging to your own account:

1. Open the official Endfield SKPORT attendance page at `https://game.skport.com/endfield/sign-in`, sign in, and select the intended game role.
2. Open browser developer tools (`F12` or **Inspect**) and select the **Network** tab.
3. Reload the page and filter requests by `/web/v1/game/endfield/attendance`.
4. Select the attendance request and open **Headers** → **Request Headers**.
5. Copy only the value of `Cred` into `accounts[].cred`.
6. Copy only the value of `Sk-Game-Role` into `accounts[].game_role`.

HTTP header names are case-insensitive, so the browser may display them as `cred`, `Cred`, `sk-game-role`, or `Sk-Game-Role`. Do not copy `Sign` or `Timestamp`; the program generates those values for each signed request.

```toml
[[accounts]]
name = "main"
enabled = true
cred = "<CRED_HEADER_VALUE>"
game_role = "<SK_GAME_ROLE_HEADER_VALUE>"
language = "en"
```

Do not use **Copy as cURL**, export a HAR file, post a screenshot, or share the full request headers; those forms commonly contain additional session material. If either value is exposed, log out of SKPORT, sign in again, and replace the stored values before continuing. Repeat the steps after switching account or role when configuring multiple accounts.

### Complete setup

Edit the TOML file printed by the installer. Configuration is TOML-only; environment variables are not read as a fallback. The full schema is documented in [configuration documentation](docs/configuration.md) and [config.example.toml](config.example.toml).

Run the installer again after saving the config. It validates the file and installs the operating-system scheduler directly. The default time is `00:05` in the user's local timezone.

On Windows, use the PowerShell parameter `-Time` (one dash). For example, to run every day at midnight:

```powershell
./scripts/install.ps1 -Time '00:00'
```

On Linux or macOS, use the Bash option `--time` (two dashes):

```bash
./scripts/install.sh --time 00:05
```

When the installation directory is on `PATH`, validate the application independently with:

```bash
akef-claim config path
akef-claim config validate
akef-claim status
```

Scheduler creation and removal are intentionally owned by the native installer scripts rather than CLI subcommands.

For additional accounts, add another `[[accounts]]` table with a unique `name`, then repeat the header-capture steps while signed in to the intended account and role.

## Use

```bash
akef-claim run
akef-claim run --account main
akef-claim status
akef-claim doctor
akef-claim doctor --network
akef-claim notify test discord-home
akef-claim --silent run
```

`status` never claims. `run` refreshes the session, checks attendance, and sends at most one claim POST only when an item is available. A claim POST is never automatically retried after a timeout or another ambiguous result. Contradictory `available=true` and `done=true` flags on the same attendance item fail closed and are treated as already claimed.

Claim-capable runs apply startup jitter before acquiring an exclusive process lock. A second run waits for the lock for up to 10 minutes and then rechecks attendance, so an overlapping manual or scheduled invocation cannot silently skip the entire day. Read-only `status` checks do not take the claim lock.

## Scheduler

```bash
./scripts/install.sh --time 00:05
./scripts/uninstall.sh
./scripts/uninstall.sh --purge
```

Windows equivalents:

```powershell
./scripts/install.ps1 -Time '00:00'
./scripts/uninstall.ps1
./scripts/uninstall.ps1 -Purge
```

- Windows: `install.ps1` creates or replaces the per-user task through the native ScheduledTasks PowerShell module. A Windows Script Host launcher starts `akef-claim.exe --silent run` with no console window, including for manual Task Scheduler runs. `uninstall.ps1` removes the task and launcher without requiring Git Bash.
- Linux: the installer writes and enables a user-level systemd service/timer. If no usable systemd user manager is available, it installs one tagged crontab block without modifying unrelated entries.
- macOS: the installer writes and loads a user LaunchAgent.

A silent scheduled invocation has a 30-minute application deadline. Windows applies a 35-minute Task Scheduler execution limit and retries up to three times, 30 minutes apart, only when the application returns transient pre-claim exit code `30`. Linux and macOS do not add process retries. The claim POST itself is never retried automatically.

Scheduled logs are written as daily files under the operating-system user cache directory. At every silent start, regular Arknights: Endfield SKPORT scheduled logs older than 45 days are deleted; a current daily file is also size-rotated after 5 MiB. Uninstall retains configuration, logs, and notification state unless `--purge` is supplied.

GitHub Actions is used only for repository build and test CI on pushes and pull requests. Daily attendance runs belong on the user's local scheduler; the repository has no scheduled Actions workflow and stores no attendance credentials in GitHub. See [scheduler documentation](docs/scheduler.md).

## Notifications

Discord webhooks, Telegram Bot API, and ntfy are supported. Test notifications are synthetic and never contact SKPORT:

```bash
akef-claim notify test telegram-admin
```

Notification failure never causes another claim request. See [notification documentation](docs/notifications.md).

## Exit codes

- `0`: success, already claimed, or unavailable
- `10`: configuration error
- `20`: authentication/session expired
- `30`: transient pre-claim failure, including network/server errors, lock timeout, or scheduled deadline
- `40`: definite claim API error
- `41`: ambiguous claim result; do not retry automatically
- `50`: unexpected internal error

Silent scheduled mode uses the same exit codes. The Windows task maps only exit code `30` to a retryable Task Scheduler failure; codes `40` and `41` can therefore never trigger another automatic claim attempt. Linux and macOS schedulers do not add process retries.

## Development

The repository uses Bash-backed Make targets:

```bash
make repo-check
make check
make ci
make lint
make vuln
make coverage
make build
make install SCHEDULE_TIME=00:05
make uninstall
make snapshot
```

`make repo-check` rejects secret-bearing or stale tracked files. `make check` also verifies modules, tidy state, script syntax, golangci-lint, vet, and tests. `make ci` additionally scans reachable Go code for known vulnerabilities, runs the race detector, enforces at least 95% statement coverage, and builds the current platform. `make lint` requires the golangci-lint version recorded in `.golangci-lint-version`; follow the [official installation guide](https://golangci-lint.run/docs/welcome/install/local/) to install its binary. `make vuln` runs the `govulncheck` tool pinned in `go.mod`. `make coverage` writes `coverage.out` and fails below `COVERAGE_MIN` (default `95.0`). `make install` and `make uninstall` select the native PowerShell scripts on Windows and Bash scripts on Linux/macOS. `make snapshot` requires GoReleaser and creates local release archives without publishing them.

## Troubleshooting and reporting

Run `akef-claim doctor` first, then consult [troubleshooting](docs/troubleshooting.md). When reporting an issue, include the version, operating system, redacted outcome, and safe reproduction steps. Never attach the real config or secret-bearing screenshots.

See [SECURITY.md](SECURITY.md) for private vulnerability reporting guidance.

## License

Licensed under the MIT License. See [LICENSE](LICENSE).
