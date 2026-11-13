package runner

import (
	"context"
	"testing"

	"github.com/Vishv07/syncfault/internal/scenario"
)

func TestExecuteSingleConcurrentEditWithConflictCopyCleanOnRefGood(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sc, err := scenario.LoadFile("../../scenarios/concurrent_edit_sigkill.yaml")
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}
	res, err := ExecuteSingle(context.Background(), RunConfig{
		AdapterName: "refgood",
		Scenario:    sc,
		Seed:        1,
	})
	if err != nil {
		t.Fatalf("ExecuteSingle failed: %v", err)
	}
	if res.Verdict != VerdictClean {
		t.Fatalf("expected CLEAN on refgood concurrent_edit, got %s (%s)", res.Verdict, res.Reason)
	}
}
