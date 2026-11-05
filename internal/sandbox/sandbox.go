package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Vishv07/syncfault/internal/doctor"
)

// Sandbox manages the confined filesystem and namespace directories for a single run.
type Sandbox struct {
	RunID      string
	Root       string
	SideA      string
	SideB      string
	Remote     string
	Logs       string
	Work       string
	Keep       bool
	QuotaBytes int64

	mu       sync.Mutex
	tornDown bool
}

// New creates and initializes a confined sandbox directory tree for runID.
func New(runID string, keep bool) (*Sandbox, error) {
	if runID == "" {
		return nil, fmt.Errorf("runID must not be empty")
	}
	baseRoot := doctor.SandboxRoot()
	if err := os.MkdirAll(baseRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create sandbox base root: %w", err)
	}

	runRoot := filepath.Join(baseRoot, runID)
	if err := os.RemoveAll(runRoot); err != nil {
		return nil, fmt.Errorf("clear existing sandbox %s: %w", runRoot, err)
	}
	if err := os.MkdirAll(runRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create sandbox root %s: %w", runRoot, err)
	}

	realRoot, err := filepath.EvalSymlinks(runRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve sandbox root symlinks: %w", err)
	}

	sb := &Sandbox{
		RunID:  runID,
		Root:   realRoot,
		SideA:  filepath.Join(realRoot, "sideA"),
		SideB:  filepath.Join(realRoot, "sideB"),
		Remote: filepath.Join(realRoot, "remote"),
		Logs:   filepath.Join(realRoot, "logs"),
		Work:   filepath.Join(realRoot, "work"),
		Keep:   keep,
	}

	for _, dir := range []string{sb.SideA, sb.SideB, sb.Remote, sb.Logs, sb.Work} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			_ = os.RemoveAll(realRoot)
			return nil, fmt.Errorf("initialize sandbox subdir %s: %w", dir, err)
		}
	}

	return sb, nil
}

// Confine validates that a relative or absolute path stays strictly inside the sandbox root.
func (s *Sandbox) Confine(targetPath string) (string, error) {
	return ResolveAndConfine(s.Root, targetPath)
}

// ConfineSide resolves a relative path inside replica side "A", "B", or "remote" while enforcing confinement.
func (s *Sandbox) ConfineSide(side, relPath string) (string, error) {
	var sideRoot string
	switch side {
	case "A":
		sideRoot = s.SideA
	case "B":
		sideRoot = s.SideB
	case "remote":
		sideRoot = s.Remote
	default:
		return "", fmt.Errorf("unknown replica side %q", side)
	}
	resolved, err := ResolveAndConfine(sideRoot, relPath)
	if err != nil {
		return "", err
	}
	return ResolveAndConfine(s.Root, resolved)
}

// UsedBytes computes the total bytes currently stored across Remote and SideB for quota/enospc enforcement.
func (s *Sandbox) UsedBytes() (int64, error) {
	var total int64
	for _, dir := range []string{s.SideA, s.SideB, s.Remote} {
		err := filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if !info.IsDir() {
				total += info.Size()
			}
			return nil
		})
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

// Teardown removes the sandbox directory tree unless Keep is true. Safe to invoke multiple times.
func (s *Sandbox) Teardown() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.tornDown {
		return nil
	}
	s.tornDown = true
	if s.Keep {
		return nil
	}
	if err := os.RemoveAll(s.Root); err != nil {
		return fmt.Errorf("sandbox teardown failed for %s: %w", s.Root, err)
	}
	return nil
}
