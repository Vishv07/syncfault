package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Vishv07/syncfault/internal/journal"
	"github.com/Vishv07/syncfault/internal/scenario"
)

// ReplayResult summarizes a deterministic replay check across N executions.
type ReplayResult struct {
	RunID         string
	TotalReplays  int
	MatchedCount  int
	Original      Verdict
	Observed      map[Verdict]int
	Flaky         bool
	LastRunResult *RunResult
}

// ReplayRun loads a previously recorded run from Store, re-executes its exact scenario and seed
// `replays` times, and marks the run as FLAKY in the database if verdicts vary across replays (§8.1).
func ReplayRun(ctx context.Context, store *journal.Store, runID string, scenariosDir string, replays int) (*ReplayResult, error) {
	if replays <= 0 {
		replays = 1
	}
	stored, err := store.GetRun(runID)
	if err != nil {
		return nil, err
	}

	var sc *scenario.Scenario
	if stored.ScenarioYAML != "" {
		sc, err = scenario.Parse(stored.ScenarioYAML)
		if err != nil {
			return nil, fmt.Errorf("parse stored scenario YAML: %w", err)
		}
	} else {
		candidate := filepath.Join(scenariosDir, stored.ScenarioName+".yaml")
		if _, statErr := os.Stat(candidate); statErr == nil {
			sc, err = scenario.LoadFile(candidate)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("cannot locate scenario %q for replay", stored.ScenarioName)
		}
	}

	out := &ReplayResult{
		RunID:        runID,
		TotalReplays: replays,
		Original:     Verdict(stored.Verdict),
		Observed:     make(map[Verdict]int),
	}

	for i := 0; i < replays; i++ {
		res, err := ExecuteSingle(ctx, RunConfig{
			RunID:       fmt.Sprintf("%s-replay-%d", runID, i+1),
			AdapterName: stored.Adapter,
			Scenario:    sc,
			Seed:        stored.Seed,
		})
		if err != nil {
			return nil, err
		}
		out.LastRunResult = res
		out.Observed[res.Verdict]++
		if res.Verdict == out.Original {
			out.MatchedCount++
		}
	}

	if out.MatchedCount != replays || len(out.Observed) > 1 {
		out.Flaky = true
		stored.Flaky = true
		_ = store.SaveRun(*stored)
	}

	return out, nil
}

// PersistResult saves a RunResult and its scenario YAML into the Store.
func PersistResult(store *journal.Store, res *RunResult, scenarioYAML string) error {
	if store == nil || res == nil {
		return nil
	}
	var totalBytes int64
	for _, e := range res.Entries {
		totalBytes += e.Bytes
	}
	return store.SaveRun(journal.StoredRun{
		RunID:            res.RunID,
		Adapter:          res.Adapter,
		TargetVersion:    res.TargetVersion,
		ScenarioName:     res.ScenarioName,
		ScenarioYAML:     scenarioYAML,
		Recipe:           res.Recipe,
		FaultType:        res.FaultType,
		Seed:             res.Seed,
		Verdict:          string(res.Verdict),
		Reason:           res.Reason,
		Flags:            res.Flags,
		DurationMS:       res.Duration.Milliseconds(),
		BytesTransferred: totalBytes,
		Entries:          res.Entries,
	})
}
