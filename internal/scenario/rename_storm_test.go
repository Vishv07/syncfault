package scenario

import "testing"

func TestExpandRenameStorm(t *testing.T) {
	spec := WorkloadSpec{
		Recipe:   "rename_storm",
		Depth:    4,
		Files:    12,
		FileSize: SizeRange{MinBytes: 512, MaxBytes: 4096},
		Sides:    []string{"A"},
	}

	ops, err := ExpandWorkload(spec, 412)
	if err != nil {
		t.Fatalf("ExpandWorkload rename_storm failed: %v", err)
	}

	renames := 0
	for _, op := range ops {
		if op.Kind == OpRename {
			renames++
		}
	}
	if renames != 12 {
		t.Fatalf("expected 12 rename ops, got %d", renames)
	}
}
