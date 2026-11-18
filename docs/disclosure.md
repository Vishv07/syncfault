# Responsible Disclosure Policy

SyncFault exists to make file-synchronization tools safer. Maintainers of tested tools are our partners.

## Rules

1. **Definition of a Finding:** A non-`CLEAN` verdict of severity `>= DUPLICATE` that reproduces **10/10** from its seed on a tagged release of the target. Runs that vary across replays due to target thread-scheduling nondeterminism are marked `FLAKY(seed)` and are never reported as findings.
2. **Private Maintainer Notification (`DATA_LOSS` & `WRONG_BYTES`):**
   - Any `DATA_LOSS` or `WRONG_BYTES` finding is reported **privately** to the project maintainer first (via security contact or private issue report) along with the minimal scenario YAML, seed, journal, logs, and exact `syncfault replay` command.
   - Public scorecards withhold full reproduction details for **14 days** or until the maintainer acknowledges/fixes the issue, whichever comes first.
3. **Public Bug Filing (`DUPLICATE` & `STALL`):**
   - `DUPLICATE` and `STALL` findings do not destroy user data and may be filed on public issue trackers immediately with reproduction seeds.
4. **Constructive Tone:**
   - Every published scorecard leads with methodology and limitations, praises what passed as prominently as what failed, and avoids sensationalism.
