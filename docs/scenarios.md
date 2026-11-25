# Authoring Scenarios Without Writing Go

In SyncFault, **scenarios are data, not code**. You can add new fault timings, file sizing distributions, or conflict-resolution tests by adding a YAML file under `scenarios/`.

## Scenario Schema

```yaml
name: rename_storm_sigkill
description: Deep directory renamed while files sync; process killed mid-cycle.
workload:
  recipe: rename_storm        # basic_crud | rename_storm | move_tree | mass_delete | concurrent_edit | many_small_files | one_huge_file | case_collision | unicode_norm | symlink_swap
  params:
    depth: 4
    files: 24
    file_size: { min: 1KiB, max: 64KiB }
  sides: [A]                  # [A] = single-side sequential; [A, B] = concurrent mutations
fault:
  type: sigkill               # none | sigkill | netcut | netslow | enospc | volatile_write | clock_skew | state_corrupt
  at:
    op_index: 12              # operation anchor in the journal
    jitter_ms: { min: 0, max: 40, seeded: true }
  params: {}
expect:
  conflict_policy: default    # default | conflict_copy | lww
runs:
  seeds: [1..50]              # expands to 50 deterministic seeds
  timeout_s: 180
```

## Built-In Workload Recipes

| Recipe | Description |
|---|---|
| `basic_crud` | Creates files across nested directories, edits a subset, renames, and deletes. |
| `rename_storm` | Populates a deep directory tree and renames every file across sync cycles. |
| `move_tree` | Relocates an entire directory hierarchy to a new branch mid-sync. |
| `mass_delete` | Creates a dataset and deletes the majority of files while retaining a sentinel subset. |
| `concurrent_edit` | Mutates overlapping paths on both `sideA` and `sideB` inside shared sync windows. |
| `many_small_files` | Creates dozens of small files (`64B` to `2KiB`) to stress metadata and lock handling. |
| `one_huge_file` | Creates and modifies a multi-hundred-KiB/MiB payload to test interrupted transfers and `enospc`. |
| `case_collision` | Executes case-only renames (`Report_001.txt` to `report_001.txt`) to catch case-folding deletions across APFS, NTFS, and ext4. |
| `unicode_norm` | Creates and edits files using composed (`NFC`) and decomposed (`NFD`) UTF-8 sequences to detect normalization duplicates. |
| `symlink_swap` | Rotates release directories (`releases/v1` to `releases/v2`) and active pointer manifests under sync interruption. |

## Built-In Fault Injectors

| Fault Type | Description |
|---|---|
| `none` | Baseline fault-free execution. |
| `sigkill` | Sends `SIGKILL` to the target process group at the anchored `op_index` plus seeded jitter. |
| `netcut` | Cuts the sandbox network link (`fault-netcut.active` / Linux `netns` veth down) until `Heal()`. |
| `netslow` | Injects configurable link delay and packet loss (`delay_ms`, `loss_pct`). |
| `enospc` | Enforces a deterministic byte quota on the sandbox replica/remote (`syscall.ENOSPC`). |
| `volatile_write` | Mutates an active file mid-sync while transfers are in flight to detect non-atomic torn writes. |
| `clock_skew` | Shifts and coarsens replica file `mtime` (`skew_s`, `granularity_s`) to test LWW under clock drift and FAT32/WebDAV rounding. |
| `state_corrupt` | Truncates or corrupts local sync state caches (`*.lst`, `*.json`, `*.db`) to verify protection against false mass deletions. |

## Running Your Scenario

```bash
syncfault run --adapter refgood --scenario scenarios/case_collision_sigkill.yaml --seed 412
```
