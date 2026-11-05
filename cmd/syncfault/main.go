package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Vishv07/syncfault/internal/doctor"
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
