package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Vishv07/syncfault/internal/adapter"
	_ "github.com/Vishv07/syncfault/internal/adapter/refgood"
	"github.com/Vishv07/syncfault/internal/fault"
	"github.com/Vishv07/syncfault/internal/journal"
	"github.com/Vishv07/syncfault/internal/oracle"
	"github.com/Vishv07/syncfault/internal/sandbox"
	"github.com/Vishv07/syncfault/internal/scenario"
	"github.com/Vishv07/syncfault/internal/verify"
	"github.com/Vishv07/syncfault/internal/workload"
)

// Verdict aliases verify.Verdict for callers of the runner package.
type Verdict = verify.Verdict

const (
	VerdictClean      = verify.VerdictClean
	VerdictStall      = verify.VerdictStall
	VerdictDuplicate  = verify.VerdictDuplicate
	VerdictWrongBytes = verify.VerdictWrongBytes
	VerdictDataLoss   = verify.VerdictDataLoss
)

// RunConfig holds parameters for executing a single (scenario, seed, adapter) run.
type RunConfig struct {
	RunID       string
	AdapterName string
	Scenario    *scenario.Scenario
	Seed        uint64
	KeepSandbox bool
}

// RunResult captures the outcome of a single run.
type RunResult struct {
	RunID         string
	Adapter       string
	TargetVersion string
	ScenarioName  string
	Recipe        string
	FaultType     string
	Seed          uint64
	Verdict       Verdict
	Reason        string
	Flags         []string
	Duration      time.Duration
	Entries       []journal.Entry
	SandboxRoot   string
}

// ExecuteSingle runs one (scenario, seed) against the specified adapter, injects the configured fault,
// heals and runs recovery sync cycles, and classifies the end state via the Oracle and Verifier.
func ExecuteSingle(ctx context.Context, cfg RunConfig) (*RunResult, error) {
	start := time.Now()
	if cfg.Scenario == nil {
		return nil, fmt.Errorf("nil scenario in RunConfig")
	}
	if cfg.RunID == "" {
		cfg.RunID = fmt.Sprintf("%s-%s-seed-%d", cfg.AdapterName, cfg.Scenario.Name, cfg.Seed)
	}

	timeout := time.Duration(cfg.Scenario.Runs.TimeoutS) * time.Second
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	sb, err := sandbox.New(cfg.RunID, cfg.KeepSandbox)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = sb.Teardown()
	}()

	target, err := adapter.New(cfg.AdapterName)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = target.Teardown(context.Background())
	}()

	ver, _ := target.Version(runCtx)
	if err := target.Setup(runCtx, sb); err != nil {
		return nil, fmt.Errorf("adapter setup: %w", err)
	}

	jPath := filepath.Join(sb.Work, "journal.jsonl")
	j, err := journal.Open(cfg.RunID, jPath)
	if err != nil {
		return nil, err
	}
	defer j.Close()

	// Arm fault injector with seeded jitter anchored on OpIndex
	inj, err := fault.New(cfg.Scenario.Fault.Type, cfg.Scenario.Fault.Params, cfg.Seed)
	if err != nil {
		return nil, err
	}
	jitterStream := scenario.NewStream(cfg.Seed ^ 0xabcdef)
	jitterMS := jitterStream.JitterMS(cfg.Scenario.Fault.At.JitterMS)
	if err := inj.Arm(runCtx, fault.Trigger{
		OpIndex:  cfg.Scenario.Fault.At.OpIndex,
		JitterMS: jitterMS,
		Journal:  j,
		Targetfn: target.PID,
	}, sb); err != nil {
		return nil, err
	}
	defer func() {
		_ = inj.Heal()
	}()

	// Initial baseline sync cycle
	if err := target.SyncCycle(runCtx); err != nil {
		return nil, fmt.Errorf("initial baseline sync: %w", err)
	}

	ops, err := scenario.ExpandWorkload(cfg.Scenario.Workload, cfg.Seed)
	if err != nil {
		return nil, err
	}

	drv := &workload.Driver{
		Seed:    cfg.Seed,
		Sandbox: sb,
		Journal: j,
		SyncHook: func(sctx context.Context, _ int) error {
			_ = target.SyncCycle(sctx)
			return nil
		},
	}

	if err := drv.Execute(runCtx, ops); err != nil {
		return nil, err
	}

	// Heal fault and run quiescence convergence loop (§7.8)
	if err := inj.Heal(); err != nil {
		return nil, fmt.Errorf("fault heal: %w", err)
	}

	snap, converged, err := WaitForQuiescence(runCtx, target, sb, 4)
	if err != nil {
		return nil, fmt.Errorf("verify walk: %w", err)
	}

	policy := cfg.Scenario.Expect.ConflictPolicy
	if policy == "" || policy == scenario.ConflictPolicyDefault {
		policy = target.ConflictPolicy()
	}

	entries := j.Entries()
	model := oracle.Compute(entries, policy)
	eval := verify.Evaluate(model, snap, converged)

	return &RunResult{
		RunID:         cfg.RunID,
		Adapter:       cfg.AdapterName,
		TargetVersion: ver,
		ScenarioName:  cfg.Scenario.Name,
		Recipe:        cfg.Scenario.Workload.Recipe,
		FaultType:     cfg.Scenario.Fault.Type,
		Seed:          cfg.Seed,
		Verdict:       eval.Verdict,
		Reason:        eval.Reason,
		Flags:         eval.Flags,
		Duration:      time.Since(start),
		Entries:       entries,
		SandboxRoot:   sb.Root,
	}, nil
}
