# Milestone M2 — Fault Injectors, Oracle, Verifier, and Selfcheck Gate

- **Status:** Complete
- **Summary:**
  - Implemented operation-count anchored fault injectors (`sigkill`, `netcut`, `netslow`, `enospc`) with seeded jitter and `Heal()` restoration.
  - Added remaining sequential workload recipes (`move_tree`, `mass_delete`, `many_small_files`, `one_huge_file`).
  - Built pure `oracle.Compute` acceptable-state lattice and `verify.Evaluate` 5-verdict classifier (`DATA_LOSS > WRONG_BYTES > DUPLICATE > STALL > CLEAN`) with property-based tests.
  - Built sabotaged `refbad` adapter and `syncfault selfcheck` gate (10/10 `DATA_LOSS` and 10/10 `WRONG_BYTES` caught on `refbad`, 0 failures on `refgood`), wired into CI.
- **Next:** Milestone M3 — SQLite `runs.db` store, deterministic `replay`, quiescence loop, `concurrent_edit` recipe, embedded SFTP server, and `rclone bisync` adapter.
