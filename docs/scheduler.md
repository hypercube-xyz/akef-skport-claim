# Native scheduler installation

Scheduler lifecycle is owned by the native installer scripts, not by `akef-claim` CLI subcommands. On Linux and macOS:

```bash
./scripts/install.sh --time 00:05
./scripts/uninstall.sh
./scripts/uninstall.sh --purge
```

On Windows, use the native PowerShell scripts:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\install.ps1 -Time 00:05
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\uninstall.ps1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\uninstall.ps1 -Purge
```

`-Time` on Windows and `--time` on Linux/macOS accept local 24-hour `HH:mm` and default to `00:05`. Installation and removal are user-scoped and idempotent. The first install invocation creates the default TOML file and exits; edit its placeholders and run the installer again.

## Execution safety

Startup random delay is controlled by `[run].random_delay` and occurs before the process takes the claim lock. A claim-capable invocation waits up to 10 minutes for another run and then checks attendance again. The read-only `status` command does not take that lock.

`akef-claim --silent run` has a 30-minute application deadline. Deadline or lock-wait expiration returns transient exit code `30`; the claim POST itself is never automatically retried. Scheduled logs use one file per local day, rotate after 5 MiB, and are retained for at most 45 days. Cleanup runs at the start of every silent invocation.

## Windows

`install.ps1` uses the native ScheduledTasks PowerShell module. The relevant operations are equivalent to:

```powershell
$action = New-ScheduledTaskAction -Execute "$env:SystemRoot\System32\wscript.exe" -Argument $backgroundLauncher
$trigger = New-ScheduledTaskTrigger -Daily -At 00:05
Register-ScheduledTask -TaskName 'Arknights: Endfield SKPORT Daily Claim' -InputObject $task
```

The task uses the current user's interactive token with least privilege, `IgnoreNew`, missed-start handling, and a 35-minute execution limit. Its Windows Script Host launcher invokes the installed `akef-claim.exe --silent run --config ...` without creating a console window, including when the task is started manually. The launcher maps only exit code `30` to Task Scheduler failure, allowing at most three delayed retries. Definite or ambiguous claim outcomes are mapped to scheduler success so they cannot cause another claim attempt.

Removal uses the same native module:

```powershell
Unregister-ScheduledTask -TaskName 'Arknights: Endfield SKPORT Daily Claim' -Confirm:$false
```

Query the task directly with:

```powershell
Get-ScheduledTask -TaskName 'Arknights: Endfield SKPORT Daily Claim'
```

## Linux

When a systemd user manager is usable, `install.sh` writes `akef-skport-claim.service` and `akef-skport-claim.timer` under the user systemd directory, then enables the timer. Otherwise it manages only the tagged `# BEGIN/END akef-skport-claim` crontab block and preserves unrelated entries. Cron cannot guarantee missed-run catch-up. If one marker is missing, either marker is duplicated, or the markers are out of order, installation and removal stop without replacing the crontab; repair the managed block manually and rerun the script.

Inspect systemd state with:

```bash
systemctl --user status akef-skport-claim.timer
```

## macOS

The installer writes `~/Library/LaunchAgents/io.github.hypercube-xyz.akef-skport-claim.plist` and loads it into the current GUI user domain. Inspect it with:

```bash
launchctl print "gui/$(id -u)/io.github.hypercube-xyz.akef-skport-claim"
```

Normal uninstall retains the TOML configuration, logs, lock, and notification state. Use `powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\uninstall.ps1 -Purge` on Windows or `./scripts/uninstall.sh --purge` on Linux/macOS to remove them after removing the scheduler and installed binary.
