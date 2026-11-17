package report

import (
	"strings"
	"testing"

	"github.com/Vishv07/syncfault/internal/journal"
)

func TestSummarizeAndRenderMarkdown(t *testing.T) {
	runs := []journal.StoredRun{
		{
			RunID:         "refgood-basic-1",
			Adapter:       "refgood",
			TargetVersion: "1.0.0",
			ScenarioName:  "basic_crud_none",
			FaultType:     "none",
			Seed:          1,
			Verdict:       "CLEAN",
			DurationMS:    120,
		},
		{
			RunID:         "refbad-rename-412",
			Adapter:       "refbad",
			TargetVersion: "1.0.0",
			ScenarioName:  "rename_storm_sigkill",
			FaultType:     "sigkill",
			Seed:          412,
			Verdict:       "DATA_LOSS",
			Reason:        "required file missing",
			DurationMS:    210,
		},
	}

	data := Summarize(runs)
	md := RenderMarkdown(data)

	if !strings.Contains(md, "SyncFault Correctness Scorecard") {
		t.Fatal("missing scorecard title")
	}
	if !strings.Contains(md, "syncfault replay --run-id refbad-rename-412 --replays 10") {
		t.Fatal("missing replay command for non-CLEAN finding")
	}
	if !strings.Contains(md, "Limitations (What Was Not Tested in v1)") {
		t.Fatal("missing mandatory Limitations section")
	}
}
