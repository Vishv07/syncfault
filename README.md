# SyncFault

**SyncFault is a deterministic fault-injection correctness harness for file-synchronization tools.**

Think *"Jepsen, but for desktop file sync."*

---

## Why SyncFault Exists

Suppose a user renames `thesis_draft.docx` to `thesis_final.docx` and their laptop battery dies at that exact instant. Inside a file-sync engine, a rename is multiple state transitions; the crash landed between them. On reboot, does the tool recover the file, duplicate it, wedge itself, or propagate the half-finished delete to the cloud and back to the laptop—destroying every copy?

SyncFault makes that crash happen on demand, anchored to a deterministic operation index, across thousands of seeded runs, and verifies every byte of the resulting replicas against an Oracle of acceptable end states.

Each run produces one of five severity-ordered verdicts:

```
DATA_LOSS (40) > WRONG_BYTES (30) > DUPLICATE (20) > STALL (10) > CLEAN (0)
```

---

## Architecture

```
 scenarios/*.yaml ──▶ ┌──────────────────────────────────────────┐
                      │ ORCHESTRATOR (seeded PCG stream)         │
                      │  • workload driver   • fault injector    │
                      │  • append-only fsynced run journal       │
                      └───────────────┬──────────────────────────┘
                                      │ controls (never reaches inside target)
                 ┌────────────────────▼─────────────────────────────┐
                 │ SANDBOX  ($XDG_RUNTIME_DIR/syncfault/<run_id>/)  │
                 │  sideA/ ◀──▶ [ target process ] ◀──▶ remote ◀──▶ sideB/ │
                 │          (bisync | alto | unison | refgood/bad)  │
                 └────────────────────┬─────────────────────────────┘
                                      │ after quiescence loop
                      ┌───────────────▼──────────────────────────┐
                      │ ORACLE: acceptable end states from journal│
                      │ VERIFIER: walk A, B, remote → classify    │
                      └───────────────┬──────────────────────────┘
                                      ▼
                       runs.db (SQLite) ──▶ report/scorecard.md & index.html
```

---

## Quickstart

```bash
# 1. Build the single static binary
go build -o syncfault ./cmd/syncfault

# 2. Verify host environment and sandbox capabilities
./syncfault doctor

# 3. Run the Self-Validation Gate (verifies refgood passes and refbad is caught 10/10)
./syncfault selfcheck

# 4. Run a single scenario with a specific seed
./syncfault run --adapter refgood --scenario scenarios/rename_storm_sigkill.yaml --seed 412

# 5. Replay a recorded run deterministically 10 times
./syncfault replay --run-id refgood-rename_storm_sigkill-seed-412 --replays 10

# 6. Run a full scenario suite and generate the Markdown + HTML scorecard
./syncfault suite --adapter bisync --suite smoke --db runs.db
./syncfault report --db runs.db --out report/
```

---

## CLI Exit Codes

| Exit Code | Meaning |
|---:|---|
| `0` | `CLEAN` — Replicas converged to an acceptable state |
| `2` | Harness or configuration error (never masquerades as a verdict) |
| `10` | `STALL` — Failed to converge within the quiescence window |
| `20` | `DUPLICATE` — Unexpected duplicate artifacts remain |
| `30` | `WRONG_BYTES` — Torn or corrupted file content hash detected |
| `40` | `DATA_LOSS` — Required file version missing across all replicas and trash |

---

## Documentation

- [Methodology, Oracle Semantics & Quiescence Rule](docs/methodology.md)
- [Write a Target Adapter in an Afternoon](docs/adapters.md)
- [Author Scenarios in YAML Without Writing Go](docs/scenarios.md)
- [Responsible Disclosure Policy](docs/disclosure.md)
- [Contributing & Good First Issues](CONTRIBUTING.md)

---

## Limitations (v1)

- **Process, network, and quota faults only:** v1 injects `sigkill`, `netcut`, `netslow`, and `enospc`. VM/QEMU snapshot power-loss testing (destroying unsynced kernel page cache) is on the v2 roadmap.
- **Two-replica topology:** v1 tests `sideA <-> remote <-> sideB`; multi-node mesh graphs are planned for v2.
- **Not a speed benchmark:** Convergence time is recorded strictly as a secondary diagnostic metric.

## License

Apache-2.0 — see [LICENSE](LICENSE).
