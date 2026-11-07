package scenario

import "testing"

func TestExpandManySmallAndOneHugeFile(t *testing.T) {
	smallOps, err := ExpandWorkload(WorkloadSpec{
		Recipe: "many_small_files",
		Files:  16,
	}, 55)
	if err != nil || len(smallOps) < 16 {
		t.Fatalf("unexpected many_small_files ops: len=%d err=%v", len(smallOps), err)
	}

	hugeOps, err := ExpandWorkload(WorkloadSpec{
		Recipe:   "one_huge_file",
		FileSize: SizeRange{MinBytes: 65536, MaxBytes: 131072},
	}, 55)
	if err != nil || len(hugeOps) != 4 {
		t.Fatalf("unexpected one_huge_file ops: len=%d err=%v", len(hugeOps), err)
	}
}
