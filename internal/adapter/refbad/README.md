# `refbad` — Deliberately Sabotaged Reference Sync Adapter

`refbad` exists solely to power SyncFault's **Self-Validation Gate** (`syncfault selfcheck`).
Before SyncFault is trusted to judge any external sync tool, it must prove that it catches `refbad` and passes `refgood`.

## Documented Sabotages

| Sabotage | Mechanism | Expected Verdict |
|---|---|---|
| **1. Non-atomic rename without journal** | During a `rename_storm` or `move_tree` workload interrupted by `sigkill`, `refbad` loses track of the rename journal and propagates the missing old path as a deletion over the new path on both replicas. | `DATA_LOSS` (10/10 replayable) |
| **2. Torn in-place write without temp file or `fsync`** | When `sigkill`, `netcut`, or `enospc` interrupts an active write, `refbad` leaves a truncated buffer (`TORN_WRITE:...`) at the destination path and replicates the torn file back to `sideA`. | `WRONG_BYTES` (10/10 replayable) |
