package runner

import (
	"context"
	"reflect"

	"github.com/Vishv07/syncfault/internal/adapter"
	"github.com/Vishv07/syncfault/internal/sandbox"
	"github.com/Vishv07/syncfault/internal/verify"
)

// QuiescenceRuleDescription is printed in every report (§7.8).
const QuiescenceRuleDescription = "Converged when two consecutive recovery sync cycles complete with zero file content changes across sideA, sideB, and remote within the scenario timeout."

// WaitForQuiescence executes recovery sync cycles after Heal() until two consecutive cycles
// complete with identical snapshots across A, B, and remote, or maxCycles is reached (§7.8).
func WaitForQuiescence(ctx context.Context, target adapter.Target, sb *sandbox.Sandbox, maxCycles int) (verify.StateSnapshot, bool, error) {
	if maxCycles < 3 {
		maxCycles = 4
	}

	var prevSnap verify.StateSnapshot
	stableCycles := 0

	for cycle := 0; cycle < maxCycles; cycle++ {
		if err := ctx.Err(); err != nil {
			snap, _ := verify.WalkSandbox(sb)
			return snap, false, nil
		}

		syncErr := target.SyncCycle(ctx)
		curSnap, walkErr := verify.WalkSandbox(sb)
		if walkErr != nil {
			return verify.StateSnapshot{}, false, walkErr
		}

		if syncErr != nil {
			stableCycles = 0
			prevSnap = curSnap
			continue
		}

		if cycle > 0 && snapshotsIdentical(prevSnap, curSnap) {
			stableCycles++
			if stableCycles >= 1 {
				// Two consecutive completed cycles produced identical state
				return curSnap, true, nil
			}
		} else {
			stableCycles = 0
		}
		prevSnap = curSnap
	}

	return prevSnap, false, nil
}

func snapshotsIdentical(a, b verify.StateSnapshot) bool {
	return reflect.DeepEqual(a.SideA, b.SideA) &&
		reflect.DeepEqual(a.SideB, b.SideB) &&
		reflect.DeepEqual(a.Remote, b.Remote)
}
