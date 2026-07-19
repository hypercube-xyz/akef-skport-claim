#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'

readonly SCRIPT_NAME="$(basename -- "$0")"
readonly ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
readonly EXPECTED_MODULE="github.com/hypercube-xyz/akef-skport-claim"

errors=0
checks=0

ok() {
  checks=$((checks + 1))
  printf 'ok: %s\n' "$1"
}

fail() {
  errors=$((errors + 1))
  printf 'error: %s\n' "$1" >&2
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'error: required command not found: %s\n' "$1" >&2
    exit 2
  fi
}

is_tracked() {
  git ls-files --error-unmatch -- "$1" >/dev/null 2>&1
}

is_placeholder() {
  local value
  value="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"

  [[ -z "$value" ]] && return 0
  [[ "$value" == *replace-me* ]] && return 0
  [[ "$value" == *replace_me* ]] && return 0
  [[ "$value" == *redacted* ]] && return 0
  [[ "$value" == *placeholder* ]] && return 0
  [[ "$value" == *example* ]] && return 0
  [[ "$value" == your_* ]] && return 0
  [[ "$value" == '<'*'>' ]] && return 0
  [[ "$value" == '${'*'}' ]] && return 0

  return 1
}

check_required_files() {
  local required=(
    '.gitignore'
    'go.mod'
    'go.sum'
    'Makefile'
    'README.md'
    'config.example.toml'
    'cmd/akef-claim/main.go'
    'scripts/install.sh'
    'scripts/uninstall.sh'
    'scripts/check-repository.sh'
    '.github/workflows/ci.yml'
    '.github/workflows/release.yml'
  )
  local path

  for path in "${required[@]}"; do
    if [[ ! -f "$path" ]]; then
      fail "missing required file: $path"
    elif ! is_tracked "$path"; then
      fail "required file is not tracked by Git: $path"
    fi
  done

  ok 'required repository files checked'
}


check_license_files() {
  if is_tracked 'LICENSE' && [[ -f LICENSE ]]; then
    ok 'single project license file found'
    return
  fi

  if is_tracked 'LICENSE-MIT' && [[ -f LICENSE-MIT ]] && \
     is_tracked 'LICENSE-APACHE' && [[ -f LICENSE-APACHE ]]; then
    ok 'dual project license files found'
    return
  fi

  fail 'missing project license: track LICENSE, or both LICENSE-MIT and LICENSE-APACHE'
}

check_go_module() {
  local module_path go_version toolchain

  module_path="$(awk '$1 == "module" { print $2; exit }' go.mod)"
  go_version="$(awk '$1 == "go" { print $2; exit }' go.mod)"
  toolchain="$(awk '$1 == "toolchain" { print $2; exit }' go.mod)"

  if [[ "$module_path" != "$EXPECTED_MODULE" ]]; then
    fail "go.mod module must be $EXPECTED_MODULE (found: ${module_path:-missing})"
  fi

  if [[ "$go_version" != '1.26.0' ]]; then
    fail "go.mod must declare 'go 1.26.0' (found: ${go_version:-missing})"
  fi

  if [[ ! "$toolchain" =~ ^go1\.26\.[0-9]+$ ]]; then
    fail "go.mod toolchain must be a Go 1.26 patch release (found: ${toolchain:-missing})"
  fi

  ok 'Go module identity and version checked'
}

check_gitignore() {
  local ignored_paths=(
    '.env'
    '.env.local'
    'config.toml'
    'dist/example'
    'bin/example'
    'akef-claim'
    'akef-claim.exe'
  )
  local path

  for path in "${ignored_paths[@]}"; do
    if ! git check-ignore -q --no-index -- "$path"; then
      fail ".gitignore does not cover expected local/build path: $path"
    fi
  done

  if git check-ignore -q --no-index -- cmd/akef-claim/main.go; then
    fail '.gitignore incorrectly ignores cmd/akef-claim/main.go'
  fi

  ok '.gitignore coverage checked'
}

