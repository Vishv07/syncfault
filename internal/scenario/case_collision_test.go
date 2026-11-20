package scenario

import (
	"strings"
	"testing"
)

func TestCaseCollisionRecipeDeterminism(t *testing.T) {
	spec := WorkloadSpec{
		Recipe:   "case_collision",
		Files:    4,
		FileSize: SizeRange{MinBytes: 256, MaxBytes: 1024},
		Sides:    []string{"A"},
	}

	ops1, err := ExpandWorkload(spec, 77)
	if err != nil {
		t.Fatalf("ExpandWorkload failed: %v", err)
	}
	ops2, err := ExpandWorkload(spec, 77)
	if err != nil {
		t.Fatalf("ExpandWorkload second call failed: %v", err)
	}
	if len(ops1) == 0 || len(ops1) != len(ops2) {
		t.Fatalf("unexpected op counts: %d vs %d", len(ops1), len(ops2))
	}

	hasCaseRename := false
	for i := range ops1 {
		if ops1[i] != ops2[i] {
			t.Fatalf("op %d mismatch: %+v vs %+v", i, ops1[i], ops2[i])
		}
		if ops1[i].Kind == OpRename && strings.EqualFold(ops1[i].Path, ops1[i].TargetPath) && ops1[i].Path != ops1[i].TargetPath {
			hasCaseRename = true
		}
	}
	if !hasCaseRename {
		t.Fatal("expected at least one case-only rename in case_collision recipe")
	}
}
