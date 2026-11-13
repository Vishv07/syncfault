package scenario

import "testing"

func TestExpandConcurrentEditMutatesBothSides(t *testing.T) {
	ops, err := ExpandWorkload(WorkloadSpec{
		Recipe:   "concurrent_edit",
		Files:    6,
		FileSize: SizeRange{MinBytes: 128, MaxBytes: 1024},
		Sides:    []string{"A", "B"},
	}, 303)
	if err != nil {
		t.Fatalf("ExpandWorkload concurrent_edit failed: %v", err)
	}

	sawA, sawB := false, false
	for _, op := range ops {
		if op.Side == "A" && op.Kind == OpEdit {
			sawA = true
		}
		if op.Side == "B" && op.Kind == OpEdit {
			sawB = true
		}
	}
	if !sawA || !sawB {
		t.Fatalf("expected edits on both A and B, got sawA=%v sawB=%v", sawA, sawB)
	}
}
