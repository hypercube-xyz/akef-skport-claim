SHELL := bash
.SHELLFLAGS := -Eeuo pipefail -c

GO ?= go
GORELEASER ?= goreleaser
BINARY := akef-claim
COMMAND := ./cmd/akef-claim
OUTPUT_DIR ?= bin
SCHEDULE_TIME ?= 00:05

.PHONY: all help fmt fmt-check shell-check repo-check tidy tidy-check verify vet test test-race build install uninstall check ci snapshot clean

all: check

help:
	@printf '%s\n' \
		'make check      Verify modules, formatting, Bash syntax, vet, and tests' \
		'make ci         Run the complete local CI suite, including the race detector' \
		'make build      Build the current-platform executable under bin/' \
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
	bash -n scripts/*.sh
	./scripts/install.sh --help >/dev/null
	./scripts/uninstall.sh --help >/dev/null

repo-check:
	./scripts/check-repository.sh

tidy:
	$(GO) mod tidy

tidy-check:
	$(GO) mod tidy -diff

verify:
	$(GO) mod verify

vet:
	$(GO) vet ./...

test:
	$(GO) test -count=1 ./...

test-race:
	$(GO) test -race -count=1 ./...

build:
	@mkdir -p "$(OUTPUT_DIR)"; \
	goexe="$$($(GO) env GOEXE)"; \
	$(GO) build -trimpath -o "$(OUTPUT_DIR)/$(BINARY)$${goexe}" "$(COMMAND)"

install:
	./scripts/install.sh --time "$(SCHEDULE_TIME)"

uninstall:
	./scripts/uninstall.sh

check: repo-check verify tidy-check fmt-check shell-check vet test

ci: repo-check verify tidy-check fmt-check shell-check vet test-race build

snapshot:
	command -v "$(GORELEASER)" >/dev/null
	"$(GORELEASER)" release --snapshot --clean

clean:
	rm -rf bin dist coverage.out