check_tracked_paths() {
  local path base
  local bad=0

  while IFS= read -r -d '' path; do
    base="${path##*/}"

    case "$path" in
      .env|*/.env|.env.*|*/.env.*|config.toml|*/config.toml)
        fail "tracked local configuration or environment file: $path"
        bad=1
        ;;
      bak/*|*/bak/*|backup/*|*/backup/*|dist/*|bin/*)
        fail "tracked generated or backup directory content: $path"
        bad=1
        ;;
    esac

    case "$base" in
      *.7z|*.zip|*.rar|*.tar|*.tar.gz|*.tgz|*.exe|*.dll|*.so|*.dylib|*.test|*.bak|*.swp|*.swo|*~|.DS_Store|Thumbs.db)
        fail "tracked generated, archive, binary, or editor file: $path"
        bad=1
        ;;
      *.pem|*.key|*.p12|*.pfx)
        fail "tracked private-key or certificate container: $path"
        bad=1
        ;;
    esac
  done < <(git ls-files -z)

  if [[ "$bad" -eq 0 ]]; then
    ok 'tracked paths contain no local secrets, archives, or build artifacts'
  fi
}

check_forbidden_content() {
  local output
  local pattern

  pattern='ubuntu-24\.06|example\.com/akef-skport-claim|akef-claim-runner|run_silent\.vbs|wscript(\.exe)?|akef-claim[[:space:]]+schedule[[:space:]]+(install|uninstall)|SKPORT_CRED|SKPORT_GAME_ROLE'

  output="$(git grep -nEI "$pattern" -- . ":(exclude)scripts/$SCRIPT_NAME" || true)"
  if [[ -n "$output" ]]; then
    fail 'obsolete project, scheduler, environment-variable, or workflow references found:'
    printf '%s\n' "$output" >&2
  else
    ok 'no obsolete project or scheduler references found'
  fi

  output="$(git grep -nEI 'https://(discord(app)?\.com)/api/webhooks/[0-9]+/[A-Za-z0-9._-]+|[0-9]{6,}:[A-Za-z0-9_-]{30,}|-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----|Authorization:[[:space:]]*(Bearer|Basic)[[:space:]]+[A-Za-z0-9+/._=-]{16,}' -- . ":(exclude)scripts/$SCRIPT_NAME" || true)"
  if [[ -n "$output" ]]; then
    fail 'probable credential, webhook, bot token, authorization value, or private key found:'
    printf '%s\n' "$output" >&2
  else
    ok 'no high-confidence secret patterns found'
  fi
}

check_config_assignments() {
  local record file line_number key value
  local found_bad=0

  while IFS= read -r record; do
    [[ -z "$record" ]] && continue

    if [[ "$record" =~ ^([^:]+):([0-9]+):[[:space:]]*(cred|game_role|webhook|bot_token|chat_id|token)[[:space:]]*=[[:space:]]*\"([^\"]*)\" ]]; then
      file="${BASH_REMATCH[1]}"
      line_number="${BASH_REMATCH[2]}"
      key="${BASH_REMATCH[3]}"
      value="${BASH_REMATCH[4]}"
      if ! is_placeholder "$value"; then
        fail "non-placeholder value assigned to $key at $file:$line_number"
        found_bad=1
      fi
    fi
  done < <(git grep -nE '^[[:space:]]*(cred|game_role|webhook|bot_token|chat_id|token)[[:space:]]*=' -- . ":(exclude)scripts/$SCRIPT_NAME" || true)

  if [[ "$found_bad" -eq 0 ]]; then
    ok 'configuration examples contain placeholders only'
  fi
}

check_workflows() {
  local workflow line target ref
  local bad=0

  while IFS= read -r -d '' workflow; do
    if ! grep -Eq '^permissions:' "$workflow"; then
      fail "workflow does not declare top-level permissions: $workflow"
      bad=1
    fi

    if grep -Eq '^[[:space:]]*pull_request_target:' "$workflow"; then
      fail "workflow uses pull_request_target, which is not allowed: $workflow"
      bad=1
    fi

    if grep -Eq '^[[:space:]]*schedule:' "$workflow"; then
      fail "scheduled GitHub Actions workflows are not allowed: $workflow"
      bad=1
    fi

    while IFS= read -r line; do
      [[ "$line" =~ ^[[:space:]]*uses:[[:space:]]*([^[:space:]#]+) ]] || continue
      target="${BASH_REMATCH[1]}"

      case "$target" in
        ./*|docker://*)
          continue
          ;;
      esac

      if [[ "$target" != *@* ]]; then
        fail "workflow action has no ref in $workflow: $target"
        bad=1
        continue
      fi

      ref="${target##*@}"
      if [[ ! "$ref" =~ ^[0-9a-fA-F]{40}$ ]]; then
        fail "workflow action is not pinned to a full commit SHA in $workflow: $target"
        bad=1
      fi
    done < "$workflow"
  done < <(find .github/workflows -maxdepth 1 -type f \( -name '*.yml' -o -name '*.yaml' \) -print0)

  if [[ "$bad" -eq 0 ]]; then
    ok 'GitHub workflows use restricted triggers, explicit permissions, and full-SHA action pins'
  fi
}

check_script_modes() {
  local path mode
  local bad=0

  while IFS= read -r -d '' path; do
    mode="$(git ls-files --stage -- "$path" | awk 'NR == 1 { print $1 }')"
    if [[ "$mode" != '100755' ]]; then
      fail "$path must be tracked with Git mode 100755 (found: ${mode:-untracked})"
      bad=1
    fi
  done < <(find scripts -maxdepth 1 -type f -name '*.sh' -print0)

  if [[ "$bad" -eq 0 ]]; then
    ok 'Bash scripts are executable in the Git index'
  fi
}

check_readme() {
  local needle
  local required_text=(
    './scripts/install.sh'
    './scripts/uninstall.sh'
    'accounts[].cred'
    'accounts[].game_role'
    'Cred'
    'Sk-Game-Role'
  )

  for needle in "${required_text[@]}"; do
    if ! grep -Fq -- "$needle" "README.md"; then
      fail "README.md is missing required setup text: $needle"
    fi
  done

  ok 'README setup sections checked'
}

check_source_formatting() {
  local unformatted

  if ! command -v go >/dev/null 2>&1; then
    printf 'skip: Go is unavailable; gofmt check was not run\n'
    return
  fi

  unformatted="$(gofmt -l .)"
  if [[ -n "$unformatted" ]]; then
    fail 'Go source files require gofmt:'
    printf '%s\n' "$unformatted" >&2
  else
    ok 'Go source files are formatted'
  fi
}

check_git_whitespace() {
  if ! git diff --check; then
    fail 'working-tree diff contains whitespace errors'
  else
    ok 'working-tree diff has no whitespace errors'
  fi

  if ! git diff --cached --check; then
    fail 'staged diff contains whitespace errors'
  else
    ok 'staged diff has no whitespace errors'
  fi
}

main() {
  require_command git
  require_command awk
  require_command grep
  require_command find

  cd "$ROOT_DIR"

  if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    printf 'error: this script must run inside a Git working tree\n' >&2
    exit 2
  fi

  check_required_files
  check_license_files
  check_go_module
  check_gitignore
  check_tracked_paths
  check_forbidden_content
  check_config_assignments
  check_workflows
  check_script_modes
  check_readme
  check_source_formatting
  check_git_whitespace

  printf '\nRepository checks completed: %d passed, %d failed.\n' "$checks" "$errors"

  if [[ "$errors" -ne 0 ]]; then
    exit 1
  fi
}

main "$@"
