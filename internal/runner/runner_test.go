package runner

import (
	"context"
	"testing"

	"github.com/Vishv07/syncfault/internal/scenario"
)

func TestExecuteSingleRefGoodBasicCRUDClean(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	sc, err := scenario.Parse(`
name: basic_crud_none
description: Basic CRUD with no fault on refgood.
workload:
  recipe: basic_crud
  params:
    depth: 2
    files: 6
    file_size: { min: 128B, max: 1KiB }
  sides: [A]
fault:
  type: none
expect:
  conflict_policy: default
runs:
  seeds: [1]
  timeout_s: 30
`)
	if err != nil {
		t.Fatal(err)
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
		t.Fatalf("expected CLEAN verdict, got %s (%s)", res.Verdict, res.Reason)
	}
}
