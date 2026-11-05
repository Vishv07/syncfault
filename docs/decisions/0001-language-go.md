# ADR 0001: Implement SyncFault in Go with Standard Library Priority

- **Status:** Accepted
- **Date:** 2026-10-29

## Context

SyncFault orchestrates disposable sandboxes, spawns and signals child process groups, drives deterministic filesystem workloads, records append-only journals, and verifies directory trees across multiple replicas. Contributors and sync-tool maintainers need to run the harness in CI or locally with minimal setup friction.

## Decision

1. **Language:** Go (>= 1.22).
   - Produces a single static binary with trivial cross-platform compilation.
   - Provides `math/rand/v2` (PCG stream) in the standard library for deterministic seeded simulation.
   - Provides first-class process group control (`os/exec`, `syscall.Kill`, Linux namespace clone flags).
   - Matches the ecosystem of major sync targets such as `rclone`.
2. **Dependency Policy:** Prefer the Go standard library (`flag`, `log/slog`, `crypto/sha256`, `math/rand/v2`, `encoding/json`, `html/template`) over external packages wherever practical. Every external dependency must be justified in `docs/decisions/`.

## Consequences

- Builds and tests run offline in seconds with zero supply-chain overhead.
- A purpose-built parser handles the normative scenario YAML subset without pulling in reflection-heavy third-party dependencies.
