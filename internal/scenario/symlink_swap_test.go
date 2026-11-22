package scenario

import (
	"testing"
)

func TestSymlinkSwapRecipeDeterminism(t *testing.T) {
	spec := WorkloadSpec{
		Recipe:   "symlink_swap",
		Files:    4,
		FileSize: SizeRange{MinBytes: 256, MaxBytes: 1024},
		Sides:    []string{"A"},
	}

	ops1, err := ExpandWorkload(spec, 55)
	if err != nil {
		t.Fatalf("ExpandWorkload failed: %v", err)
	}
	ops2, err := ExpandWorkload(spec, 55)
	if err != nil {
		t.Fatalf("ExpandWorkload second call failed: %v", err)
	}
	if len(ops1) == 0 || len(ops1) != len(ops2) {
		t.Fatalf("unexpected op counts: %d vs %d", len(ops1), len(ops2))
	}
	for i := range ops1 {
		if ops1[i] != ops2[i] {
			t.Fatalf("op %d mismatch: %+v vs %+v", i, ops1[i], ops2[i])
		}
	}
}
