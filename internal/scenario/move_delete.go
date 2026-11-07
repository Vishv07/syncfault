package scenario

import (
	"fmt"
	"path/filepath"
)

func init() {
	RegisterRecipe("move_tree", expandMoveTree)
	RegisterRecipe("mass_delete", expandMassDelete)
}

func expandMoveTree(spec WorkloadSpec, stream *Stream) ([]PlannedOp, error) {
	side := primarySide(spec)
	fileCount := spec.Files
	if fileCount <= 0 {
		fileCount = 10
	}

	var ops []PlannedOp
	var oldPaths []string

	for i := 0; i < fileCount; i++ {
		sub := fmt.Sprintf("project_alpha/module_%d/item_%03d.dat", (i%3)+1, i+1)
		relPath := filepath.ToSlash(sub)
		size := stream.Int64Range(spec.FileSize.MinBytes, spec.FileSize.MaxBytes)
		ops = append(ops, PlannedOp{
			Side: side,
			Kind: OpCreate,
			Path: relPath,
			Size: size,
		})
		oldPaths = append(oldPaths, relPath)
	}
	ops = append(ops, PlannedOp{Side: side, Kind: OpSyncCycle})

	for i, oldPath := range oldPaths {
		newPath := filepath.ToSlash(fmt.Sprintf("archive_omega/relocated_%d/item_%03d.dat", (i%3)+1, i+1))
		ops = append(ops, PlannedOp{
			Side:       side,
			Kind:       OpRename,
			Path:       oldPath,
			TargetPath: newPath,
		})
		if (i+1)%4 == 0 {
			ops = append(ops, PlannedOp{Side: side, Kind: OpSyncCycle})
		}
	}
	ops = append(ops, PlannedOp{Side: side, Kind: OpSyncCycle})
	return ops, nil
}

func expandMassDelete(spec WorkloadSpec, stream *Stream) ([]PlannedOp, error) {
	side := primarySide(spec)
	fileCount := spec.Files
	if fileCount <= 0 {
		fileCount = 12
	}

	var ops []PlannedOp
	var created []string

	for i := 0; i < fileCount; i++ {
		relPath := filepath.ToSlash(fmt.Sprintf("dataset/shard_%d/record_%03d.bin", (i%3)+1, i+1))
		size := stream.Int64Range(spec.FileSize.MinBytes, spec.FileSize.MaxBytes)
		ops = append(ops, PlannedOp{
			Side: side,
			Kind: OpCreate,
			Path: relPath,
			Size: size,
		})
		created = append(created, relPath)
	}
	ops = append(ops, PlannedOp{Side: side, Kind: OpSyncCycle})

	// Delete most files while keeping the last two intact
	keepCount := 2
	if len(created) <= keepCount {
		keepCount = 1
	}
	for i := 0; i < len(created)-keepCount; i++ {
		ops = append(ops, PlannedOp{
			Side: side,
			Kind: OpDelete,
			Path: created[i],
		})
		if (i+1)%5 == 0 {
			ops = append(ops, PlannedOp{Side: side, Kind: OpSyncCycle})
		}
	}
	ops = append(ops, PlannedOp{Side: side, Kind: OpSyncCycle})
	return ops, nil
}
