# Milestone M3 — Bisync Adapter, Replay Engine, and Concurrent Edit Matrix

- **Status:** Complete
- **Summary:**
  - Implemented persistent `runs.db` storage (`internal/journal/store.go`) and `syncfault replay --run-id` with automatic `FLAKY(seed)` detection across replays.
  - Added the published §7.8 quiescence loop (`WaitForQuiescence`) and embedded loopback remote server (`internal/sandbox/remote_server.go`) honoring `netcut` and `netslow` markers.
  - Built the `bisync` (`rclone bisync`) target adapter with strictly confined sandbox configuration (`rclone.conf`).
  - Added the `concurrent_edit` two-replica workload recipe (`sides: [A,B]`), conflict policy overrides (`conflict_copy`, `lww`), and `suite`/`ci` CLI commands.
- **Next:** Milestone M4 — Second target adapter (`alto` / `unison`), parallel worker pool, and log collection on failure.
