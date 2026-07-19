#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

readonly task_name='AKEF SKPort Daily Claim'
readonly systemd_unit='akef-skport-claim'
readonly launchd_label='io.github.hypercube-xyz.akef-skport-claim'
readonly cron_begin='# BEGIN akef-skport-claim'
readonly cron_end='# END akef-skport-claim'

schedule_time='00:05'
while (($# > 0)); do
  case "$1" in
    --time)
      (($# >= 2)) || {
        printf 'Missing value for --time.\n' >&2
        exit 2
      }
      schedule_time="$2"
      shift 2
      ;;
    --help | -h)
      printf 'Usage: %s [--time HH:MM]\n' "$0"
      exit 0
      ;;
    *)
      printf 'Unknown argument: %s\nUsage: %s [--time HH:MM]\n' "$1" "$0" >&2
      exit 2
      ;;
  esac
done

if [[ ! "$schedule_time" =~ ^([01][0-9]|2[0-3]):[0-5][0-9]$ ]]; then
  printf 'Invalid schedule time %q; expected HH:MM in 24-hour local time.\n' "$schedule_time" >&2
  exit 2
fi

schedule_hour="${schedule_time%%:*}"
schedule_minute="${schedule_time##*:}"
schedule_hour_number=$((10#$schedule_hour))
schedule_minute_number=$((10#$schedule_minute))

cleanup_paths=()
cleanup() {
  local path
  for path in "${cleanup_paths[@]}"; do
    [[ -n "$path" ]] && rm -f -- "$path"
  done
}
trap cleanup EXIT

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd -- "$script_dir/.." && pwd)"
os_name="$(uname -s)"

case "$os_name" in
  MINGW* | MSYS* | CYGWIN*)
    platform='windows'
    binary_name='akef-claim.exe'
    ;;
  Linux*)
    platform='linux'
    binary_name='akef-claim'
    ;;
  Darwin*)
    platform='macos'
    binary_name='akef-claim'
    ;;
  *)
    printf 'Unsupported operating system: %s\n' "$os_name" >&2
    exit 1
    ;;
esac

install_dir="${XDG_BIN_HOME:-$HOME/.local/bin}"
installed_binary="$install_dir/$binary_name"

warn_path() {
  case ":${PATH}:" in
    *":${install_dir}:"*) ;;
    *)
      printf 'WARN %s is not on PATH; add it before invoking %s by name.\n' \
        "$install_dir" "$binary_name" >&2
      ;;
  esac
}

xml_escape() {
  local value="$1"
  value="${value//&/&amp;}"
  value="${value//</&lt;}"
  value="${value//>/&gt;}"
  value="${value//\"/&quot;}"
  value="${value//\'/&apos;}"
  printf '%s' "$value"
}

powershell_quote() {
  local value="$1"
  value="${value//\'/\'\'}"
  printf "'%s'" "$value"
}

windows_path() {
  local value="$1"
  if [[ "$value" =~ ^[A-Za-z]:[\\/] ]]; then
    printf '%s' "$value"
    return 0
  fi

  command -v cygpath >/dev/null 2>&1 || {
    printf 'cygpath is required on Windows. Run this script from Git Bash.\n' >&2
    return 1
  }
  cygpath -aw "$value"
}

systemd_quote() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//%/%%}"
  printf '"%s"' "$value"
}

shell_quote() {
  local value="$1"
  value="${value//\'/\'\\\'\'}"
  printf "'%s'" "$value"
}

