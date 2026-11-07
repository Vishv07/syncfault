package scenario

import (
	"fmt"
	"path/filepath"
)

func init() {
	RegisterRecipe("many_small_files", expandManySmallFiles)
	RegisterRecipe("one_huge_file", expandOneHugeFile)
}

func expandManySmallFiles(spec WorkloadSpec, stream *Stream) ([]PlannedOp, error) {
	side := primarySide(spec)
	fileCount := spec.Files
	if fileCount <= 0 {
		fileCount = 32
	}
	minB := spec.FileSize.MinBytes
	maxB := spec.FileSize.MaxBytes
	if maxB <= 0 || maxB > 4096 {
		minB = 64
		maxB = 2048
	}

	var ops []PlannedOp
	for i := 0; i < fileCount; i++ {
		bucket := fmt.Sprintf("tiny/bucket_%02d/note_%04d.txt", (i%4)+1, i+1)
		relPath := filepath.ToSlash(bucket)
		size := stream.Int64Range(minB, maxB)
		ops = append(ops, PlannedOp{
			Side: side,
			Kind: OpCreate,
			Path: relPath,
			Size: size,
		})
		if (i+1)%8 == 0 {
			ops = append(ops, PlannedOp{Side: side, Kind: OpSyncCycle})
		}
	}
	ops = append(ops, PlannedOp{Side: side, Kind: OpSyncCycle})
	return ops, nil
}

func expandOneHugeFile(spec WorkloadSpec, stream *Stream) ([]PlannedOp, error) {
	side := primarySide(spec)
	hugeSize := spec.FileSize.MaxBytes
	if hugeSize < 64*1024 {
		hugeSize = 256 * 1024
	}

	relPath := "blobs/iso_payload.bin"
	ops := []PlannedOp{
		{
			Side: side,
			Kind: OpCreate,
			Path: relPath,
			Size: hugeSize,
		},
		{
			Side: side,
			Kind: OpSyncCycle,
		},
		{
			Side: side,
			Kind: OpEdit,
			Path: relPath,
			Size: hugeSize + int64(stream.IntRange(1024, 8192)),
		},
		{
			Side: side,
			Kind: OpSyncCycle,
		},
	}
	return ops, nil
}
