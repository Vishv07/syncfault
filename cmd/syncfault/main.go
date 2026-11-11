package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/Vishv07/syncfault/internal/doctor"
	"github.com/Vishv07/syncfault/internal/runner"
	"github.com/Vishv07/syncfault/internal/scenario"
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
		if err := fs.Parse(args[1:]); err != nil {
			return ExitHarnessError
		}
		if *scenarioPath == "" {
			fmt.Fprintln(os.Stderr, "error: --scenario is required")
			return ExitHarnessError
		}
		sc, err := scenario.LoadFile(*scenarioPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading scenario: %v\n", err)
			return ExitHarnessError
		}
		seeds := sc.Runs.Seeds
		if *seedFlag != 0 {
			seeds = []uint64{*seedFlag}
		}
		worst := 0
		for _, seed := range seeds {
			res, err := runner.ExecuteSingle(context.Background(), runner.RunConfig{
				AdapterName: *adapterName,
				Scenario:    sc,
				Seed:        seed,
				KeepSandbox: *keep,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "harness error (seed %d): %v\n", seed, err)
				return ExitHarnessError
			}
			fmt.Printf("[%s] run=%s seed=%d fault=%s duration=%s (%s)\n",
				res.Verdict, res.RunID, res.Seed, res.FaultType, res.Duration.Round(1e6), res.Reason)
			if sev := res.Verdict.Severity(); sev > worst {
				worst = sev
			}
		}
		return worst

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

func printUsage() {
	fmt.Println(`Usage: syncfault <command> [flags]

Commands:
  doctor      Verify host environment and sweep orphaned sandboxes
  run         Execute a single scenario against a target adapter
  replay      Re-execute a recorded run deterministically from seed
  suite       Run a named scenario suite (smoke, nightly, full)
  report      Generate Markdown and HTML scorecard reports from runs.db
  ci          Run a suite and exit with the worst verdict severity code
  selfcheck   Validate the harness against refgood and refbad reference syncs`)
}
