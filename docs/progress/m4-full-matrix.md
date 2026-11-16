# Milestone M4 — Second Target Adapter and Full Matrix Runner

- **Status:** Complete
- **Summary:**
  - Implemented headless `alto` and `unison` target adapters (`internal/adapter/alto`).
  - Added parallel sandbox worker pool (`--jobs N`, defaulting to `min(4, NumCPU/2)`) to `syncfault suite` with zero shared mutable state between sandboxes.
  - Added structured per-run JSON logging (`log/slog`) and automatic target log collection on non-CLEAN verdicts.
  - Expanded the YAML scenario corpus (`scenarios/`) and created the findings disclosure draft template in `docs/findings/`.
- **Next:** Milestone M5 — Scorecard generator (`report/scorecard.md` + `report/index.html`), contributor guides (`adapters.md`, `scenarios.md`, `methodology.md`, `disclosure.md`), and open-source governance files.
