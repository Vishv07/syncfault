# Milestone M0 — Repository Bootstrap

- **Status:** Complete
- **Summary:**
  - Initialized `github.com/Vishv07/syncfault` Go module with Apache-2.0 license and ADR 0001.
  - Implemented `syncfault doctor` environment preflight checker with unprivileged user namespace detection, degraded `sigkill`-only mode fallback, and `--sweep` orphan sandbox cleanup.
  - Configured GitHub Actions CI workflow (`gofmt`, `go vet`, `go build`, `go test -race`, and `syncfault doctor`).
- **Next:** Milestone M1 — Deterministic core, sandbox path confinement, scenario YAML parser, run journal, workload driver, and `refgood` adapter.
