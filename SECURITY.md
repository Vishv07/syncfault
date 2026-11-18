# Security Policy

## Reporting a Vulnerability in SyncFault

SyncFault executes third-party file synchronization binaries inside local sandboxes. Any bug that allows a workload or adapter to escape `$XDG_RUNTIME_DIR/syncfault/<run_id>/` (violating `internal/sandbox.ErrPathEscape`) is treated as a critical security vulnerability.

Please report sandbox-escape or privilege-handling vulnerabilities privately via **GitHub Private Vulnerability Reporting** on this repository rather than opening a public issue.

## Findings in Tested Sync Tools

For data-loss or corruption findings discovered in upstream sync tools using SyncFault, we strictly follow our 14-day maintainer-first [Responsible Disclosure Policy](docs/disclosure.md).
