# Contributing

Contributions are welcome for bug fixes, tests, documentation, platform
compatibility, notification integrations, and maintainability improvements.

## Before opening an issue

- Search existing issues first.
- Use placeholders instead of credentials or account data.
- Never attach full configuration files, HAR files, cookies, request headers,
  bot tokens, chat IDs, or webhook URLs.
- Security-sensitive reports must follow `SECURITY.md`.

## Development setup

```bash
git clone https://github.com/hypercube-xyz/akef-skport-claim.git
cd akef-skport-claim

go mod download
golangci-lint run
make vuln
make coverage
make check
```

Use the golangci-lint release recorded in `.golangci-lint-version`; the project configuration is `.golangci.yml`.

## Project layout

- `cmd/akef-claim`: process entry point only.
- `internal/cli`: commands and exit-code handling.
- `internal/app`: run orchestration and account decisions.
- `internal/skport`: SKPORT HTTP, signing, and response normalization.
- `internal/config` and `internal/state`: user configuration and persisted state.
- `internal/notify`: notification selection and destination transports.
- `internal/result` and `internal/report`: run data and presentation.
- `internal/atomicfile`, `internal/lock`, and `internal/logging`: shared infrastructure.

Keep protocol code in `skport`, command behavior in `cli`, and workflow
decisions in `app`. Prefer extending an existing package over adding a package
for a single type or constant.
