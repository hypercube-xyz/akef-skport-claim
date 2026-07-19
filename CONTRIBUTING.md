# Contributing

Use Go 1.26.5, keep changes focused, and run the complete local checks:

```bash
make check
make ci
```

`make check` verifies modules, module tidiness, formatting, vet, and tests. `make ci` additionally runs the race detector and builds the current-platform executable. Run `make snapshot` when modifying release packaging; it requires GoReleaser.

Tests must use fixtures, fakes, or `httptest.Server`; never contact real SKPORT or notification endpoints. Do not submit credentials or captured private responses. Preserve status-before-claim, conservative handling of contradictory attendance flags, process-lock rechecking, and claim-at-most-once behavior.

All committed scripts and shell examples must be Bash. Generated Windows Task Scheduler XML is the only place that may invoke the built-in PowerShell host.

## Bash file modes

All files under `scripts/` must be committed as executable and use LF line endings. On Windows, after staging a new script, run:

```bash
git update-index --chmod=+x scripts/*.sh
```

`make repo-check` and CI reject tracked Bash scripts without mode `100755`.
