# SyncFault Methodology

## 1. Black-Box Principle

SyncFault never reaches inside a sync target. It controls only what the operating system and hardware control: files, process lifecycles, network links, and storage quotas. Target logs are collected for human autopsy on non-CLEAN runs, but the Verifier never reads them.

## 2. Deterministic Seeded Simulation

Every random decision in a run — file names, directory depths, payload bytes, operation order, and post-anchor fault jitter — derives from a single `math/rand/v2` PCG stream initialized from the run's integer seed.

Fault injectors anchor on **operation indices** recorded in the append-only journal (`journal.jsonl`), never on arbitrary wall-clock timers. Running `syncfault replay --run-id <id>` re-executes the identical operation sequence and fault anchor.

## 3. Oracle & Acceptable End States

Rather than comparing replicas against a single static golden tree, the Oracle computes a **lattice of acceptable end states** from the journal alone:

- **In-flight single-side writes:** A file whose mutation was in flight when the fault fired may settle in either its `pre` or `post` content hash, but never vanish if it existed in both and never settle with torn/unrecorded bytes.
- **Renames across a fault:** Acceptable outcomes are `{old name only, new name only, both where the extra copy is a documented conflict/trash artifact}`. Losing both copies is `DATA_LOSS`.
- **Concurrent edits (`sides: [A, B]`):** Under `conflict_copy` (default), both versions must survive (one canonical, one conflict copy). Under `lww`, losing one concurrent version is permitted and flagged `lww_discard`.

## 4. Five-Verdict Taxonomy

Evaluated worst-first:

1. **`DATA_LOSS` (Exit 40):** A required file version exists nowhere across `sideA`, `sideB`, `remote`, or documented conflict/trash directories.
2. **`WRONG_BYTES` (Exit 30):** Any file on any replica has a SHA-256 digest that matches no version ever written per the journal.
3. **`DUPLICATE` (Exit 20):** All required data survives, but unexpected duplicate artifacts remain.
4. **`STALL` (Exit 10):** Replicas failed to converge across recovery sync cycles within the quiescence window, with no `DATA_LOSS` or `WRONG_BYTES` in the frozen state.
5. **`CLEAN` (Exit 0):** Replicas converged to an acceptable state with zero extra artifacts.

## 5. Quiescence Rule

After `Heal()`, SyncFault alternates recovery `SyncCycle` calls and full tree walks (`WalkSandbox`). A run is marked **converged** when two consecutive recovery sync cycles complete with zero file content changes across `sideA`, `sideB`, and `remote` within the scenario timeout.
