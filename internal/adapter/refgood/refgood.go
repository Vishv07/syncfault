package refgood

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/Vishv07/syncfault/internal/adapter"
	"github.com/Vishv07/syncfault/internal/sandbox"
	"github.com/Vishv07/syncfault/internal/scenario"
)

func init() {
	adapter.Register("refgood", func() adapter.Target {
		return &Adapter{}
	})
}

// Adapter is the correct in-tree reference two-way sync engine (§10.2).
// It uses atomic temp-file writes, fsync discipline, baseline state tracking,
// and conflict copy preservation on concurrent modifications.
type Adapter struct {
	sb        *sandbox.Sandbox
	stateFile string
	logFile   string
}

func (a *Adapter) Name() string { return "refgood" }

func (a *Adapter) Version(_ context.Context) (string, error) {
	return "refgood-1.0.0-builtin", nil
}

func (a *Adapter) ConflictPolicy() scenario.ConflictPolicy {
	return scenario.ConflictPolicyConflictCopy
}

func (a *Adapter) PID() int { return 0 }

func (a *Adapter) Setup(_ context.Context, sb *sandbox.Sandbox) error {
	a.sb = sb
	a.stateFile = filepath.Join(sb.Work, "refgood_baseline.json")
	a.logFile = filepath.Join(sb.Logs, "refgood.log")
	return a.logf("setup completed for sandbox %s", sb.RunID)
}

func (a *Adapter) SyncCycle(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a.sb == nil {
		return fmt.Errorf("refgood not set up")
	}

	baseline, err := a.loadBaseline()
	if err != nil {
		return err
	}

	treeA, err := scanReplica(a.sb.SideA)
	if err != nil {
		return err
	}
	treeB, err := scanReplica(a.sb.SideB)
	if err != nil {
		return err
	}

	allPathsSet := make(map[string]struct{})
	for p := range baseline {
		allPathsSet[p] = struct{}{}
	}
	for p := range treeA {
		allPathsSet[p] = struct{}{}
	}
	for p := range treeB {
		allPathsSet[p] = struct{}{}
	}

	allPaths := make([]string, 0, len(allPathsSet))
	for p := range allPathsSet {
		allPaths = append(allPaths, p)
	}
	sort.Strings(allPaths)

	newBaseline := make(map[string]string)

	for _, rel := range allPaths {
		baseHash, inBase := baseline[rel]
		entryA, inA := treeA[rel]
		entryB, inB := treeB[rel]

		switch {
		case inA && inB:
			if entryA.hash == entryB.hash {
				// Both replicas agree; mirror to remote
				if err := a.atomicWrite("remote", rel, entryA.data); err != nil {
					return err
				}
				newBaseline[rel] = entryA.hash
			} else if inBase && entryA.hash == baseHash {
				// B modified; propagate B -> A and remote
				if err := a.atomicWrite("A", rel, entryB.data); err != nil {
					return err
				}
				if err := a.atomicWrite("remote", rel, entryB.data); err != nil {
					return err
				}
				newBaseline[rel] = entryB.hash
			} else if inBase && entryB.hash == baseHash {
				// A modified; propagate A -> B and remote
				if err := a.atomicWrite("B", rel, entryA.data); err != nil {
					return err
				}
				if err := a.atomicWrite("remote", rel, entryA.data); err != nil {
					return err
				}
				newBaseline[rel] = entryA.hash
			} else {
				// Concurrent edit on both A and B: keep A at canonical path and save B as conflict copy
				conflictRel := conflictCopyPath(rel)
				if err := a.atomicWrite("B", rel, entryA.data); err != nil {
					return err
				}
				if err := a.atomicWrite("remote", rel, entryA.data); err != nil {
					return err
				}
				if err := a.atomicWrite("A", conflictRel, entryB.data); err != nil {
					return err
				}
				if err := a.atomicWrite("B", conflictRel, entryB.data); err != nil {
					return err
				}
				if err := a.atomicWrite("remote", conflictRel, entryB.data); err != nil {
					return err
				}
				newBaseline[rel] = entryA.hash
				newBaseline[conflictRel] = entryB.hash
			}

		case inA && !inB:
			if inBase && entryA.hash == baseHash {
				// Deleted on B; propagate delete to A and remote
				if err := a.removePath("A", rel); err != nil {
					return err
				}
				if err := a.removePath("remote", rel); err != nil {
					return err
				}
			} else {
				// Created on A (or edited on A while deleted on B: preserve A)
				if err := a.atomicWrite("B", rel, entryA.data); err != nil {
					return err
				}
				if err := a.atomicWrite("remote", rel, entryA.data); err != nil {
					return err
				}
				newBaseline[rel] = entryA.hash
			}

		case !inA && inB:
			if inBase && entryB.hash == baseHash {
				// Deleted on A; propagate delete to B and remote
				if err := a.removePath("B", rel); err != nil {
					return err
				}
				if err := a.removePath("remote", rel); err != nil {
					return err
				}
			} else {
				// Created on B (or edited on B while deleted on A: preserve B)
				if err := a.atomicWrite("A", rel, entryB.data); err != nil {
					return err
				}
				if err := a.atomicWrite("remote", rel, entryB.data); err != nil {
					return err
				}
				newBaseline[rel] = entryB.hash
			}
		}
	}

	if err := a.saveBaseline(newBaseline); err != nil {
		return err
	}
	return a.logf("sync cycle completed cleanly (%d files tracked)", len(newBaseline))
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
	return os.WriteFile(filepath.Join(dst, "refgood.log"), data, 0o644)
}

func (a *Adapter) Teardown(_ context.Context) error {
	return nil
}

type fileSnapshot struct {
	hash string
	data []byte
}

func scanReplica(root string) (map[string]fileSnapshot, error) {
	out := make(map[string]fileSnapshot)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if strings.HasPrefix(base, ".tmp-syncfault-") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = fileSnapshot{
			hash: scenario.HashBytes(data),
			data: data,
		}
		return nil
	})
	return out, err
}

func (a *Adapter) atomicWrite(side, rel string, data []byte) error {
	dst, err := a.sb.ConfineSide(side, rel)
	if err != nil {
		return err
	}
	if existing, err := os.ReadFile(dst); err == nil && bytes.Equal(existing, data) {
		return nil
	}

	if a.sb.QuotaBytes > 0 {
		used, err := a.sb.UsedBytes()
		if err != nil {
			return err
		}
		if used+int64(len(data)) > a.sb.QuotaBytes {
			return fmt.Errorf("write %s on %s: %w", rel, side, syscall.ENOSPC)
		}
	}

	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmpPath := filepath.Join(dir, ".tmp-syncfault-"+filepath.Base(dst))
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, dst)
}

func (a *Adapter) removePath(side, rel string) error {
	dst, err := a.sb.ConfineSide(side, rel)
	if err != nil {
		return err
	}
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (a *Adapter) loadBaseline() (map[string]string, error) {
	data, err := os.ReadFile(a.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, err
	}
	out := make(map[string]string)
	if err := json.Unmarshal(data, &out); err != nil {
		return make(map[string]string), nil
	}
	return out, nil
}

func (a *Adapter) saveBaseline(m map[string]string) error {
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := a.stateFile + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, a.stateFile)
}

func (a *Adapter) logf(format string, args ...any) error {
	f, err := os.OpenFile(a.logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, format+"\n", args...)
	return err
}

func conflictCopyPath(rel string) string {
	ext := filepath.Ext(rel)
	base := strings.TrimSuffix(rel, ext)
	return base + ".sync-conflict-B" + ext
}
