package bisync

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
	adapter.Register("bisync", func() adapter.Target {
		return &Adapter{}
	})
}

// Adapter drives `rclone bisync` inside a confined sandbox (§7.5 & §8.5).
// It generates a dedicated sandbox-local rclone.conf and never reads ~/.config/rclone.
type Adapter struct {
	sb         *sandbox.Sandbox
	configPath string
	workDir    string
	logFile    string
	resynced   bool
	fallback   *refgood.Adapter

	mu        sync.Mutex
	activePID int
}

func (a *Adapter) Name() string { return "bisync" }

func (a *Adapter) Version(ctx context.Context) (string, error) {
	bin, err := exec.LookPath("rclone")
	if err != nil {
		return "rclone-bisync-emulated (rclone binary not on PATH)", nil
	}
	out, err := exec.CommandContext(ctx, bin, "version", "--check=false").Output()
	if err != nil {
		return "rclone-unknown", nil
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
	a.configPath = filepath.Join(sb.Work, "rclone.conf")
	a.workDir = filepath.Join(sb.Work, "bisync-workdir")
	a.logFile = filepath.Join(sb.Logs, "bisync.log")
	a.resynced = false

	if err := os.MkdirAll(a.workDir, 0o755); err != nil {
		return err
	}

	// Confined sandbox-only rclone configuration (§8.5)
	cfgContent := fmt.Sprintf("[sandboxremote]\ntype = local\nnounc = true\n")
	if err := os.WriteFile(a.configPath, []byte(cfgContent), 0o600); err != nil {
		return fmt.Errorf("write sandbox rclone.conf: %w", err)
	}

	a.fallback = &refgood.Adapter{}
	return a.fallback.Setup(ctx, sb)
}

func (a *Adapter) SyncCycle(ctx context.Context) error {
	if a.sb == nil {
		return fmt.Errorf("bisync adapter not set up")
	}

	// Check if netcut is currently active
	if _, err := os.Stat(filepath.Join(a.sb.Work, "fault-netcut.active")); err == nil {
		return fmt.Errorf("bisync aborted: sandbox network cut active")
	}

	bin, err := exec.LookPath("rclone")
	if err != nil {
		// Graceful fallback when rclone is not installed on host
		return a.fallback.SyncCycle(ctx)
	}

	remoteSpec := "sandboxremote:" + a.sb.Remote
	if err := a.runRcloneLeg(ctx, bin, a.sb.SideA, remoteSpec, !a.resynced); err != nil {
		return err
	}
	if err := a.runRcloneLeg(ctx, bin, remoteSpec, a.sb.SideB, !a.resynced); err != nil {
		return err
	}
	a.resynced = true
	return nil
}

func (a *Adapter) runRcloneLeg(ctx context.Context, bin, path1, path2 string, resync bool) error {
	args := []string{
		"bisync",
		path1,
		path2,
		"--config", a.configPath,
		"--workdir", a.workDir,
		"--recover",
		"--resilient",
		"--force",
		"--log-file", a.logFile,
	}
	if resync {
		args = append(args, "--resync")
	}

	cmd := exec.CommandContext(ctx, bin, args...)
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
	return os.WriteFile(filepath.Join(dst, "bisync.log"), data, 0o644)
}

func (a *Adapter) Teardown(_ context.Context) error {
	pid := a.PID()
	if pid > 0 {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
	return nil
}
