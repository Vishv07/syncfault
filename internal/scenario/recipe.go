package scenario

import (
	"fmt"
	"path/filepath"
)

// OpKind identifies the type of operation emitted by a workload recipe.
type OpKind string

const (
	OpCreate    OpKind = "create"
	OpEdit      OpKind = "edit"
	OpRename    OpKind = "rename"
	OpDelete    OpKind = "delete"
	OpMkdir     OpKind = "mkdir"
	OpSyncCycle OpKind = "synccycle"
)

// PlannedOp represents a single deterministic operation in an expanded workload sequence.
type PlannedOp struct {
	OpIndex    int
	Side       string
	Kind       OpKind
	Path       string
	TargetPath string
	Size       int64
}

// RecipeFunc expands a WorkloadSpec and deterministic Stream into an ordered slice of PlannedOps.
type RecipeFunc func(spec WorkloadSpec, stream *Stream) ([]PlannedOp, error)

var recipeRegistry = map[string]RecipeFunc{
	"basic_crud": expandBasicCRUD,
}

// RegisterRecipe registers a named workload recipe generator.
func RegisterRecipe(name string, fn RecipeFunc) {
	recipeRegistry[name] = fn
}

// ExpandWorkload generates the deterministic sequence of PlannedOps for the scenario and seed.
func ExpandWorkload(spec WorkloadSpec, seed uint64) ([]PlannedOp, error) {
	fn, ok := recipeRegistry[spec.Recipe]
	if !ok {
		return nil, fmt.Errorf("unknown workload recipe %q", spec.Recipe)
	}
	stream := NewStream(seed)
	ops, err := fn(spec, stream)
	if err != nil {
		return nil, err
	}
	for i := range ops {
		ops[i].OpIndex = i + 1
	}
	return ops, nil
}

func primarySide(spec WorkloadSpec) string {
	if len(spec.Sides) > 0 && spec.Sides[0] != "" {
		return spec.Sides[0]
	}
	return "A"
}

func expandBasicCRUD(spec WorkloadSpec, stream *Stream) ([]PlannedOp, error) {
	side := primarySide(spec)
	fileCount := spec.Files
	if fileCount <= 0 {
		fileCount = 8
	}
	depth := spec.Depth
	if depth <= 0 {
		depth = 2
	}

	var ops []PlannedOp
	var activePaths []string

	// Phase 1: Create initial files across directories
	for i := 0; i < fileCount; i++ {
		dirParts := make([]string, 0, depth)
		for d := 0; d < stream.IntRange(1, depth); d++ {
			dirParts = append(dirParts, fmt.Sprintf("dir_%d", stream.IntRange(1, 3)))
		}
		relDir := filepath.Join(dirParts...)
		fileName := fmt.Sprintf("doc_%03d.txt", i+1)
		fullRel := filepath.ToSlash(filepath.Join(relDir, fileName))
		size := stream.Int64Range(spec.FileSize.MinBytes, spec.FileSize.MaxBytes)

		ops = append(ops, PlannedOp{
			Side: side,
			Kind: OpCreate,
			Path: fullRel,
			Size: size,
		})
		activePaths = append(activePaths, fullRel)

		if (i+1)%4 == 0 {
			ops = append(ops, PlannedOp{Side: side, Kind: OpSyncCycle})
		}
	}

	// Phase 2: Edit a subset of files
	editCount := max(1, len(activePaths)/2)
	for i := 0; i < editCount; i++ {
		idx := stream.IntRange(0, len(activePaths)-1)
		size := stream.Int64Range(spec.FileSize.MinBytes, spec.FileSize.MaxBytes)
		ops = append(ops, PlannedOp{
			Side: side,
			Kind: OpEdit,
			Path: activePaths[idx],
			Size: size,
		})
	}
	ops = append(ops, PlannedOp{Side: side, Kind: OpSyncCycle})

	// Phase 3: Rename and delete a couple of files
	if len(activePaths) >= 2 {
		oldPath := activePaths[0]
		newPath := oldPath + ".renamed"
		ops = append(ops, PlannedOp{
			Side:       side,
			Kind:       OpRename,
			Path:       oldPath,
			TargetPath: newPath,
		})
		activePaths[0] = newPath

		delPath := activePaths[len(activePaths)-1]
		ops = append(ops, PlannedOp{
			Side: side,
			Kind: OpDelete,
			Path: delPath,
		})
	}
	ops = append(ops, PlannedOp{Side: side, Kind: OpSyncCycle})

	return ops, nil
}
