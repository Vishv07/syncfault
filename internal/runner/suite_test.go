package runner

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Vishv07/syncfault/internal/journal"
)

func TestRunSuiteParallelWorkersOnRefGood(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	dbPath := filepath.Join(t.TempDir(), "runs.db")
	st, err := journal.OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	sum, err := RunSuite(context.Background(), SuiteOptions{
		AdapterName:  "refgood",
		SuiteName:    "smoke",
		ScenariosDir: "../../scenarios",
		Store:        st,
		Jobs:         4,
	})
	if err != nil {
		t.Fatalf("RunSuite failed: %v", err)
	}
	if sum.TotalRuns == 0 || sum.Worst != VerdictClean {
		t.Fatalf("expected all smoke runs on refgood to be CLEAN, got %+v", sum)
	}
}
