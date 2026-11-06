package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Vishv07/syncfault/internal/adapter"
	_ "github.com/Vishv07/syncfault/internal/adapter/refgood"
	"github.com/Vishv07/syncfault/internal/journal"
	"github.com/Vishv07/syncfault/internal/sandbox"
	"github.com/Vishv07/syncfault/internal/scenario"
	"github.com/Vishv07/syncfault/internal/workload"
)

// Verdict represents one of the five severity-ordered outcome classifications (§7.6).
type Verdict string

const (
	VerdictClean      Verdict = "CLEAN"
	VerdictStall      Verdict = "STALL"
	VerdictDuplicate  Verdict = "DUPLICATE"
	VerdictWrongBytes Verdict = "WRONG_BYTES"
	VerdictDataLoss   Verdict = "DATA_LOSS"
)

// Severity returns the numeric severity rank (higher = worse).
func (v Verdict) Severity() int {
	switch v {
	case VerdictClean:
		return 0
	case VerdictStall:
		return 10
	case VerdictDuplicate:
		return 20
	case VerdictWrongBytes:
		return 30
	case VerdictDataLoss:
		return 40
	default:
		return 2
	}
}

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

// ExecuteSingle runs one (scenario, seed) against the specified adapter.
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
			// Best-effort sync during workload (fault may interrupt it)
			_ = target.SyncCycle(sctx)
			return nil
		},
	}

	if err := drv.Execute(runCtx, ops); err != nil {
		return nil, err
	}

	// Recovery sync cycle
	if err := target.SyncCycle(runCtx); err != nil {
		return &RunResult{
			RunID:         cfg.RunID,
			Adapter:       cfg.AdapterName,
			TargetVersion: ver,
			ScenarioName:  cfg.Scenario.Name,
			Recipe:        cfg.Scenario.Workload.Recipe,
			FaultType:     cfg.Scenario.Fault.Type,
			Seed:          cfg.Seed,
			Verdict:       VerdictStall,
			Reason:        fmt.Sprintf("recovery sync failed: %v", err),
			Duration:      time.Since(start),
			Entries:       j.Entries(),
			SandboxRoot:   sb.Root,
		}, nil
	}

	return &RunResult{
		RunID:         cfg.RunID,
		Adapter:       cfg.AdapterName,
		TargetVersion: ver,
		ScenarioName:  cfg.Scenario.Name,
		Recipe:        cfg.Scenario.Workload.Recipe,
		FaultType:     cfg.Scenario.Fault.Type,
		Seed:          cfg.Seed,
		Verdict:       VerdictClean,
		Reason:        "replicas converged cleanly",
		Duration:      time.Since(start),
		Entries:       j.Entries(),
		SandboxRoot:   sb.Root,
	}, nil
}
