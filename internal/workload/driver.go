package workload

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Vishv07/syncfault/internal/journal"
	"github.com/Vishv07/syncfault/internal/sandbox"
	"github.com/Vishv07/syncfault/internal/scenario"
)

// SyncHook is invoked when the workload schedule reaches an OpSyncCycle step.
type SyncHook func(ctx context.Context, opIndex int) error

// Driver executes PlannedOps against a confined Sandbox and logs every action to the Journal.
type Driver struct {
	Seed     uint64
	Sandbox  *sandbox.Sandbox
	Journal  *journal.Journal
	SyncHook SyncHook
}

// Execute runs all planned operations in order.
func (d *Driver) Execute(ctx context.Context, ops []scenario.PlannedOp) error {
	for _, op := range ops {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := d.ExecuteOp(ctx, op); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteOp performs a single PlannedOp inside the confined sandbox and records it in the journal.
func (d *Driver) ExecuteOp(ctx context.Context, op scenario.PlannedOp) error {
	side := op.Side
	if side == "" {
		side = "A"
	}

	switch op.Kind {
	case scenario.OpCreate, scenario.OpEdit:
		absPath, err := d.Sandbox.ConfineSide(side, op.Path)
		if err != nil {
			return err
		}
		beforeHash := fileHashIfExists(absPath)
		content := scenario.ContentBytes(d.Seed, op.OpIndex, op.Size)
		afterHash := scenario.HashBytes(content)

		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			return fmt.Errorf("mkdir parent for %s: %w", op.Path, err)
		}
		if err := os.WriteFile(absPath, content, 0o644); err != nil {
			return fmt.Errorf("write file %s: %w", op.Path, err)
		}

		return d.Journal.Append(journal.Entry{
			OpIndex:           op.OpIndex,
			Side:              side,
			Op:                string(op.Kind),
			Path:              op.Path,
			ContentHashBefore: beforeHash,
			ContentHashAfter:  afterHash,
			Bytes:             op.Size,
		})

	case scenario.OpRename:
		absSrc, err := d.Sandbox.ConfineSide(side, op.Path)
		if err != nil {
			return err
		}
		absDst, err := d.Sandbox.ConfineSide(side, op.TargetPath)
		if err != nil {
			return err
		}
		beforeHash := fileHashIfExists(absSrc)
		if err := os.MkdirAll(filepath.Dir(absDst), 0o755); err != nil {
			return fmt.Errorf("mkdir parent for rename target %s: %w", op.TargetPath, err)
		}
		if err := os.Rename(absSrc, absDst); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("rename %s -> %s: %w", op.Path, op.TargetPath, err)
		}
		afterHash := fileHashIfExists(absDst)

		return d.Journal.Append(journal.Entry{
			OpIndex:           op.OpIndex,
			Side:              side,
			Op:                string(scenario.OpRename),
			Path:              op.Path,
			TargetPath:        op.TargetPath,
			ContentHashBefore: beforeHash,
			ContentHashAfter:  afterHash,
		})

	case scenario.OpDelete:
		absPath, err := d.Sandbox.ConfineSide(side, op.Path)
		if err != nil {
			return err
		}
		beforeHash := fileHashIfExists(absPath)
		if err := os.RemoveAll(absPath); err != nil {
			return fmt.Errorf("delete %s: %w", op.Path, err)
		}
		return d.Journal.Append(journal.Entry{
			OpIndex:           op.OpIndex,
			Side:              side,
			Op:                string(scenario.OpDelete),
			Path:              op.Path,
			ContentHashBefore: beforeHash,
		})

	case scenario.OpMkdir:
		absPath, err := d.Sandbox.ConfineSide(side, op.Path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(absPath, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", op.Path, err)
		}
		return d.Journal.Append(journal.Entry{
			OpIndex: op.OpIndex,
			Side:    side,
			Op:      string(scenario.OpMkdir),
			Path:    op.Path,
		})

	case scenario.OpSyncCycle:
		if err := d.Journal.Append(journal.Entry{
			OpIndex: op.OpIndex,
			Side:    side,
			Op:      string(scenario.OpSyncCycle),
		}); err != nil {
			return err
		}
		if d.SyncHook != nil {
			return d.SyncHook(ctx, op.OpIndex)
		}
		return nil

	default:
		return fmt.Errorf("unsupported planned op kind %q", op.Kind)
	}
}

func fileHashIfExists(path string) string {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return scenario.HashBytes(data)
}
