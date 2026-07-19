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
make check
