package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

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

type suiteJob struct {
	runID   string
	sc      *scenario.Scenario
	rawYAML string
	seed    uint64
}

// DefaultJobs returns the default sandbox worker concurrency: min(4, max(1, NumCPU/2)) (§8.2).
func DefaultJobs() int {
	half := runtime.NumCPU() / 2
	if half < 1 {
		half = 1
	}
	if half > 4 {
		return 4
	}
	return half
}

// RunSuite loads all scenarios from ScenariosDir, trims seeds per SuiteName (smoke, nightly, full),
// executes the matrix across a parallel worker pool of independent sandboxes, persists each run into Store,
// and returns the SuiteSummary.
func RunSuite(ctx context.Context, opts SuiteOptions) (*SuiteSummary, error) {
	if opts.ScenariosDir == "" {
		opts.ScenariosDir = "scenarios"
	}
	if opts.Jobs <= 0 {
		opts.Jobs = DefaultJobs()
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

	maxSeeds := seedCapForSuite(opts.SuiteName)
	var jobs []suiteJob

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
			jobs = append(jobs, suiteJob{
				runID:   fmt.Sprintf("%s-%s-%d", opts.AdapterName, sc.Name, seed),
				sc:      sc,
				rawYAML: string(rawBytes),
				seed:    seed,
			})
		}
	}

	summary := &SuiteSummary{
		Counts: make(map[Verdict]int),
		Worst:  VerdictClean,
	}

	jobCh := make(chan suiteJob, len(jobs))
	for _, jb := range jobs {
		jobCh <- jb
	}
	close(jobCh)

	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup

	workerCount := opts.Jobs
	if workerCount > len(jobs) && len(jobs) > 0 {
		workerCount = len(jobs)
	}

	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for jb := range jobCh {
				res, err := ExecuteSingle(ctx, RunConfig{
					RunID:       jb.runID,
					AdapterName: opts.AdapterName,
					Scenario:    jb.sc,
					Seed:        jb.seed,
				})
				mu.Lock()
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					return
				}
				_ = PersistResult(opts.Store, res, jb.rawYAML)
				summary.TotalRuns++
				summary.Counts[res.Verdict]++
				if res.Verdict.Severity() > summary.Worst.Severity() {
					summary.Worst = res.Verdict
				}
				if opts.Out != nil {
					fmt.Fprintf(opts.Out, "[%s] %s (seed=%d fault=%s duration=%s)\n",
						res.Verdict, res.RunID, res.Seed, res.FaultType, res.Duration.Round(1e6))
				}
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
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
		return 2
	case "nightly":
		return 10
	case "full":
		return 0
	default:
		return 2
	}
}
