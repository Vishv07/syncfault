// Package cmdexec is a zero-code target adapter. It tests any command-line sync tool by running a
// user-supplied shell command once per sync cycle, with the sandbox paths substituted in.
//
//	rsync -a {sideA}/ {sideB}/ && rsync -a {sideB}/ {sideA}/
//
// Placeholders: {sideA} {sideB} {remote} {work}. The same values are exported to the command as
// SYNCFAULT_SIDE_A, SYNCFAULT_SIDE_B, SYNCFAULT_REMOTE and SYNCFAULT_WORK. Substituted paths are
// single-quoted for the shell.
package cmdexec

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
	"github.com/Vishv07/syncfault/internal/sandbox"
	"github.com/Vishv07/syncfault/internal/scenario"
)

// Adapter runs Command through `sh -c` for every SyncCycle.
type Adapter struct {
	Command string
	Policy  scenario.ConflictPolicy

	sb      *sandbox.Sandbox
	logFile string

	mu        sync.Mutex
	activePID int
}

var _ adapter.Target = (*Adapter)(nil)

// New returns an adapter that runs command each sync cycle.
func New(command string) *Adapter {
	return &Adapter{Command: command}
}

// Validate checks that the command is non-empty and only uses known placeholders.
func Validate(command string) error {
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("sync command is empty")
	}
	rest := command
	for {
		i := strings.Index(rest, "{")
		if i < 0 {
			return nil
		}
		j := strings.Index(rest[i:], "}")
		if j < 0 {
			return nil
		}
		token := rest[i : i+j+1]
		switch token {
		case "{sideA}", "{sideB}", "{remote}", "{work}":
		default:
			if isPlaceholderLike(token) {
				return fmt.Errorf("unknown placeholder %s (use {sideA} {sideB} {remote} {work})", token)
			}
		}
		rest = rest[i+j+1:]
	}
}

// isPlaceholderLike keeps shell brace syntax such as ${VAR} or {a,b} from being flagged.
func isPlaceholderLike(token string) bool {
	inner := strings.TrimSuffix(strings.TrimPrefix(token, "{"), "}")
	if inner == "" {
		return false
	}
	for _, r := range inner {
		isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if !isLetter {
			return false
		}
	}
	return strings.HasPrefix(inner, "side") || inner == "remote" || inner == "work" || inner == "src" || inner == "dst"
}

func (a *Adapter) Name() string { return "cmd" }

func (a *Adapter) Version(context.Context) (string, error) {
	fields := strings.Fields(a.Command)
	if len(fields) == 0 {
		return "cmd", nil
	}
	return "cmd:" + filepath.Base(fields[0]), nil
}

func (a *Adapter) ConflictPolicy() scenario.ConflictPolicy {
	if a.Policy == "" || a.Policy == scenario.ConflictPolicyDefault {
		return scenario.ConflictPolicyConflictCopy
	}
	return a.Policy
}

func (a *Adapter) PID() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.activePID
}

func (a *Adapter) Setup(_ context.Context, sb *sandbox.Sandbox) error {
	if err := Validate(a.Command); err != nil {
		return err
	}
	a.sb = sb
	a.logFile = filepath.Join(sb.Logs, "cmd.log")
	return nil
}

func (a *Adapter) expand() string {
	return strings.NewReplacer(
		"{sideA}", shellQuote(a.sb.SideA),
		"{sideB}", shellQuote(a.sb.SideB),
		"{remote}", shellQuote(a.sb.Remote),
		"{work}", shellQuote(a.sb.Work),
	).Replace(a.Command)
}

func (a *Adapter) SyncCycle(ctx context.Context) error {
	if a.sb == nil {
		return fmt.Errorf("cmd adapter not set up")
	}
	if _, err := os.Stat(filepath.Join(a.sb.Work, "fault-netcut.active")); err == nil {
		return fmt.Errorf("cmd sync aborted: sandbox network cut active")
	}

	log, err := os.OpenFile(a.logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer log.Close()

	expanded := a.expand()
	fmt.Fprintf(log, "$ %s\n", expanded)

	cmd := exec.CommandContext(ctx, "sh", "-c", expanded)
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.Dir = a.sb.Work
	cmd.Env = append(os.Environ(),
		"SYNCFAULT_SIDE_A="+a.sb.SideA,
		"SYNCFAULT_SIDE_B="+a.sb.SideB,
		"SYNCFAULT_REMOTE="+a.sb.Remote,
		"SYNCFAULT_WORK="+a.sb.Work,
	)
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

	if err := cmd.Wait(); err != nil {
		fmt.Fprintf(log, "exit: %v\n", err)
		return err
	}
	return nil
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
	return os.WriteFile(filepath.Join(dst, "cmd.log"), data, 0o644)
}

func (a *Adapter) Teardown(context.Context) error {
	if pid := a.PID(); pid > 0 {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
