# Contributing to SyncFault

Thank you for helping make file-synchronization tools safer and more reliable.

## Quick Development Setup

```bash
git clone https://github.com/Vishv07/syncfault.git
cd syncfault
go build -o syncfault ./cmd/syncfault
./syncfault doctor
./syncfault selfcheck
```

## Guides

- **Add a new sync tool adapter:** Read [docs/adapters.md](docs/adapters.md) ("Writing a SyncFault Target Adapter in an Afternoon").
- **Add new fault scenarios:** Read [docs/scenarios.md](docs/scenarios.md) ("Authoring Scenarios Without Writing Go").
- **Understand the Oracle & Verifier:** Read [docs/methodology.md](docs/methodology.md).

## Pull Request Rules

1. `gofmt`, `go vet ./...`, and `go test -race ./...` must pass cleanly.
2. `./syncfault selfcheck` must pass (`refgood` clean, `refbad` caught 10/10).
3. Every PR description must answer: **"Does this change affect verdicts? (Yes/No + why)"**.
4. Never weaken a verdict in the Oracle or Verifier to make a target pass.

## Good First Issues for New Contributors

1. **`[good-first-issue] [scenario]` Case-collision rename scenario:** Add a YAML scenario testing case-only renames (`Report.txt` -> `report.txt`) under `sigkill`.
2. **`[good-first-issue] [scenario]` Deep nesting boundary scenario:** Add a `move_tree` scenario with `depth: 8` and `netslow` packet loss.
3. **`[good-first-issue] [adapter]` Syncthing two-node REST adapter:** Implement `internal/adapter/syncthing` using Syncthing's `/rest/db/scan` and `/rest/db/completion` endpoints.
4. **`[good-first-issue] [adapter]` Nextcloud desktop CLI (`nextcloudcmd`) adapter:** Implement `internal/adapter/nextcloudcmd` driving `nextcloudcmd --non-interactive` in a sandbox.
5. **`[good-first-issue] [injector]` Unicode normalization path stress:** Extend workload file naming with NFC/NFD Unicode path variants behind a scenario param.
