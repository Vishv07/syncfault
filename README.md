# SyncFault

SyncFault is a deterministic fault-injection correctness harness for file-synchronization tools.

It executes seeded file workloads inside a confined sandbox, injects precisely timed faults (`sigkill`, `netcut`, `netslow`, `enospc`), allows the target sync tool to recover, and verifies every byte of the resulting replicas against an oracle of acceptable end states. Each run produces one of five severity-ordered verdicts:

```
DATA_LOSS > WRONG_BYTES > DUPLICATE > STALL > CLEAN
```

## License

Apache-2.0
