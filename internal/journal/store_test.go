package journal

import (
	"path/filepath"
	"testing"
)

func TestStoreSaveGetAndListRuns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	st, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("OpenStore failed: %v", err)
	}

	run := StoredRun{
		RunID:        "2026-11-03-refgood-412",
		Adapter:      "refgood",
		ScenarioName: "rename_storm_sigkill",
		Recipe:       "rename_storm",
		FaultType:    "sigkill",
		Seed:         412,
		Verdict:      "CLEAN",
		Reason:       "converged cleanly",
		DurationMS:   185,
	}
	if err := st.SaveRun(run); err != nil {
		t.Fatalf("SaveRun failed: %v", err)
	}

	got, err := st.GetRun("2026-11-03-refgood-412")
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if got.Seed != 412 || got.Verdict != "CLEAN" {
		t.Fatalf("unexpected StoredRun: %+v", got)
	}

	all, err := st.ListRuns()
	if err != nil || len(all) != 1 {
		t.Fatalf("expected 1 run in ListRuns, got %d (err=%v)", len(all), err)
	}
}