install_windows_scheduler() {
  command -v schtasks.exe >/dev/null 2>&1 || {
    printf 'schtasks.exe is required.\n' >&2
    return 1
  }
  command -v powershell.exe >/dev/null 2>&1 || {
    printf 'Windows PowerShell is required.\n' >&2
    return 1
  }
  command -v iconv >/dev/null 2>&1 || {
    printf 'iconv is required to generate Task Scheduler XML.\n' >&2
    return 1
  }
  command -v base64 >/dev/null 2>&1 || {
    printf 'base64 is required to generate the hidden task action.\n' >&2
    return 1
  }

  local binary_windows config_windows task_author task_sid
  local ps_command encoded_command start_date
  local xml_utf8 xml_utf16 xml_windows

  binary_windows="$(windows_path "$installed_binary")"
  config_windows="$(windows_path "$config_path")"
  task_author="$(whoami.exe | tr -d '\r\n')"
  task_sid="$({
    powershell.exe \
      -NoLogo \
      -NoProfile \
      -NonInteractive \
      -Command '[System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value'
  } | tr -d '\r\n')"

  [[ -n "$task_author" ]] || {
    printf 'Unable to determine the current Windows account.\n' >&2
    return 1
  }
  [[ "$task_sid" == S-1-* ]] || {
    printf 'Unable to determine the current Windows user SID.\n' >&2
    return 1
  }

  # Task Scheduler retries only errors that are safe to repeat. The claim POST
  # itself is never retried by the application. Exit 30 represents a transient
  # network/server failure; failure to launch the executable is also retryable.
  ps_command=$(cat <<POWERSHELL
\$ErrorActionPreference = 'Stop'
try {
    \$binary = $(powershell_quote "$binary_windows")
    \$config = $(powershell_quote "$config_windows")

    if (-not (Test-Path -LiteralPath \$binary -PathType Leaf)) {
        throw 'akef-claim executable was not found'
    }

    & \$binary --silent run --config \$config
    \$code = \$LASTEXITCODE

    if (\$null -eq \$code) {
        throw 'akef-claim did not return an exit code'
    }
}
catch {
    exit 1
}

if (\$code -eq 30) {
    exit 1
}

exit 0
POWERSHELL
)

  encoded_command="$({
    printf '%s' "$ps_command" |
      iconv -f UTF-8 -t UTF-16LE |
      base64
  } | tr -d '\r\n')"

  start_date="$(date +%Y-%m-%d)"
  xml_utf8="$(mktemp "${TMPDIR:-/tmp}/akef-task.XXXXXX.xml")"
  xml_utf16="${xml_utf8%.xml}.utf16.xml"
  cleanup_paths+=("$xml_utf8" "$xml_utf16")

  cat >"$xml_utf8" <<XML
<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Author>$(xml_escape "$task_author")</Author>
    <URI>\\$(xml_escape "$task_name")</URI>
    <Description>Run the local AKEF SKPORT attendance claim once per day.</Description>
  </RegistrationInfo>
  <Triggers>
    <CalendarTrigger>
      <StartBoundary>${start_date}T${schedule_time}:00</StartBoundary>
      <Enabled>true</Enabled>
      <ScheduleByDay>
        <DaysInterval>1</DaysInterval>
      </ScheduleByDay>
    </CalendarTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>$(xml_escape "$task_sid")</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>true</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <WakeToRun>false</WakeToRun>
    <ExecutionTimeLimit>PT35M</ExecutionTimeLimit>
    <Priority>7</Priority>
    <RestartOnFailure>
      <Interval>PT30M</Interval>
      <Count>3</Count>
    </RestartOnFailure>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>powershell.exe</Command>
      <Arguments>-NoLogo -NoProfile -NonInteractive -WindowStyle Hidden -EncodedCommand ${encoded_command}</Arguments>
    </Exec>
  </Actions>
</Task>
XML

  # schtasks expects the XML declaration and bytes to agree. Write a UTF-16LE
  # BOM followed by UTF-16LE content.
  printf '\xFF\xFE' >"$xml_utf16"
  iconv -f UTF-8 -t UTF-16LE "$xml_utf8" >>"$xml_utf16"
  xml_windows="$(windows_path "$xml_utf16")"

  schtasks.exe /Create /TN "$task_name" /XML "$xml_windows" /F
  schtasks.exe /Query /TN "$task_name" /FO LIST /V
}

install_systemd_scheduler() {
  local user_dir service_path timer_path binary_arg config_arg

  user_dir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
  service_path="$user_dir/${systemd_unit}.service"
  timer_path="$user_dir/${systemd_unit}.timer"
  mkdir -p -- "$user_dir"

  binary_arg="$(systemd_quote "$installed_binary")"
  config_arg="$(systemd_quote "$config_path")"

  cat >"$service_path" <<UNIT
[Unit]
Description=AKEF SKPORT daily attendance claim
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=${binary_arg} --silent run --config ${config_arg}
TimeoutStartSec=35min

[Install]
WantedBy=default.target
UNIT

  cat >"$timer_path" <<TIMER
[Unit]
Description=Run AKEF SKPORT daily attendance claim

[Timer]
OnCalendar=*-*-* ${schedule_time}:00
Persistent=true
Unit=${systemd_unit}.service

[Install]
WantedBy=timers.target
TIMER

  systemctl --user daemon-reload
  systemctl --user enable --now "${systemd_unit}.timer"
  systemctl --user --no-pager status "${systemd_unit}.timer" || true
}

