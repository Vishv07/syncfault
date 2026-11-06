# Milestone M1 — Deterministic Core and Safety Rails

- **Status:** Complete
- **Summary:**
  - Implemented strict path confinement (`internal/sandbox`) with symlink resolution and `ErrPathEscape` enforcement under `$XDG_RUNTIME_DIR/syncfault/<run_id>/`.
  - Built normative YAML scenario parser and single-stream PCG (`math/rand/v2`) deterministic workload expansion for `basic_crud` and `rename_storm`.
  - Added append-only fsynced JSONL run journal (`internal/journal`) and verified that running the same seed twice yields byte-identical deterministic journals.
  - Built `refgood` two-way reference sync adapter (`internal/adapter/refgood`) and wired `syncfault run` end-to-end.
- **Next:** Milestone M2 — Fault injectors (`sigkill`, `netcut`, `netslow`, `enospc`), remaining recipes, Oracle, Verifier, `refbad`, and `syncfault selfcheck` gate.
