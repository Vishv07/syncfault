package scenario

import (
	"reflect"
	"testing"
)

func TestExpandBasicCRUDDeterministic(t *testing.T) {
	spec := WorkloadSpec{
		Recipe:   "basic_crud",
		Depth:    3,
		Files:    8,
		FileSize: SizeRange{MinBytes: 512, MaxBytes: 4096},
		Sides:    []string{"A"},
	}

	ops1, err := ExpandWorkload(spec, 99)
	if err != nil {
		t.Fatalf("ExpandWorkload 1 failed: %v", err)
	}
	ops2, err := ExpandWorkload(spec, 99)
	if err != nil {
		t.Fatalf("ExpandWorkload 2 failed: %v", err)
	}

	if !reflect.DeepEqual(ops1, ops2) {
		t.Fatal("expected byte-identical PlannedOp slices for identical seed")
	}
	if len(ops1) < 10 {
		t.Fatalf("expected at least 10 planned ops, got %d", len(ops1))
	}
}
