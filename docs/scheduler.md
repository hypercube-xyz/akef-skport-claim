# Native scheduler installation

Scheduler lifecycle is owned by the Bash scripts, not by `akef-claim` CLI subcommands:

```bash
./scripts/install.sh --time 00:05
./scripts/uninstall.sh
./scripts/uninstall.sh --purge
```

`--time` accepts local 24-hour `HH:MM` and defaults to `00:05`. Installation and removal are user-scoped and idempotent. The first install invocation creates the default TOML file and exits; edit its placeholders and run the installer again.

## Execution safety

Startup random delay is controlled by `[run].random_delay` and occurs before the process takes the claim lock. A claim-capable invocation waits up to 10 minutes for another run and then checks attendance again. The read-only `status` command does not take that lock.

`akef-claim --silent run` has a 30-minute application deadline. Deadline or lock-wait expiration returns transient exit code `30`; the claim POST itself is never automatically retried. Scheduled logs use one file per local day, rotate after 5 MiB, and are retained for at most 45 days. Cleanup runs at the start of every silent invocation.

## Windows

From Git Bash, `install.sh` generates a temporary Task Scheduler XML document and executes a command equivalent to:

```bash
schtasks.exe /Create /TN "AKEF SKPort Daily Claim" /XML "<temporary-xml>" /F
```

The XML configures a daily interactive-token task with `IgnoreNew`, missed-start handling, least privilege, a 30-minute execution limit, and a hidden built-in PowerShell action. That action invokes the installed `akef-claim.exe --silent run --config ...` and maps only exit code `30` to Task Scheduler failure, allowing at most three delayed retries. Definite or ambiguous claim outcomes are mapped to scheduler success so they cannot cause another claim attempt. The temporary XML is deleted by the installer.

Removal uses:

```bash
schtasks.exe /Delete /TN "AKEF SKPort Daily Claim" /F
```

Query the task directly with:

```bash
schtasks.exe /Query /TN "AKEF SKPort Daily Claim" /V /FO LIST
```

## Linux

When a systemd user manager is usable, `install.sh` writes `akef-skport-claim.service` and `akef-skport-claim.timer` under the user systemd directory, then enables the timer. Otherwise it manages only the tagged `# BEGIN/END akef-skport-claim` crontab block and preserves unrelated entries. Cron cannot guarantee missed-run catch-up.

Inspect systemd state with:

```bash
systemctl --user status akef-skport-claim.timer
```

## macOS

The installer writes `~/Library/LaunchAgents/io.github.hypercube-xyz.akef-skport-claim.plist` and loads it into the current GUI user domain. Inspect it with:

```bash
launchctl print "gui/$(id -u)/io.github.hypercube-xyz.akef-skport-claim"
```

Normal uninstall retains the TOML configuration, logs, lock, and notification state. `./scripts/uninstall.sh --purge` explicitly removes them after removing the scheduler and installed binary.
