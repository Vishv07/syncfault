package scenario

import "testing"

func TestExpandMoveTreeAndMassDelete(t *testing.T) {
	for _, recipe := range []string{"move_tree", "mass_delete"} {
		ops, err := ExpandWorkload(WorkloadSpec{
			Recipe:   recipe,
			Files:    10,
			FileSize: SizeRange{MinBytes: 256, MaxBytes: 1024},
			Sides:    []string{"A"},
		}, 101)
		if err != nil {
			t.Fatalf("ExpandWorkload(%s) failed: %v", recipe, err)
		}
		if len(ops) < 15 {
			t.Fatalf("expected >= 15 ops for %s, got %d", recipe, len(ops))
		}
	}
}
