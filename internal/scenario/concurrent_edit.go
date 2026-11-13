package scenario

import (
	"fmt"
	"path/filepath"
)

func init() {
	RegisterRecipe("concurrent_edit", expandConcurrentEdit)
}

// expandConcurrentEdit deterministically interleaves conflicting and non-conflicting mutations
// across both side A and side B inside shared sync windows (§7.3).
func expandConcurrentEdit(spec WorkloadSpec, stream *Stream) ([]PlannedOp, error) {
	fileCount := spec.Files
	if fileCount <= 0 {
		fileCount = 8
	}

	var ops []PlannedOp
	var sharedPaths []string

	// Phase 1: Seed baseline files on Side A and sync so both replicas start with identical files
	for i := 0; i < fileCount; i++ {
		relPath := filepath.ToSlash(fmt.Sprintf("shared/doc_%03d.md", i+1))
		size := stream.Int64Range(spec.FileSize.MinBytes, spec.FileSize.MaxBytes)
		ops = append(ops, PlannedOp{
			Side: "A",
			Kind: OpCreate,
			Path: relPath,
			Size: size,
		})
		sharedPaths = append(sharedPaths, relPath)
	}
	ops = append(ops, PlannedOp{Side: "A", Kind: OpSyncCycle})

	// Phase 2: Mutate the same paths on both Side A and Side B before the next sync cycle
	for i, relPath := range sharedPaths {
		sizeA := stream.Int64Range(spec.FileSize.MinBytes, spec.FileSize.MaxBytes)
		sizeB := stream.Int64Range(spec.FileSize.MinBytes, spec.FileSize.MaxBytes)
		if sizeA == sizeB {
			sizeB += 17
		}
		if stream.IntRange(0, 1) == 0 {
			ops = append(ops,
				PlannedOp{Side: "A", Kind: OpEdit, Path: relPath, Size: sizeA},
				PlannedOp{Side: "B", Kind: OpEdit, Path: relPath, Size: sizeB},
			)
		} else {
			ops = append(ops,
				PlannedOp{Side: "B", Kind: OpEdit, Path: relPath, Size: sizeB},
				PlannedOp{Side: "A", Kind: OpEdit, Path: relPath, Size: sizeA},
			)
		}
		if (i+1)%3 == 0 {
			ops = append(ops, PlannedOp{Side: "A", Kind: OpSyncCycle})
		}
	}
	ops = append(ops, PlannedOp{Side: "A", Kind: OpSyncCycle})

	return ops, nil
}
