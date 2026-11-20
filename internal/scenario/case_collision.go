package scenario

import (
	"fmt"
	"path/filepath"
	"strings"
)

func init() {
	RegisterRecipe("case_collision", expandCaseCollision)
}

// expandCaseCollision generates case-only renames (such as Report_001.txt -> report_001.txt)
// and mixed-case directory mutations to test replica safety across case-sensitive and
// case-insensitive filesystems.
func expandCaseCollision(spec WorkloadSpec, stream *Stream) ([]PlannedOp, error) {
	side := primarySide(spec)
	fileCount := spec.Files
	if fileCount <= 0 {
		fileCount = 6
	}

	var ops []PlannedOp
	var created []string

	// Phase 1: Create mixed-case files on primary side and sync
	for i := 0; i < fileCount; i++ {
		rel := filepath.ToSlash(fmt.Sprintf("CaseDir/Report_%03d.txt", i+1))
		size := stream.Int64Range(spec.FileSize.MinBytes, spec.FileSize.MaxBytes)
		ops = append(ops, PlannedOp{
			Side: side,
			Kind: OpCreate,
			Path: rel,
			Size: size,
		})
		created = append(created, rel)
	}
	ops = append(ops, PlannedOp{Side: side, Kind: OpSyncCycle})

	// Phase 2: Perform case-only renames and content edits
	for i, orig := range created {
		dir := filepath.Dir(orig)
		base := filepath.Base(orig)
		var targetBase string
		if i%2 == 0 {
			targetBase = strings.ToLower(base)
		} else {
			targetBase = strings.ToUpper(base)
		}
		targetRel := filepath.ToSlash(filepath.Join(dir, targetBase))
		ops = append(ops, PlannedOp{
			Side:       side,
			Kind:       OpRename,
			Path:       orig,
			TargetPath: targetRel,
		})

		size := stream.Int64Range(spec.FileSize.MinBytes, spec.FileSize.MaxBytes)
		ops = append(ops, PlannedOp{
			Side: side,
			Kind: OpEdit,
			Path: targetRel,
			Size: size,
		})

		if (i+1)%2 == 0 {
			ops = append(ops, PlannedOp{Side: side, Kind: OpSyncCycle})
		}
	}
	ops = append(ops, PlannedOp{Side: side, Kind: OpSyncCycle})

	return ops, nil
}
