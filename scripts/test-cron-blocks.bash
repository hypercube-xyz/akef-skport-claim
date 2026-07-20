#!/usr/bin/env bash
set -Eeuo pipefail

readonly begin='# BEGIN akef-skport-claim'
readonly end='# END akef-skport-claim'

fail() {
  printf 'cron block test failed: %s\n' "$1" >&2
  exit 1
}

for script in install.sh uninstall.sh; do
  # Load only the pure crontab transformation function; installer top-level
  # behavior must never run as part of this test.
  source <(sed -n '/^strip_managed_cron_block()/,/^}/p' "$(dirname -- "$0")/$script")
  cron_begin="$begin"
  cron_end="$end"

  input="before
$begin
managed
$end
after"
  output="$(printf '%s\n' "$input" | strip_managed_cron_block)"
  [[ "$output" == $'before\nafter' ]] || fail "$script did not remove exactly one managed block"

  input=$'before\nafter'
  output="$(printf '%s\n' "$input" | strip_managed_cron_block)"
  [[ "$output" == "$input" ]] || fail "$script changed a crontab without managed markers"

  for input in \
    $'before\n# BEGIN akef-skport-claim\nmanaged' \
    $'before\n# END akef-skport-claim\nafter' \
    $'# BEGIN akef-skport-claim\none\n# END akef-skport-claim\n# BEGIN akef-skport-claim\ntwo\n# END akef-skport-claim'; do
    if printf '%s\n' "$input" | strip_managed_cron_block >/dev/null 2>&1; then
      fail "$script accepted malformed or duplicate managed markers"
    fi
  done
done

printf 'Cron block safety tests passed.\n'
