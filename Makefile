SHELL := bash
.SHELLFLAGS := -Eeuo pipefail -c

GO ?= go
GORELEASER ?= goreleaser
GOLANGCI_LINT ?= golangci-lint
COVERAGE_MIN ?= 95.0
COVERAGE_PROFILE ?= coverage.out
BINARY := akef-claim
COMMAND := ./cmd/akef-claim
OUTPUT_DIR ?= bin
SCHEDULE_TIME ?= 00:05

ifeq ($(OS),Windows_NT)
INSTALL_COMMAND := powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File scripts/install.ps1 -Time "$(SCHEDULE_TIME)"
UNINSTALL_COMMAND := powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File scripts/uninstall.ps1
INSTALL_HELP_COMMAND := powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File scripts/install.ps1 -Help
UNINSTALL_HELP_COMMAND := powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File scripts/uninstall.ps1 -Help
INSTALL_TEST_COMMAND := powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File scripts/test-install.ps1
else
INSTALL_COMMAND := ./scripts/install.sh --time "$(SCHEDULE_TIME)"
UNINSTALL_COMMAND := ./scripts/uninstall.sh
INSTALL_HELP_COMMAND := ./scripts/install.sh --help
UNINSTALL_HELP_COMMAND := ./scripts/uninstall.sh --help
INSTALL_TEST_COMMAND := :
endif

.PHONY: all help fmt fmt-check shell-check repo-check tidy tidy-check verify lint vet vuln test test-race coverage build install uninstall check ci snapshot clean

all: check

help:
	@printf '%s\n' \
		'make check      Verify modules, formatting, Bash syntax, vet, and tests' \
		'make ci         Run the complete local CI suite, including the race detector' \
		'make build      Build the current-platform executable under bin/' \
		'make lint       Run the pinned golangci-lint configuration' \
		'make vuln       Scan reachable Go code for known vulnerabilities' \
		'make coverage   Enforce at least 95% statement coverage' \
		'make install    Install the binary and local scheduler (SCHEDULE_TIME=HH:MM)' \
		'make uninstall  Remove the binary and local scheduler' \
		'make repo-check Check tracked-file hygiene and credential patterns' \
		'make snapshot   Build a local GoReleaser snapshot under dist/' \
		'make clean      Remove generated build output'

fmt:
	gofmt -w .

fmt-check:
	@unformatted="$$(gofmt -l .)"; \
	if [[ -n "$${unformatted}" ]]; then \
		printf 'The following files require gofmt:\n%s\n' "$${unformatted}"; \
		exit 1; \
	fi

shell-check:
	bash -n scripts/*.sh scripts/*.bash
	bash scripts/test-cron-blocks.bash
	$(INSTALL_HELP_COMMAND) >/dev/null
	$(UNINSTALL_HELP_COMMAND) >/dev/null
	$(INSTALL_TEST_COMMAND)

repo-check:
	./scripts/check-repository.sh

tidy:
	$(GO) mod tidy

tidy-check:
	$(GO) mod tidy -diff

verify:
	$(GO) mod verify

lint:
	$(GOLANGCI_LINT) run

vet:
	$(GO) vet ./...

vuln:
	$(GO) tool govulncheck ./...

test:
	$(GO) test -count=1 ./...

test-race:
	$(GO) test -race -count=1 ./...

coverage:
	$(GO) test -count=1 -coverprofile="$(COVERAGE_PROFILE)" ./...
	@awk -v minimum="$(COVERAGE_MIN)" 'NR > 1 { total += $$(NF-1); if ($$NF > 0) covered += $$(NF-1) } END { actual = 100 * covered / total; if (actual < minimum) { printf "statement coverage %.4f%% is below required %.4f%%\n", actual, minimum > "/dev/stderr"; exit 1 } printf "statement coverage %.4f%% meets required %.4f%%\n", actual, minimum }' "$(COVERAGE_PROFILE)"

build:
	@mkdir -p "$(OUTPUT_DIR)"; \
	goexe="$$($(GO) env GOEXE)"; \
	$(GO) build -trimpath -o "$(OUTPUT_DIR)/$(BINARY)$${goexe}" "$(COMMAND)"

install:
	$(INSTALL_COMMAND)

uninstall:
	$(UNINSTALL_COMMAND)

check: repo-check verify tidy-check fmt-check shell-check lint vet test

ci: repo-check verify tidy-check fmt-check shell-check lint vet vuln test-race coverage build

snapshot:
	command -v "$(GORELEASER)" >/dev/null
	"$(GORELEASER)" release --snapshot --clean

clean:
	rm -rf bin dist coverage.out
