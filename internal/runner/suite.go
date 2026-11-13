package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Vishv07/syncfault/internal/journal"
	"github.com/Vishv07/syncfault/internal/scenario"
)

// SuiteOptions configures a batch execution of a named scenario suite (smoke, nightly, full).
type SuiteOptions struct {
	AdapterName  string
	SuiteName    string
	ScenariosDir string
	Store        *journal.Store
	Jobs         int
	Out          io.Writer
}

// SuiteSummary aggregates verdicts across all runs in a suite.
type SuiteSummary struct {
	TotalRuns int
	Counts    map[Verdict]int
	Worst     Verdict
}

// RunSuite loads all scenarios from ScenariosDir, trims seeds per SuiteName (smoke, nightly, full),
// executes the matrix, persists each run into Store, and returns the SuiteSummary.
func RunSuite(ctx context.Context, opts SuiteOptions) (*SuiteSummary, error) {
	if opts.ScenariosDir == "" {
		opts.ScenariosDir = "scenarios"
	}
	entries, err := os.ReadDir(opts.ScenariosDir)
	if err != nil {
		return nil, fmt.Errorf("read scenarios dir %s: %w", opts.ScenariosDir, err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") && e.Name() != "suites.yaml" {
			files = append(files, filepath.Join(opts.ScenariosDir, e.Name()))
		}
	}
	sort.Strings(files)

	summary := &SuiteSummary{
		Counts: make(map[Verdict]int),
		Worst:  VerdictClean,
	}

	maxSeeds := seedCapForSuite(opts.SuiteName)

	for _, path := range files {
		rawBytes, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		sc, err := scenario.Parse(string(rawBytes))
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		seeds := sc.Runs.Seeds
		if maxSeeds > 0 && len(seeds) > maxSeeds {
			seeds = seeds[:maxSeeds]
		}

		for _, seed := range seeds {
			runID := fmt.Sprintf("%s-%s-%d", opts.AdapterName, sc.Name, seed)
			res, err := ExecuteSingle(ctx, RunConfig{
				RunID:       runID,
				AdapterName: opts.AdapterName,
				Scenario:    sc,
				Seed:        seed,
			})
			if err != nil {
				return nil, err
			}
			_ = PersistResult(opts.Store, res, string(rawBytes))

			summary.TotalRuns++
			summary.Counts[res.Verdict]++
			if res.Verdict.Severity() > summary.Worst.Severity() {
				summary.Worst = res.Verdict
			}
			if opts.Out != nil {
				fmt.Fprintf(opts.Out, "[%s] %s (seed=%d fault=%s duration=%s)\n",
					res.Verdict, res.RunID, res.Seed, res.FaultType, res.Duration.Round(1e6))
			}
		}
	}

	return summary, nil
}

// ShouldFailCI checks whether any observed verdict in SuiteSummary matches the comma-separated failOn list.
func (s *SuiteSummary) ShouldFailCI(failOn string) (bool, int) {
	if failOn == "" {
		failOn = "data_loss,wrong_bytes"
	}
	triggers := make(map[string]bool)
	for _, part := range strings.Split(strings.ToLower(failOn), ",") {
		triggers[strings.TrimSpace(part)] = true
	}

	worstTriggered := 0
	for v, count := range s.Counts {
		if count == 0 {
			continue
		}
		key := strings.ToLower(string(v))
		if triggers[key] && v.Severity() > worstTriggered {
			worstTriggered = v.Severity()
		}
	}
	return worstTriggered > 0, worstTriggered
}

func seedCapForSuite(suite string) int {
	switch strings.ToLower(suite) {
	case "smoke":
		return 3
	case "nightly":
		return 15
	case "full":
		return 0
	default:
		return 3
	}
}
