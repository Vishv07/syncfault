// Package syncfault is the public Go SDK for SyncFault. It lets any Go project fault-test
// its own sync function inside a normal `go test` run, without writing an adapter.
//
//	func TestMySyncSurvivesFaults(t *testing.T) {
//	    syncfault.AssertClean(t, syncfault.Spec{
//	        Recipe:   "case_collision",
//	        Fault:    "volatile_write",
//	        OpIndex:  6,
//	        Seeds:    syncfault.SeedRange(1, 10),
//	        SyncFunc: func(ctx context.Context, d syncfault.Dirs) error {
//	            return mySync.RunOnce(ctx, d.SideA, d.SideB)
//	        },
//	    })
//	}
package syncfault

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Vishv07/syncfault/internal/adapter"
	"github.com/Vishv07/syncfault/internal/runner"
	"github.com/Vishv07/syncfault/internal/sandbox"
	"github.com/Vishv07/syncfault/internal/scenario"
)

// Dirs are the confined directories handed to SyncFunc on every sync cycle.
type Dirs struct {
	SideA  string
	SideB  string
	Remote string
	Work   string
}

// SyncFunc performs one full reconciliation between SideA and SideB (optionally via Remote).
// It is called once for the baseline, once per sync step in the workload, and again during
// the recovery loop after the fault heals. Returning an error is allowed while a fault is active.
type SyncFunc func(ctx context.Context, d Dirs) error

// Spec describes one fault-test: a workload recipe, a fault, and the seeds to run.
type Spec struct {
	// Name labels the scenario in run IDs. Defaults to "<recipe>_<fault>".
	Name string
	// Recipe is a registered workload recipe (see Catalog).
	Recipe string
	// Fault is a registered fault injector, or "none" (default).
	Fault string
	// FaultParams are passed to the injector (for example {"skew_s": "-300"}).
	FaultParams map[string]string
	// OpIndex anchors the fault to a journal operation index. Ignored for fault "none".
	OpIndex int
	// Files and Depth tune the workload size. Zero uses the recipe defaults.
	Files int
	Depth int
	// Sides lists the replicas the workload mutates: ["A"] or ["A", "B"].
	Sides []string
	// Policy is the conflict policy your sync claims: "default", "lww" or "conflict_copy".
	Policy string
	// Seeds are the deterministic seeds to run. Defaults to 1 through 5.
	Seeds []uint64
	// Timeout bounds each seed. Defaults to 60 seconds.
	Timeout time.Duration
	// SyncFunc is the code under test.
	SyncFunc SyncFunc
}

// Result is the outcome of one seed.
type Result struct {
	Seed     uint64
	RunID    string
	Verdict  string
	Reason   string
	Flags    []string
	Duration time.Duration
}

// Clean reports whether the seed converged to an acceptable state.
func (r Result) Clean() bool { return r.Verdict == string(runner.VerdictClean) }

// SeedRange returns the seeds from..to inclusive.
func SeedRange(from, to uint64) []uint64 {
	if to < from {
		return nil
	}
	out := make([]uint64, 0, to-from+1)
	for s := from; s <= to; s++ {
		out = append(out, s)
	}
	return out
}

// Catalog lists the workload recipes and fault injectors available to a Spec.
func Catalog() (recipes, faults []string) {
	return runner.Recipes(), runner.Faults()
}

// Run executes every seed in spec and returns one Result per seed. A non-nil error means the
// harness itself failed (bad spec, sandbox error), never that the sync code misbehaved.
func Run(ctx context.Context, spec Spec) ([]Result, error) {
	sc, seeds, err := buildScenario(spec)
	if err != nil {
		return nil, err
	}
	policy := sc.Expect.ConflictPolicy

	results := make([]Result, 0, len(seeds))
	for _, seed := range seeds {
		target := &funcTarget{fn: spec.SyncFunc, policy: policy}
		res, err := runner.ExecuteSingle(ctx, runner.RunConfig{
			AdapterName: "func",
			Target:      target,
			Scenario:    sc,
			Seed:        seed,
		})
		if err != nil {
			return results, fmt.Errorf("seed %d: %w", seed, err)
		}
		results = append(results, Result{
			Seed:     seed,
			RunID:    res.RunID,
			Verdict:  string(res.Verdict),
			Reason:   res.Reason,
			Flags:    res.Flags,
			Duration: res.Duration,
		})
	}
	return results, nil
}

