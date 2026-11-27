package runner

// RunJSON is the stable machine-readable form of a single run result, used by `--json`
// output and the HTTP API.
type RunJSON struct {
	RunID         string   `json:"run_id"`
	Adapter       string   `json:"adapter"`
	TargetVersion string   `json:"target_version,omitempty"`
	Scenario      string   `json:"scenario"`
	Recipe        string   `json:"recipe"`
	Fault         string   `json:"fault"`
	Seed          uint64   `json:"seed"`
	Verdict       string   `json:"verdict"`
	ExitCode      int      `json:"exit_code"`
	Reason        string   `json:"reason"`
	Flags         []string `json:"flags"`
	DurationMS    int64    `json:"duration_ms"`
}

// JSON converts the result to its machine-readable form.
func (r *RunResult) JSON() RunJSON {
	flags := r.Flags
	if flags == nil {
		flags = []string{}
	}
	return RunJSON{
		RunID:         r.RunID,
		Adapter:       r.Adapter,
		TargetVersion: r.TargetVersion,
		Scenario:      r.ScenarioName,
		Recipe:        r.Recipe,
		Fault:         r.FaultType,
		Seed:          r.Seed,
		Verdict:       string(r.Verdict),
		ExitCode:      r.Verdict.Severity(),
		Reason:        r.Reason,
		Flags:         flags,
		DurationMS:    r.Duration.Milliseconds(),
	}
}

// SummaryJSON is the machine-readable form of a suite summary.
type SummaryJSON struct {
	TotalRuns int            `json:"total_runs"`
	Counts    map[string]int `json:"counts"`
	Worst     string         `json:"worst_verdict"`
	ExitCode  int            `json:"exit_code"`
}

// JSON converts the suite summary to its machine-readable form.
func (s *SuiteSummary) JSON() SummaryJSON {
	counts := make(map[string]int, len(s.Counts))
	for v, n := range s.Counts {
		counts[string(v)] = n
	}
	return SummaryJSON{
		TotalRuns: s.TotalRuns,
		Counts:    counts,
		Worst:     string(s.Worst),
		ExitCode:  s.Worst.Severity(),
	}
}
