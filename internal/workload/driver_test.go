package workload

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/Vishv07/syncfault/internal/journal"
	"github.com/Vishv07/syncfault/internal/sandbox"
	"github.com/Vishv07/syncfault/internal/scenario"
)

func TestDriverProducesByteIdenticalJournalsForSameSeed(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	spec := scenario.WorkloadSpec{
		Recipe:   "basic_crud",
		Depth:    3,
		Files:    8,
		FileSize: scenario.SizeRange{MinBytes: 256, MaxBytes: 2048},
		Sides:    []string{"A"},
	}

	runOnce := func(runID string) []byte {
		sb, err := sandbox.New(runID, false)
		if err != nil {
			t.Fatal(err)
		}
		defer sb.Teardown()

		j, err := journal.Open(runID, filepath.Join(sb.Work, "journal.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		defer j.Close()

		ops, err := scenario.ExpandWorkload(spec, 777)
		if err != nil {
			t.Fatal(err)
		}
		drv := &Driver{
			Seed:    777,
			Sandbox: sb,
			Journal: j,
		}
		if err := drv.Execute(context.Background(), ops); err != nil {
			t.Fatalf("Execute failed: %v", err)
		}
		raw, err := j.DeterministicJSONL()
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}

	j1 := runOnce("det-run-1")
	j2 := runOnce("det-run-2")
	if !bytes.Equal(j1, j2) {
		t.Fatalf("journals differed for identical seed:\n--- j1 ---\n%s\n--- j2 ---\n%s", string(j1), string(j2))
	}
}
