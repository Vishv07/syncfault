package runner

import (
	"context"
	"fmt"
	"io"

	_ "github.com/Vishv07/syncfault/internal/adapter/refbad"
	_ "github.com/Vishv07/syncfault/internal/adapter/refgood"
	"github.com/Vishv07/syncfault/internal/scenario"
)

// SelfCheckSummary holds the aggregated validation results of the refgood/refbad gate (§10.2).
type SelfCheckSummary struct {
	RefGoodRuns      int
	RefGoodFailures  int
	RefBadDataLoss   int
	RefBadWrongBytes int
	Passed           bool
}

// RunSelfCheck executes the self-validation gate against refgood and refbad,
// verifying that refgood passes CLEAN across all test scenarios and refbad triggers
// 10/10 reproducible DATA_LOSS and WRONG_BYTES verdicts.
func RunSelfCheck(ctx context.Context, out io.Writer) (*SelfCheckSummary, error) {
	scenarios := []*scenario.Scenario{
		{
			Name: "selfcheck_rename_sigkill",
			Workload: scenario.WorkloadSpec{
				Recipe:   "rename_storm",
				Depth:    3,
				Files:    8,
				FileSize: scenario.SizeRange{MinBytes: 256, MaxBytes: 1024},
				Sides:    []string{"A"},
			},
			Fault: scenario.FaultSpec{
				Type: "sigkill",
				At:   scenario.FaultTrigger{OpIndex: 10},
			},
			Runs: scenario.RunsSpec{Seeds: []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, TimeoutS: 30},
		},
		{
			Name: "selfcheck_crud_enospc",
			Workload: scenario.WorkloadSpec{
				Recipe:   "basic_crud",
				Depth:    2,
				Files:    6,
				FileSize: scenario.SizeRange{MinBytes: 512, MaxBytes: 2048},
				Sides:    []string{"A"},
			},
			Fault: scenario.FaultSpec{
				Type: "enospc",
				At:   scenario.FaultTrigger{OpIndex: 3},
			},
			Runs: scenario.RunsSpec{Seeds: []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, TimeoutS: 30},
		},
	}

	summary := &SelfCheckSummary{}

	// 1. Validate refgood across all scenarios & seeds
	for _, sc := range scenarios {
		for _, seed := range sc.Runs.Seeds {
			res, err := ExecuteSingle(ctx, RunConfig{
				AdapterName: "refgood",
				Scenario:    sc,
				Seed:        seed,
			})
			if err != nil {
				return nil, fmt.Errorf("refgood %s seed %d error: %w", sc.Name, seed, err)
			}
			summary.RefGoodRuns++
			if res.Verdict == VerdictDataLoss || res.Verdict == VerdictWrongBytes {
				summary.RefGoodFailures++
			}
		}
	}

	// 2. Validate refbad triggers 10/10 DATA_LOSS on rename_storm+sigkill and 10/10 WRONG_BYTES on crud+enospc
	for _, seed := range scenarios[0].Runs.Seeds {
		res, err := ExecuteSingle(ctx, RunConfig{
			AdapterName: "refbad",
			Scenario:    scenarios[0],
			Seed:        seed,
		})
		if err != nil {
			return nil, fmt.Errorf("refbad rename seed %d error: %w", seed, err)
		}
		if res.Verdict == VerdictDataLoss {
			summary.RefBadDataLoss++
		}
	}

	for _, seed := range scenarios[1].Runs.Seeds {
		res, err := ExecuteSingle(ctx, RunConfig{
			AdapterName: "refbad",
			Scenario:    scenarios[1],
			Seed:        seed,
		})
		if err != nil {
			return nil, fmt.Errorf("refbad enospc seed %d error: %w", seed, err)
		}
		if res.Verdict == VerdictWrongBytes {
			summary.RefBadWrongBytes++
		}
	}

	summary.Passed = summary.RefGoodFailures == 0 &&
		summary.RefBadDataLoss == 10 &&
		summary.RefBadWrongBytes == 10

	if out != nil {
		fmt.Fprintf(out, "SyncFault Self-Validation Gate\n")
		fmt.Fprintf(out, "==============================\n")
		fmt.Fprintf(out, "refgood runs:          %d (DATA_LOSS/WRONG_BYTES failures: %d)\n", summary.RefGoodRuns, summary.RefGoodFailures)
		fmt.Fprintf(out, "refbad DATA_LOSS:      %d/10 reproduced\n", summary.RefBadDataLoss)
		fmt.Fprintf(out, "refbad WRONG_BYTES:    %d/10 reproduced\n", summary.RefBadWrongBytes)
		if summary.Passed {
			fmt.Fprintf(out, "GATE RESULT:           PASS\n")
		} else {
			fmt.Fprintf(out, "GATE RESULT:           FAIL\n")
		}
	}

	return summary, nil
}
