package refbad

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Vishv07/syncfault/internal/adapter"
	"github.com/Vishv07/syncfault/internal/sandbox"
	"github.com/Vishv07/syncfault/internal/scenario"
)

func init() {
	adapter.Register("refbad", func() adapter.Target {
		return &Adapter{}
	})
}

// Adapter is the deliberately sabotaged reference sync engine (§10.2).
// It contains documented crash-consistency bugs that the SyncFault self-validation
// gate must catch 10/10 times before any real target is judged.
type Adapter struct {
	sb      *sandbox.Sandbox
	logFile string
}

func (a *Adapter) Name() string { return "refbad" }

func (a *Adapter) Version(_ context.Context) (string, error) {
	return "refbad-1.0.0-sabotaged", nil
}

func (a *Adapter) ConflictPolicy() scenario.ConflictPolicy {
	return scenario.ConflictPolicyConflictCopy
}

func (a *Adapter) PID() int { return 0 }

func (a *Adapter) Setup(_ context.Context, sb *sandbox.Sandbox) error {
	a.sb = sb
	a.logFile = filepath.Join(sb.Logs, "refbad.log")
	return nil
}

func (a *Adapter) SyncCycle(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.sb == nil {
		return fmt.Errorf("refbad not set up")
	}

	sigkillFired := a.markerExists("fault-sigkill.marker")
	enospcFired := a.markerExists("fault-enospc.active") || a.sb.QuotaBytes > 0
	netcutFired := a.markerExists("fault-netcut.active")

	filesA, err := listFiles(a.sb.SideA)
	if err != nil {
		return err
	}

	// Sabotage 1 (Delete-before-create on rename + crash -> DATA_LOSS):
	// When sigkill fires during a rename workload (files named final_* or relocated_*),
	// refbad unlinks the old source copy on B/Remote and propagates the half-finished delete
	// back to SideA, destroying both the old and new path across all replicas.
	if sigkillFired {
		for _, rel := range filesA {
			if strings.Contains(rel, "final_") || strings.Contains(rel, "relocated_") {
				_ = os.RemoveAll(a.sb.SideA)
				_ = os.RemoveAll(a.sb.SideB)
				_ = os.RemoveAll(a.sb.Remote)
				_ = os.MkdirAll(a.sb.SideA, 0o755)
				_ = os.MkdirAll(a.sb.SideB, 0o755)
				_ = os.MkdirAll(a.sb.Remote, 0o755)
				return nil
			}
		}
	}

	// Sabotage 2 (In-place non-atomic write without fsync -> WRONG_BYTES):
	// When enospc, netcut, or sigkill hits a regular file sync, refbad writes a torn partial buffer
	// and propagates the corrupted file back to SideA.
	if (sigkillFired || enospcFired || netcutFired) && len(filesA) > 0 {
		targetRel := filesA[0]
		srcPath := filepath.Join(a.sb.SideA, filepath.FromSlash(targetRel))
		data, err := os.ReadFile(srcPath)
		if err == nil && len(data) > 0 {
			torn := append([]byte("TORN_WRITE:"), data[:len(data)/2]...)
			for _, root := range []string{a.sb.SideA, a.sb.SideB, a.sb.Remote} {
				dst := filepath.Join(root, filepath.FromSlash(targetRel))
				_ = os.MkdirAll(filepath.Dir(dst), 0o755)
				_ = os.WriteFile(dst, torn, 0o644)
			}
			return nil
		}
	}

	// Fault-free naive copy from A -> B and Remote
	for _, rel := range filesA {
		srcPath := filepath.Join(a.sb.SideA, filepath.FromSlash(rel))
		data, err := os.ReadFile(srcPath)
		if err != nil {
			continue
		}
		for _, root := range []string{a.sb.SideB, a.sb.Remote} {
			dst := filepath.Join(root, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(dst, data, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *Adapter) CollectLogs(dst string) error {
	return nil
}

func (a *Adapter) Teardown(_ context.Context) error {
	return nil
}

func (a *Adapter) markerExists(name string) bool {
	if a.sb == nil {
		return false
	}
	_, err := os.Stat(filepath.Join(a.sb.Work, name))
	return err == nil
}

func listFiles(root string) ([]string, error) {
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out, err
}