// AssertClean runs spec and fails the test for every seed whose verdict is not CLEAN.
// Each failure message names the seed so the run can be reproduced exactly.
func AssertClean(t testing.TB, spec Spec) {
	t.Helper()
	results, err := Run(context.Background(), spec)
	if err != nil {
		t.Fatalf("syncfault harness error: %v", err)
	}
	for _, r := range results {
		if !r.Clean() {
			t.Errorf("syncfault: %s seed=%d: %s (%s)", r.RunID, r.Seed, r.Verdict, r.Reason)
		}
	}
}

func buildScenario(spec Spec) (*scenario.Scenario, []uint64, error) {
	if spec.SyncFunc == nil {
		return nil, nil, fmt.Errorf("spec.SyncFunc is required")
	}
	if spec.Recipe == "" {
		return nil, nil, fmt.Errorf("spec.Recipe is required (one of: %s)", strings.Join(runner.Recipes(), ", "))
	}
	fault := spec.Fault
	if fault == "" {
		fault = "none"
	}
	name := spec.Name
	if name == "" {
		name = spec.Recipe + "_" + fault
	}
	seeds := spec.Seeds
	if len(seeds) == 0 {
		seeds = SeedRange(1, 5)
	}
	sides := spec.Sides
	if len(sides) == 0 {
		sides = []string{"A"}
	}
	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	policy := scenario.ConflictPolicy(spec.Policy)
	switch policy {
	case "":
		policy = scenario.ConflictPolicyDefault
	case scenario.ConflictPolicyDefault, scenario.ConflictPolicyLWW, scenario.ConflictPolicyConflictCopy:
	default:
		return nil, nil, fmt.Errorf("invalid spec.Policy %q (use default, lww or conflict_copy)", spec.Policy)
	}
	if spec.OpIndex < 0 {
		return nil, nil, fmt.Errorf("spec.OpIndex must not be negative")
	}

	sc := &scenario.Scenario{
		Name: name,
		Workload: scenario.WorkloadSpec{
			Recipe:   spec.Recipe,
			Depth:    orDefault(spec.Depth, 3),
			Files:    orDefault(spec.Files, 8),
			OpsCount: 20,
			FileSize: scenario.SizeRange{MinBytes: 512, MaxBytes: 4096},
			Sides:    sides,
			Params:   map[string]string{},
		},
		Fault: scenario.FaultSpec{
			Type:   fault,
			At:     scenario.FaultTrigger{OpIndex: spec.OpIndex},
			Params: copyParams(spec.FaultParams),
		},
		Expect: scenario.ExpectSpec{ConflictPolicy: policy},
		Runs: scenario.RunsSpec{
			Seeds:    seeds,
			TimeoutS: int(timeout / time.Second),
		},
	}

	// Fail early with a readable message instead of a mid-run error.
	if _, err := scenario.ExpandWorkload(sc.Workload, seeds[0]); err != nil {
		return nil, nil, err
	}
	return sc, seeds, nil
}

func orDefault(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

func copyParams(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// funcTarget adapts a SyncFunc to the internal adapter.Target interface.
type funcTarget struct {
	fn     SyncFunc
	policy scenario.ConflictPolicy
	sb     *sandbox.Sandbox
}

var _ adapter.Target = (*funcTarget)(nil)

func (f *funcTarget) Name() string { return "func" }

func (f *funcTarget) Version(context.Context) (string, error) { return "in-process-func", nil }

func (f *funcTarget) ConflictPolicy() scenario.ConflictPolicy {
	if f.policy == "" || f.policy == scenario.ConflictPolicyDefault {
		return scenario.ConflictPolicyConflictCopy
	}
	return f.policy
}

func (f *funcTarget) PID() int { return 0 }

func (f *funcTarget) Setup(_ context.Context, sb *sandbox.Sandbox) error {
	f.sb = sb
	return nil
}

func (f *funcTarget) SyncCycle(ctx context.Context) error {
	if f.sb == nil {
		return fmt.Errorf("funcTarget not set up")
	}
	return f.fn(ctx, Dirs{SideA: f.sb.SideA, SideB: f.sb.SideB, Remote: f.sb.Remote, Work: f.sb.Work})
}

func (f *funcTarget) CollectLogs(string) error { return nil }

func (f *funcTarget) Teardown(context.Context) error { return nil }
