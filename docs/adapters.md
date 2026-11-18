# Writing a SyncFault Target Adapter in an Afternoon

SyncFault isolates all target-specific code behind a single 8-method Go interface (`internal/adapter.Target`). Adding a new sync engine takes roughly 150–200 lines of Go and requires zero changes to the workload driver, fault injectors, oracle, or verifier.

## The `Target` Interface

```go
type Target interface {
    Name() string
    Version(ctx context.Context) (string, error)
    Setup(ctx context.Context, sb *sandbox.Sandbox) error
    SyncCycle(ctx context.Context) error
    PID() int
    CollectLogs(dst string) error
    Teardown(ctx context.Context) error
    ConflictPolicy() scenario.ConflictPolicy
}
```

## Worked Example: `unison` CLI Adapter Skeleton

Below is a complete, compiling adapter skeleton for [Unison](https://github.com/bcpierce00/unison):

```go
package unison

import (
    "context"
    "os"
    "os/exec"
    "path/filepath"
    "sync"
    "syscall"

    "github.com/Vishv07/syncfault/internal/adapter"
    "github.com/Vishv07/syncfault/internal/sandbox"
    "github.com/Vishv07/syncfault/internal/scenario"
)

func init() {
    adapter.Register("unison", func() adapter.Target {
        return &Adapter{}
    })
}

type Adapter struct {
    sb         *sandbox.Sandbox
    profileDir string
    logFile    string
    mu         sync.Mutex
    activePID  int
}

func (a *Adapter) Name() string { return "unison" }

func (a *Adapter) Version(ctx context.Context) (string, error) {
    out, err := exec.CommandContext(ctx, "unison", "-version").Output()
    if err != nil {
        return "", err
    }
    return string(out), nil
}

func (a *Adapter) ConflictPolicy() scenario.ConflictPolicy {
    return scenario.ConflictPolicyConflictCopy
}

func (a *Adapter) Setup(_ context.Context, sb *sandbox.Sandbox) error {
    a.sb = sb
    a.profileDir = filepath.Join(sb.Work, "unison-profile")
    a.logFile = filepath.Join(sb.Logs, "unison.log")
    return os.MkdirAll(a.profileDir, 0o755)
}

func (a *Adapter) SyncCycle(ctx context.Context) error {
    cmd := exec.CommandContext(ctx, "unison",
        a.sb.SideA,
        a.sb.SideB,
        "-batch",
        "-auto",
        "-logfile", a.logFile,
    )
    cmd.Env = append(os.Environ(), "UNISON="+a.profileDir)
    cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
    if err := cmd.Start(); err != nil {
        return err
    }
    a.mu.Lock()
    a.activePID = cmd.Process.Pid
    a.mu.Unlock()
    defer func() {
        a.mu.Lock()
        a.activePID = 0
        a.mu.Unlock()
    }()
    return cmd.Wait()
}

func (a *Adapter) PID() int {
    a.mu.Lock()
    defer a.mu.Unlock()
    return a.activePID
}

func (a *Adapter) CollectLogs(dst string) error {
    data, err := os.ReadFile(a.logFile)
    if err != nil {
        return nil
    }
    _ = os.MkdirAll(dst, 0o755)
    return os.WriteFile(filepath.Join(dst, "unison.log"), data, 0o644)
}

func (a *Adapter) Teardown(_ context.Context) error {
    if pid := a.PID(); pid > 0 {
        _ = syscall.Kill(-pid, syscall.SIGKILL)
    }
    return nil
}
```

## Safety Checklist for Adapter Authors

1. **Confinement:** Never read or mutate user configuration outside `sb.Root` (`~/.config`, `~/.unison`, etc.). Always override `HOME` or pass explicit `--config` flags pointing inside `sb.Work`.
2. **Process Groups:** Set `SysProcAttr: &syscall.SysProcAttr{Setpgid: true}` so `sigkill` terminates the entire process tree cleanly.
3. **Kill-Safe Teardown:** `Teardown` is invoked even after `SIGKILL` runs and panics; ensure it is idempotent.
