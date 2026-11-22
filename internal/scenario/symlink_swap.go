package scenario

import (
	"fmt"
	"path/filepath"
)

func init() {
	RegisterRecipe("symlink_swap", expandSymlinkSwap)
}

// expandSymlinkSwap models deployment directory swaps (such as releases/v1 -> releases/v2
// and active pointer/directory replacement) that frequently confuse sync staging caches.
func expandSymlinkSwap(spec WorkloadSpec, stream *Stream) ([]PlannedOp, error) {
	side := primarySide(spec)
	fileCount := spec.Files
	if fileCount <= 0 {
		fileCount = 6
	}

	var ops []PlannedOp
	var v1Paths []string

	// Phase 1: Populate releases/v1 and active/current directory, then sync
	for i := 0; i < fileCount; i++ {
		rel := filepath.ToSlash(fmt.Sprintf("releases/v1/asset_%03d.bundle", i+1))
		size := stream.Int64Range(spec.FileSize.MinBytes, spec.FileSize.MaxBytes)
		ops = append(ops, PlannedOp{
			Side: side,
			Kind: OpCreate,
			Path: rel,
			Size: size,
		})
		v1Paths = append(v1Paths, rel)
	}
	ops = append(ops,
		PlannedOp{
			Side: side,
			Kind: OpCreate,
			Path: "active/pointer.manifest",
			Size: stream.Int64Range(spec.FileSize.MinBytes, spec.FileSize.MaxBytes),
		},
		PlannedOp{Side: side, Kind: OpSyncCycle},
	)

	// Phase 2: Stage releases/v2 and atomically swap active pointer and release paths
	for i, oldRel := range v1Paths {
		newRel := filepath.ToSlash(fmt.Sprintf("releases/v2/asset_%03d.bundle", i+1))
		ops = append(ops, PlannedOp{
			Side:       side,
			Kind:       OpRename,
			Path:       oldRel,
			TargetPath: newRel,
		})
		if (i+1)%2 == 0 {
			ops = append(ops, PlannedOp{
				Side: side,
				Kind: OpEdit,
				Path: "active/pointer.manifest",
				Size: stream.Int64Range(spec.FileSize.MinBytes, spec.FileSize.MaxBytes),
			})
			ops = append(ops, PlannedOp{Side: side, Kind: OpSyncCycle})
		}
	}
	ops = append(ops, PlannedOp{Side: side, Kind: OpSyncCycle})

	return ops, nil
}
