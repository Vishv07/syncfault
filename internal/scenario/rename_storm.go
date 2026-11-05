package scenario

import (
	"fmt"
	"path/filepath"
)

func init() {
	RegisterRecipe("rename_storm", expandRenameStorm)
}

func expandRenameStorm(spec WorkloadSpec, stream *Stream) ([]PlannedOp, error) {
	side := primarySide(spec)
	fileCount := spec.Files
	if fileCount <= 0 {
		fileCount = 12
	}
	depth := spec.Depth
	if depth <= 0 {
		depth = 4
	}

	var ops []PlannedOp
	var currentPaths []string

	// Populate a deep hierarchy of files
	for i := 0; i < fileCount; i++ {
		branch := fmt.Sprintf("branch_%d", (i%3)+1)
		sub := fmt.Sprintf("level_%d/sub_%d", (i%depth)+1, stream.IntRange(1, 2))
		relPath := filepath.ToSlash(filepath.Join(branch, sub, fmt.Sprintf("draft_%03d.docx", i+1)))
		size := stream.Int64Range(spec.FileSize.MinBytes, spec.FileSize.MaxBytes)

		ops = append(ops, PlannedOp{
			Side: side,
			Kind: OpCreate,
			Path: relPath,
			Size: size,
		})
		currentPaths = append(currentPaths, relPath)
	}
	ops = append(ops, PlannedOp{Side: side, Kind: OpSyncCycle})

	// Perform rapid file renames interleaved with sync cycles
	for i := 0; i < len(currentPaths); i++ {
		oldPath := currentPaths[i]
		dir := filepath.ToSlash(filepath.Dir(oldPath))
		newPath := filepath.ToSlash(filepath.Join(dir, fmt.Sprintf("final_%03d.docx", i+1)))
		ops = append(ops, PlannedOp{
			Side:       side,
			Kind:       OpRename,
			Path:       oldPath,
			TargetPath: newPath,
		})
		currentPaths[i] = newPath
		if (i+1)%4 == 0 {
			ops = append(ops, PlannedOp{Side: side, Kind: OpSyncCycle})
		}
	}
	ops = append(ops, PlannedOp{Side: side, Kind: OpSyncCycle})

	return ops, nil
}