install_cron_scheduler() {
  command -v crontab >/dev/null 2>&1 || {
    printf 'Neither a usable systemd user manager nor crontab is available.\n' >&2
    return 1
  }

  local current cleaned command_line temporary
  current="$(crontab -l 2>/dev/null || true)"
  cleaned="$(printf '%s\n' "$current" | awk -v begin="$cron_begin" -v end="$cron_end" '
    $0 == begin { skip=1; next }
    $0 == end { skip=0; next }
    !skip { print }
  ')"

  command_line="${schedule_minute_number} ${schedule_hour_number} * * * $(shell_quote "$installed_binary") --silent run --config $(shell_quote "$config_path") >/dev/null 2>&1"
  temporary="$(mktemp)"
  cleanup_paths+=("$temporary")

  {
    printf '%s\n' "$cleaned"
    printf '%s\n%s\n%s\n' "$cron_begin" "$command_line" "$cron_end"
  } | awk 'NF || previous { print } { previous=NF }' >"$temporary"

  crontab "$temporary"
  printf 'Installed managed crontab entry:\n%s\n' "$command_line"
  printf 'WARN cron does not provide missed-run catch-up.\n' >&2
}

install_linux_scheduler() {
  if command -v systemctl >/dev/null 2>&1 &&
    systemctl --user show-environment >/dev/null 2>&1; then
    install_systemd_scheduler
  else
    install_cron_scheduler
  fi
}

install_macos_scheduler() {
  local agents_dir plist_path uid

  agents_dir="$HOME/Library/LaunchAgents"
  plist_path="$agents_dir/${launchd_label}.plist"
  uid="$(id -u)"
  mkdir -p -- "$agents_dir"

  launchctl bootout "gui/${uid}/${launchd_label}" >/dev/null 2>&1 || true

  cat >"$plist_path" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>$(xml_escape "$launchd_label")</string>
  <key>ProgramArguments</key>
  <array>
    <string>$(xml_escape "$installed_binary")</string>
    <string>--silent</string>
    <string>run</string>
    <string>--config</string>
    <string>$(xml_escape "$config_path")</string>
  </array>
  <key>StartCalendarInterval</key>
  <dict>
    <key>Hour</key>
    <integer>${schedule_hour_number}</integer>
    <key>Minute</key>
    <integer>${schedule_minute_number}</integer>
  </dict>
  <key>ProcessType</key>
  <string>Background</string>
  <key>AbandonProcessGroup</key>
  <true/>
</dict>
</plist>
PLIST

  chmod 0600 -- "$plist_path"
  launchctl bootstrap "gui/${uid}" "$plist_path"
  launchctl print "gui/${uid}/${launchd_label}" | head -40
}

mkdir -p -- "$install_dir"
temporary_binary="$(mktemp "$install_dir/.${binary_name}.install.XXXXXX")"
cleanup_paths+=("$temporary_binary")

source_binary=''
for candidate in "$script_dir/$binary_name" "$repo_dir/$binary_name"; do
  if [[ -f "$candidate" ]]; then
    source_binary="$candidate"
    break
  fi
done

if [[ -n "$source_binary" ]]; then
  cp -- "$source_binary" "$temporary_binary"
elif command -v go >/dev/null 2>&1; then
  (cd -- "$repo_dir" && go build -trimpath -o "$temporary_binary" ./cmd/akef-claim)
else
  printf 'No release binary found and Go is unavailable.\n' >&2
  exit 1
fi

chmod 0755 -- "$temporary_binary"
mv -f -- "$temporary_binary" "$installed_binary"

config_path="$("$installed_binary" config path)"
config_file_path="$config_path"
if [[ "$platform" == 'windows' && "$config_path" =~ ^[A-Za-z]:[\\/] ]]; then
  config_file_path="$(cygpath -au "$config_path")"
fi

if [[ ! -f "$config_file_path" ]]; then
  "$installed_binary" config init
  printf 'Created %s\nEdit the placeholder credentials, then run this installer again.\n' \
    "$config_path"
  warn_path
  exit 0
fi

"$installed_binary" config validate

case "$platform" in
  windows) install_windows_scheduler ;;
  linux) install_linux_scheduler ;;
  macos) install_macos_scheduler ;;
esac

printf 'Installed %s\nConfiguration: %s\nDaily schedule: %s local time\n' \
  "$installed_binary" "$config_path" "$schedule_time"
warn_path