package scenario

import "testing"

const normativeScenarioYAML = `
# scenarios/rename_storm_sigkill.yaml
name: rename_storm_sigkill
description: Deep directory renamed while files sync; process killed mid-cycle.
workload:
  recipe: rename_storm        # registered recipe name
  params:
    depth: 4                  # recipe-specific knobs
    files: 24
    file_size: { min: 1KiB, max: 4MiB }
  sides: [A]                  # which replicas the driver mutates; [A,B] = concurrent
fault:
  type: sigkill               # sigkill | netcut | netslow | enospc | none
  at:
    op_index: 12              # fire when journal records op #12 as started…
    jitter_ms: { min: 0, max: 40, seeded: true }   # …plus seeded jitter
  params: {}                  # injector-specific (e.g. netslow: delay_ms, loss_pct)
expect:
  conflict_policy: default
runs:
  seeds: [1..50]              # range expands to 50 independent runs
  timeout_s: 180              # hard wall per run (STALL if exceeded)
`

func TestParseNormativeScenario(t *testing.T) {
	sc, err := Parse(normativeScenarioYAML)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if sc.Name != "rename_storm_sigkill" {
		t.Fatalf("expected name rename_storm_sigkill, got %q", sc.Name)
	}
	if sc.Workload.Recipe != "rename_storm" || sc.Workload.Depth != 4 || sc.Workload.Files != 24 {
		t.Fatalf("unexpected workload spec: %+v", sc.Workload)
	}
	if sc.Workload.FileSize.MinBytes != 1024 || sc.Workload.FileSize.MaxBytes != 4*1024*1024 {
		t.Fatalf("unexpected file_size range: %+v", sc.Workload.FileSize)
	}
	if sc.Fault.Type != "sigkill" || sc.Fault.At.OpIndex != 12 || !sc.Fault.At.JitterMS.Seeded || sc.Fault.At.JitterMS.MaxMS != 40 {
		t.Fatalf("unexpected fault spec: %+v", sc.Fault)
	}
	if len(sc.Runs.Seeds) != 50 || sc.Runs.Seeds[0] != 1 || sc.Runs.Seeds[49] != 50 {
		t.Fatalf("expected 50 expanded seeds [1..50], got len=%d", len(sc.Runs.Seeds))
	}
}
