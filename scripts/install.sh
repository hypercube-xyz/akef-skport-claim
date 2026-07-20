#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

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
    printf 'Windows installation is handled by scripts/install.ps1.\n' >&2
    exit 1
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

strip_managed_cron_block() {
  awk -v begin="$cron_begin" -v end="$cron_end" '
    $0 == begin {
      if (inside) malformed=1
      blocks++
      if (blocks > 1) malformed=1
      inside=1
      next
    }
    $0 == end {
      if (!inside) malformed=1
      inside=0
      next
    }
    !inside { print }
    END {
      if (inside) malformed=1
      if (malformed) {
        print "Refusing to modify a malformed akef-skport-claim crontab block." > "/dev/stderr"
        exit 2
      }
    }
  '
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
  cleaned="$(printf '%s\n' "$current" | strip_managed_cron_block)"

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

if [[ -f "$repo_dir/go.mod" ]] && command -v go >/dev/null 2>&1; then
  (cd -- "$repo_dir" && go build -trimpath -o "$temporary_binary" ./cmd/akef-claim)
elif [[ -n "$source_binary" ]]; then
  cp -- "$source_binary" "$temporary_binary"
else
  printf 'No release binary found and Go is unavailable.\n' >&2
  exit 1
fi

chmod 0755 -- "$temporary_binary"
mv -f -- "$temporary_binary" "$installed_binary"

config_path="$("$installed_binary" config path)"
if [[ ! -f "$config_path" ]]; then
  "$installed_binary" config init
  printf 'Created %s\nEdit the placeholder credentials, then run this installer again.\n' \
    "$config_path"
  warn_path
  exit 0
fi

"$installed_binary" config validate

case "$platform" in
  linux) install_linux_scheduler ;;
  macos) install_macos_scheduler ;;
esac

printf 'Installed %s\nConfiguration: %s\nDaily schedule: %s local time\n' \
  "$installed_binary" "$config_path" "$schedule_time"
warn_path
