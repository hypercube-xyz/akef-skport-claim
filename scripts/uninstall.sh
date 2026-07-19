#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

readonly task_name='AKEF SKPort Daily Claim'
readonly systemd_unit='akef-skport-claim'
readonly launchd_label='io.github.hypercube-xyz.akef-skport-claim'
readonly cron_begin='# BEGIN akef-skport-claim'
readonly cron_end='# END akef-skport-claim'

purge=false
while (($# > 0)); do
  case "$1" in
    --purge)
      purge=true
      shift
      ;;
    --help | -h)
      printf 'Usage: %s [--purge]\n' "$0"
      exit 0
      ;;
    *)
      printf 'Unknown argument: %s\nUsage: %s [--purge]\n' "$1" "$0" >&2
      exit 2
      ;;
  esac
done

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

cleanup_paths=()
cleanup() {
  local path
  for path in "${cleanup_paths[@]}"; do
    [[ -n "$path" ]] && rm -f -- "$path"
  done
}
trap cleanup EXIT

remove_windows_scheduler() {
  command -v schtasks.exe >/dev/null 2>&1 || {
    printf 'schtasks.exe is required to remove the Windows task.\n' >&2
    return 1
  }

  if ! schtasks.exe /Query /TN "$task_name" >/dev/null 2>&1; then
    printf 'Task Scheduler task already absent: %s\n' "$task_name"
    return 0
  fi

  # Stop an active instance first. /End fails when the task is not currently
  # running, so that result is intentionally ignored.
  schtasks.exe /End /TN "$task_name" >/dev/null 2>&1 || true
  schtasks.exe /Delete /TN "$task_name" /F

  if schtasks.exe /Query /TN "$task_name" >/dev/null 2>&1; then
    printf 'Task Scheduler task still exists after deletion: %s\n' "$task_name" >&2
    return 1
  fi

  printf 'Removed Task Scheduler task: %s\n' "$task_name"
}

remove_cron_block() {
  command -v crontab >/dev/null 2>&1 || return 0

  local current temporary
  current="$(crontab -l 2>/dev/null || true)"
  [[ "$current" == *"$cron_begin"* ]] || return 0

  temporary="$(mktemp)"
  cleanup_paths+=("$temporary")

  printf '%s\n' "$current" | awk -v begin="$cron_begin" -v end="$cron_end" '
    $0 == begin { skip=1; next }
    $0 == end { skip=0; next }
    !skip { print }
  ' >"$temporary"

  crontab "$temporary"
  printf 'Removed managed crontab entry.\n'
}

remove_linux_scheduler() {
  local user_dir service_path timer_path
  user_dir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
  service_path="$user_dir/${systemd_unit}.service"
  timer_path="$user_dir/${systemd_unit}.timer"

  if [[ -e "$service_path" || -e "$timer_path" ]]; then
    if command -v systemctl >/dev/null 2>&1 &&
      systemctl --user show-environment >/dev/null 2>&1; then
      systemctl --user stop "${systemd_unit}.timer" >/dev/null 2>&1 || true
      systemctl --user stop "${systemd_unit}.service" >/dev/null 2>&1 || true
      systemctl --user disable "${systemd_unit}.timer" >/dev/null 2>&1 || true
    fi

    rm -f -- "$timer_path" "$service_path"

    if command -v systemctl >/dev/null 2>&1 &&
      systemctl --user show-environment >/dev/null 2>&1; then
      systemctl --user daemon-reload >/dev/null 2>&1 || true
      systemctl --user reset-failed "${systemd_unit}.service" >/dev/null 2>&1 || true
      systemctl --user reset-failed "${systemd_unit}.timer" >/dev/null 2>&1 || true
    fi

    printf 'Removed systemd user service and timer.\n'
  fi

  # Always check cron as well. An older install or a system without a user
  # systemd manager may have used the managed cron fallback.
  remove_cron_block
}

remove_macos_scheduler() {
  local agents_dir plist_path uid
  agents_dir="$HOME/Library/LaunchAgents"
  plist_path="$agents_dir/${launchd_label}.plist"
  uid="$(id -u)"

  # bootout also terminates a currently running LaunchAgent instance.
  launchctl bootout "gui/${uid}/${launchd_label}" >/dev/null 2>&1 || true

  if [[ -e "$plist_path" ]]; then
    rm -f -- "$plist_path"
    printf 'Removed LaunchAgent: %s\n' "$launchd_label"
  else
    printf 'LaunchAgent already absent: %s\n' "$launchd_label"
  fi
}

remove_scheduler() {
  case "$platform" in
    windows) remove_windows_scheduler ;;
    linux) remove_linux_scheduler ;;
    macos) remove_macos_scheduler ;;
  esac
}

resolve_user_paths() {
  case "$platform" in
    windows)
      command -v cygpath >/dev/null 2>&1 || {
        printf 'cygpath is required for --purge on Windows. Run this script from Git Bash.\n' >&2
        return 1
      }
      [[ -n "${APPDATA:-}" ]] || {
        printf 'APPDATA is not set.\n' >&2
        return 1
      }
      [[ -n "${LOCALAPPDATA:-}" ]] || {
        printf 'LOCALAPPDATA is not set.\n' >&2
        return 1
      }
      config_path="$(cygpath -au "$APPDATA")/akef-skport-claim/config.toml"
      cache_dir="$(cygpath -au "$LOCALAPPDATA")/akef-skport-claim"
      ;;
    linux)
      config_path="${XDG_CONFIG_HOME:-$HOME/.config}/akef-skport-claim/config.toml"
      cache_dir="${XDG_CACHE_HOME:-$HOME/.cache}/akef-skport-claim"
      ;;
    macos)
      config_path="$HOME/Library/Application Support/akef-skport-claim/config.toml"
      cache_dir="$HOME/Library/Caches/akef-skport-claim"
      ;;
  esac
}

remove_scheduler

if [[ -e "$installed_binary" || -L "$installed_binary" ]]; then
  rm -f -- "$installed_binary"
  printf 'Removed binary: %s\n' "$installed_binary"
else
  printf 'Binary already absent: %s\n' "$installed_binary"
fi

if $purge; then
  resolve_user_paths

  rm -rf -- "$cache_dir"
  rm -f -- "$config_path"

  config_dir="$(dirname -- "$config_path")"
  rmdir -- "$config_dir" 2>/dev/null || true

  printf 'Removed configuration, scheduled logs, lock, and notification state.\n'
else
  printf 'Retained configuration, scheduled logs, lock, and notification state.\n'
fi