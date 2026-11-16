# Findings Drafts (`docs/findings/`)

Every candidate finding (`DATA_LOSS`, `WRONG_BYTES`, `DUPLICATE`, or `STALL`) that reproduces **10/10** from its seed on a tagged target release is documented here as a draft before upstream disclosure.

## Required Fields per Finding Draft

1. **Target & Version:** Exact output of `<adapter> Version()`.
2. **Verdict:** Severity classification (`DATA_LOSS > WRONG_BYTES > DUPLICATE > STALL`).
3. **Minimal Scenario & Seed:** Scenario YAML path and integer seed.
4. **Replay Command:**
   ```bash
   syncfault replay --run-id <run-id> --replays 10
   ```
5. **Journal Excerpt:** The operation anchor where the fault fired and the pre/post content hashes.
6. **Disclosure Status:** Private maintainer report date and 14-day embargo window (§12).
