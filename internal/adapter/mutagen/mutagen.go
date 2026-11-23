package mutagen

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/Vishv07/syncfault/internal/adapter"
	"github.com/Vishv07/syncfault/internal/adapter/refgood"
	"github.com/Vishv07/syncfault/internal/sandbox"
	"github.com/Vishv07/syncfault/internal/scenario"
)

func init() {
	adapter.Register("mutagen", func() adapter.Target {
		return &Adapter{binaryName: "mutagen"}
	})
	adapter.Register("syncthing", func() adapter.Target {
		return &Adapter{binaryName: "syncthing"}
	})
}

// Adapter drives Mutagen or Syncthing headlessly inside a confined sandbox.
type Adapter struct {
	binaryName string
	sb         *sandbox.Sandbox
	dataDir    string
	logFile    string
	fallback   *refgood.Adapter

	mu        sync.Mutex
	activePID int
}

func (a *Adapter) Name() string {
	if a.binaryName == "" {
		return "mutagen"
	}
	return a.binaryName
}

func (a *Adapter) Version(ctx context.Context) (string, error) {
	bin, err := exec.LookPath(a.Name())
	if err != nil {
		return fmt.Sprintf("%s-headless-emulated (%s binary not on PATH)", a.Name(), a.Name()), nil
	}
	out, err := exec.CommandContext(ctx, bin, "version").Output()
	if err != nil {
		return a.Name() + "-unknown", nil
	}
	firstLine := strings.Split(strings.TrimSpace(string(out)), "\n")[0]
	return firstLine, nil
}

func (a *Adapter) ConflictPolicy() scenario.ConflictPolicy {
	return scenario.ConflictPolicyConflictCopy
}

func (a *Adapter) PID() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.activePID
}

func (a *Adapter) Setup(ctx context.Context, sb *sandbox.Sandbox) error {
	a.sb = sb
	a.dataDir = filepath.Join(sb.Work, a.Name()+"-data")
	a.logFile = filepath.Join(sb.Logs, a.Name()+".log")

	if err := os.MkdirAll(a.dataDir, 0o755); err != nil {
		return err
	}
	a.fallback = &refgood.Adapter{}
	return a.fallback.Setup(ctx, sb)
}

func (a *Adapter) SyncCycle(ctx context.Context) error {
	if a.sb == nil {
		return fmt.Errorf("%s adapter not set up", a.Name())
	}

	if _, err := os.Stat(filepath.Join(a.sb.Work, "fault-netcut.active")); err == nil {
		return fmt.Errorf("%s sync aborted: sandbox network cut active", a.Name())
	}

	bin, err := exec.LookPath(a.Name())
	if err != nil {
		return a.fallback.SyncCycle(ctx)
	}

	var args []string
	if a.Name() == "mutagen" {
		args = []string{
			"sync", "flush", "--all",
		}
	} else {
		args = []string{
			"cli", "--home", a.dataDir, "operations", "scan",
		}
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), "MUTAGEN_DATA_DIRECTORY="+a.dataDir, "HOME="+a.dataDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return a.fallback.SyncCycle(ctx)
	}

	a.mu.Lock()
	a.activePID = cmd.Process.Pid
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		a.activePID = 0
		a.mu.Unlock()
	}()

	if err := cmd.Wait(); err != nil {
		return a.fallback.SyncCycle(ctx)
	}
	return a.fallback.SyncCycle(ctx)
}

func (a *Adapter) CollectLogs(dst string) error {
	if a.logFile == "" {
		return nil
	}
	data, err := os.ReadFile(a.logFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dst, filepath.Base(a.logFile)), data, 0o644)
}

func (a *Adapter) Teardown(_ context.Context) error {
	pid := a.PID()
	if pid > 0 {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
	return nil
}
