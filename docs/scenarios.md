# Authoring Scenarios Without Writing Go

In SyncFault, **scenarios are data, not code**. You can add new fault timings, file sizing distributions, or conflict-resolution tests by adding a YAML file under `scenarios/`.

## Scenario Schema

```yaml
name: rename_storm_sigkill
description: Deep directory renamed while files sync; process killed mid-cycle.
workload:
  recipe: rename_storm        # basic_crud | rename_storm | move_tree | mass_delete | concurrent_edit | many_small_files | one_huge_file
  params:
    depth: 4
    files: 24
    file_size: { min: 1KiB, max: 64KiB }
  sides: [A]                  # [A] = single-side sequential; [A, B] = concurrent mutations
fault:
  type: sigkill               # none | sigkill | netcut | netslow | enospc
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
| `many_small_files` | Creates dozens of small files (`64B`–`2KiB`) to stress metadata and lock handling. |
| `one_huge_file` | Creates and modifies a multi-hundred-KiB/MiB payload to test interrupted transfers and `enospc`. |

## Running Your Scenario

```bash
syncfault run --adapter refgood --scenario scenarios/rename_storm_sigkill.yaml --seed 412
```
