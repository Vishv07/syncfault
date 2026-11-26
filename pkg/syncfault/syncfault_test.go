package syncfault_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Vishv07/syncfault/internal/adapter/refgood"
	"github.com/Vishv07/syncfault/internal/sandbox"
	"github.com/Vishv07/syncfault/pkg/syncfault"
)

// refgoodSync drives the in-tree reference engine through the public SDK entry point,
// which proves SyncFunc is enough to plug in a real reconciliation implementation.
func refgoodSync(ctx context.Context, d syncfault.Dirs) error {
	root := filepath.Dir(d.SideA)
	sb := &sandbox.Sandbox{
		RunID:  "sdk-test",
		Root:   root,
		SideA:  d.SideA,
		SideB:  d.SideB,
		Remote: d.Remote,
		Work:   d.Work,
		Logs:   filepath.Join(root, "logs"),
	}
	a := &refgood.Adapter{}
	if err := a.Setup(ctx, sb); err != nil {
		return err
	}
	return a.SyncCycle(ctx)
}

func TestAssertCleanWithReferenceSync(t *testing.T) {
	syncfault.AssertClean(t, syncfault.Spec{
		Recipe:   "basic_crud",
		Seeds:    syncfault.SeedRange(1, 3),
		SyncFunc: refgoodSync,
	})
}

func TestAssertCleanWithReferenceSyncUnderSigkillAnchor(t *testing.T) {
	syncfault.AssertClean(t, syncfault.Spec{
		Recipe:   "rename_storm",
		Fault:    "sigkill",
		OpIndex:  6,
		Seeds:    syncfault.SeedRange(1, 3),
		SyncFunc: refgoodSync,
	})
}

func TestRunFlagsASyncThatDoesNothing(t *testing.T) {
	results, err := syncfault.Run(context.Background(), syncfault.Spec{
		Recipe:   "basic_crud",
		Seeds:    syncfault.SeedRange(1, 2),
		SyncFunc: func(context.Context, syncfault.Dirs) error { return nil },
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Clean() {
			t.Fatalf("seed %d: a no-op sync must not be CLEAN (got %s: %s)", r.Seed, r.Verdict, r.Reason)
		}
		if r.RunID == "" {
			t.Fatalf("seed %d: expected a run ID", r.Seed)
		}
	}
}

func TestRunRejectsBadSpecs(t *testing.T) {
	ok := func(context.Context, syncfault.Dirs) error { return nil }
	cases := []struct {
		name string
		spec syncfault.Spec
		want string
	}{
		{"missing func", syncfault.Spec{Recipe: "basic_crud"}, "SyncFunc"},
		{"missing recipe", syncfault.Spec{SyncFunc: ok}, "Recipe"},
		{"unknown recipe", syncfault.Spec{Recipe: "nope", SyncFunc: ok}, "unknown workload recipe"},
		{"unknown fault", syncfault.Spec{Recipe: "basic_crud", Fault: "nope", SyncFunc: ok}, "unknown fault injector"},
		{"bad policy", syncfault.Spec{Recipe: "basic_crud", Policy: "yolo", SyncFunc: ok}, "Policy"},
	}
	for _, tc := range cases {
		_, err := syncfault.Run(context.Background(), tc.spec)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: expected error containing %q, got %v", tc.name, tc.want, err)
		}
	}
}

func TestSeedRangeAndCatalog(t *testing.T) {
	if got := syncfault.SeedRange(3, 5); len(got) != 3 || got[0] != 3 || got[2] != 5 {
		t.Fatalf("unexpected SeedRange(3,5): %v", got)
	}
	if got := syncfault.SeedRange(5, 3); got != nil {
		t.Fatalf("expected nil for inverted range, got %v", got)
	}
	recipes, faults := syncfault.Catalog()
	if len(recipes) < 10 || len(faults) < 8 {
		t.Fatalf("catalog looks incomplete: %d recipes, %d faults", len(recipes), len(faults))
	}
}
