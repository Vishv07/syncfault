package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Vishv07/syncfault/internal/adapter"
	"github.com/Vishv07/syncfault/internal/adapter/cmdexec"
	"github.com/Vishv07/syncfault/internal/doctor"
	"github.com/Vishv07/syncfault/internal/journal"
	"github.com/Vishv07/syncfault/internal/report"
	"github.com/Vishv07/syncfault/internal/runner"
	"github.com/Vishv07/syncfault/internal/scenario"
	"github.com/Vishv07/syncfault/internal/server"
)

const (
	ExitClean        = 0
	ExitHarnessError = 2
	ExitStall        = 10
	ExitDuplicate    = 20
	ExitWrongBytes   = 30
	ExitDataLoss     = 40
)

func main() {
	os.Exit(runCLI(os.Args[1:]))
}

func runCLI(args []string) int {
	if len(args) == 0 {
		printUsage()
		return ExitHarnessError
	}

	switch args[0] {
	case "doctor":
		fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
		sweep := fs.Bool("sweep", false, "remove orphaned sandboxes")
		if err := fs.Parse(args[1:]); err != nil {
			return ExitHarnessError
		}
		rep := doctor.Inspect()
		if *sweep {
			removed, err := doctor.Sweep()
			if err != nil {
				fmt.Fprintf(os.Stderr, "doctor sweep error: %v\n", err)
				return ExitHarnessError
			}
			rep.OrphansRemoved = removed
		}
		rep.Format(os.Stdout)
		if !rep.Healthy() {
			return ExitHarnessError
		}
		return ExitClean

	case "run":
		fs := flag.NewFlagSet("run", flag.ContinueOnError)
		adapterName := fs.String("adapter", "refgood", "target adapter name")
		scenarioPath := fs.String("scenario", "", "path to scenario YAML file")
		seedFlag := fs.Uint64("seed", 0, "optional single seed override")
		keep := fs.Bool("keep", false, "keep sandbox directory after run")
		dbPath := fs.String("db", "runs.db", "path to SQLite runs database")
		syncCmd := fs.String("sync-cmd", "", "shell command run once per sync cycle (with --adapter cmd)")
		if err := fs.Parse(args[1:]); err != nil {
			return ExitHarnessError
		}
		factory, err := targetFactory(*adapterName, *syncCmd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return ExitHarnessError
		}
		if *scenarioPath == "" {
			fmt.Fprintln(os.Stderr, "error: --scenario is required")
			return ExitHarnessError
		}
		rawBytes, err := os.ReadFile(*scenarioPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading scenario: %v\n", err)
			return ExitHarnessError
		}
		sc, err := scenario.Parse(string(rawBytes))
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading scenario: %v\n", err)
			return ExitHarnessError
		}
		store, _ := journal.OpenStore(*dbPath)
		seeds := sc.Runs.Seeds
		if *seedFlag != 0 {
			seeds = []uint64{*seedFlag}
		}
		worst := 0
		for _, seed := range seeds {
			cfg := runner.RunConfig{
				AdapterName: *adapterName,
				Scenario:    sc,
				Seed:        seed,
				KeepSandbox: *keep,
			}
			if factory != nil {
				cfg.Target = factory()
			}
			res, err := runner.ExecuteSingle(context.Background(), cfg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "harness error (seed %d): %v\n", seed, err)
				return ExitHarnessError
			}
			_ = runner.PersistResult(store, res, string(rawBytes))
			fmt.Printf("[%s] run=%s seed=%d fault=%s duration=%s (%s)\n",
				res.Verdict, res.RunID, res.Seed, res.FaultType, res.Duration.Round(1e6), res.Reason)
			if sev := res.Verdict.Severity(); sev > worst {
				worst = sev
			}
		}
		return worst

	case "replay":
		fs := flag.NewFlagSet("replay", flag.ContinueOnError)
		runID := fs.String("run-id", "", "recorded run ID to replay")
		dbPath := fs.String("db", "runs.db", "path to runs database")
		replays := fs.Int("replays", 1, "number of replay executions")
		if err := fs.Parse(args[1:]); err != nil {
			return ExitHarnessError
		}
		if *runID == "" {
			fmt.Fprintln(os.Stderr, "error: --run-id is required")
			return ExitHarnessError
		}
		store, err := journal.OpenStore(*dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open store error: %v\n", err)
			return ExitHarnessError
		}
		rep, err := runner.ReplayRun(context.Background(), store, *runID, "scenarios", *replays)
		if err != nil {
			fmt.Fprintf(os.Stderr, "replay error: %v\n", err)
			return ExitHarnessError
		}
		fmt.Printf("[REPLAY] run=%s matched=%d/%d original=%s flaky=%v\n",
			rep.RunID, rep.MatchedCount, rep.TotalReplays, rep.Original, rep.Flaky)
		if rep.LastRunResult != nil {
			return rep.LastRunResult.Verdict.Severity()
		}
		return ExitClean

	case "suite":
		fs := flag.NewFlagSet("suite", flag.ContinueOnError)
		adapterName := fs.String("adapter", "refgood", "target adapter name")
		suiteName := fs.String("suite", "smoke", "suite size (smoke|nightly|full)")
		dbPath := fs.String("db", "runs.db", "path to runs database")
		jobs := fs.Int("jobs", 2, "worker concurrency")
		syncCmd := fs.String("sync-cmd", "", "shell command run once per sync cycle (with --adapter cmd)")
		if err := fs.Parse(args[1:]); err != nil {
			return ExitHarnessError
		}
		factory, ferr := targetFactory(*adapterName, *syncCmd)
		if ferr != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", ferr)
			return ExitHarnessError
		}
		store, err := journal.OpenStore(*dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open store error: %v\n", err)
			return ExitHarnessError
		}
		sum, err := runner.RunSuite(context.Background(), runner.SuiteOptions{
			AdapterName: *adapterName,
			SuiteName:   *suiteName,
			Store:       store,
			Jobs:        *jobs,
			Out:         os.Stdout,

			TargetFactory: factory,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "suite error: %v\n", err)
			return ExitHarnessError
		}
		fmt.Printf("\nSuite %q complete: %d runs, worst verdict=%s\n", *suiteName, sum.TotalRuns, sum.Worst)
		return sum.Worst.Severity()

	case "ci":
		fs := flag.NewFlagSet("ci", flag.ContinueOnError)
		adapterName := fs.String("adapter", "refgood", "target adapter name")
		suiteName := fs.String("suite", "smoke", "suite size (smoke|nightly|full)")
		failOn := fs.String("fail-on", "data_loss,wrong_bytes", "comma-separated verdicts that fail CI")
		dbPath := fs.String("db", "runs.db", "path to runs database")
		syncCmd := fs.String("sync-cmd", "", "shell command run once per sync cycle (with --adapter cmd)")
		if err := fs.Parse(args[1:]); err != nil {
			return ExitHarnessError
		}
		factory, ferr := targetFactory(*adapterName, *syncCmd)
		if ferr != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", ferr)
			return ExitHarnessError
		}
		store, err := journal.OpenStore(*dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open store error: %v\n", err)
			return ExitHarnessError
		}
		sum, err := runner.RunSuite(context.Background(), runner.SuiteOptions{
			AdapterName: *adapterName,
			SuiteName:   *suiteName,
			Store:       store,
			Out:         os.Stdout,

			TargetFactory: factory,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "ci suite error: %v\n", err)
			return ExitHarnessError
		}
		if failed, code := sum.ShouldFailCI(*failOn); failed {
			return code
		}
		return ExitClean

	case "report":
		fs := flag.NewFlagSet("report", flag.ContinueOnError)
		dbPath := fs.String("db", "runs.db", "path to runs database")
		outDir := fs.String("out", "report", "directory for scorecard.md and index.html")
		if err := fs.Parse(args[1:]); err != nil {
			return ExitHarnessError
		}
		store, err := journal.OpenStore(*dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open store error: %v\n", err)
			return ExitHarnessError
		}
		runs, err := store.ListRuns()
		if err != nil {
			fmt.Fprintf(os.Stderr, "list runs error: %v\n", err)
			return ExitHarnessError
		}
		summary := report.Summarize(runs)
		if err := report.GenerateAll(*outDir, summary); err != nil {
			fmt.Fprintf(os.Stderr, "generate report error: %v\n", err)
			return ExitHarnessError
		}
		fmt.Printf("Generated scorecard (%d runs) in %s/scorecard.md and %s/index.html\n",
			summary.TotalRuns, *outDir, *outDir)
		return ExitClean

	case "serve":
		fs := flag.NewFlagSet("serve", flag.ContinueOnError)
		addr := fs.String("addr", "127.0.0.1:8475", "listen address (loopback only)")
		scenariosDir := fs.String("scenarios", "scenarios", "directory used to resolve scenario_name")
		allowCmd := fs.Bool("allow-cmd", false, "let API requests run a shell command through the cmd adapter")
		maxConcurrent := fs.Int("max-concurrent", 2, "maximum simultaneous runs")
		if err := fs.Parse(args[1:]); err != nil {
			return ExitHarnessError
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := server.Serve(ctx, *addr, server.Options{
			ScenariosDir:  *scenariosDir,
			AllowCmd:      *allowCmd,
			MaxConcurrent: *maxConcurrent,
		}, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "serve error: %v\n", err)
			return ExitHarnessError
		}
		return ExitClean

	case "selfcheck":
		sum, err := runner.RunSelfCheck(context.Background(), os.Stdout)
		if err != nil || !sum.Passed {
			if err != nil {
				fmt.Fprintf(os.Stderr, "selfcheck error: %v\n", err)
			}
			return ExitHarnessError
		}
		return ExitClean

	case "help", "-h", "--help":
		printUsage()
		return ExitClean

	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n", args[0])
		printUsage()
		return ExitHarnessError
	}
}

// targetFactory returns a per-run adapter builder for --adapter cmd, or nil for registered adapters.
func targetFactory(adapterName, syncCmd string) (func() adapter.Target, error) {
	if adapterName != "cmd" {
		if syncCmd != "" {
			return nil, fmt.Errorf("--sync-cmd requires --adapter cmd")
		}
		return nil, nil
	}
	if err := cmdexec.Validate(syncCmd); err != nil {
		return nil, fmt.Errorf("--adapter cmd needs a valid --sync-cmd: %w", err)
	}
	return func() adapter.Target { return cmdexec.New(syncCmd) }, nil
}

func printUsage() {
	fmt.Println(`Usage: syncfault <command> [flags]

Commands:
  doctor      Verify host environment and sweep orphaned sandboxes
  run         Execute a single scenario against a target adapter
  replay      Re-execute a recorded run deterministically from seed
  suite       Run a named scenario suite (smoke, nightly, full)
  report      Generate Markdown and HTML scorecard reports from runs.db
  ci          Run a suite and exit with the worst verdict severity code
  serve       Start the local JSON HTTP API (loopback only)
  selfcheck   Validate the harness against refgood and refbad reference syncs`)
}
