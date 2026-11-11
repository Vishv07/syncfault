package runner

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Vishv07/syncfault/internal/journal"
	"github.com/Vishv07/syncfault/internal/scenario"
)

func TestReplayRunReproducesIdenticalVerdict(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	st, err := journal.OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	rawYAML := `
name: rename_sigkill_replay
description: Replay test scenario
workload:
  recipe: rename_storm
  params:
    depth: 3
    files: 6
    file_size: { min: 128B, max: 512B }
  sides: [A]
fault:
  type: sigkill
  at:
    op_index: 8
runs:
  seeds: [412]
  timeout_s: 30
`
	sc, err := scenario.Parse(rawYAML)
	if err != nil {
		t.Fatal(err)
	}

	res, err := ExecuteSingle(context.Background(), RunConfig{
		RunID:       "2026-11-03-refbad-412",
		AdapterName: "refbad",
		Scenario:    sc,
		Seed:        412,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := PersistResult(st, res, rawYAML); err != nil {
		t.Fatal(err)
	}

	rep, err := ReplayRun(context.Background(), st, "2026-11-03-refbad-412", "", 3)
	if err != nil {
		t.Fatalf("ReplayRun failed: %v", err)
	}
	if rep.Flaky || rep.MatchedCount != 3 || rep.Original != VerdictDataLoss {
		t.Fatalf("expected 3/3 deterministic DATA_LOSS replays, got %+v", rep)
	}
}
