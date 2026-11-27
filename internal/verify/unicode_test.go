package verify

import (
	"testing"

	"github.com/Vishv07/syncfault/internal/journal"
	"github.com/Vishv07/syncfault/internal/oracle"
	"github.com/Vishv07/syncfault/internal/scenario"
)

func TestEvaluateUnicodeNFCAndNFDEquivalence(t *testing.T) {
	entries := []journal.Entry{
		{OpIndex: 1, Side: "A", Op: "create", Path: "intl/caf\u00e9_001.txt", ContentHashAfter: "h1"},
		{OpIndex: 2, Side: "A", Op: "edit", Path: "intl/cafe\u0301_001.txt", ContentHashBefore: "h1", ContentHashAfter: "h2"},
	}

	model := oracle.Compute(entries, scenario.ConflictPolicyConflictCopy)
	snap := StateSnapshot{
		SideA:  map[string]string{"intl/caf\u00e9_001.txt": "h2"},
		SideB:  map[string]string{"intl/caf\u00e9_001.txt": "h2"},
		Remote: map[string]string{"intl/caf\u00e9_001.txt": "h2"},
	}

	res := Evaluate(model, snap, true)
	if res.Verdict != VerdictClean {
		t.Fatalf("expected CLEAN verdict for normalized NFC/NFD edit, got %s (%s)", res.Verdict, res.Reason)
	}
}

func TestEvaluateFlagsResurrectedDeleteOnlyWithoutFault(t *testing.T) {
	base := []journal.Entry{
		{OpIndex: 1, Side: "A", Op: "create", Path: "keep.txt", ContentHashAfter: "k1"},
		{OpIndex: 2, Side: "A", Op: "create", Path: "gone.txt", ContentHashAfter: "g1"},
		{OpIndex: 3, Side: "A", Op: "delete", Path: "gone.txt", ContentHashBefore: "g1"},
	}
	tree := map[string]string{"keep.txt": "k1", "gone.txt": "g1"}
	snap := StateSnapshot{SideA: tree, SideB: tree, Remote: tree}

	res := Evaluate(oracle.Compute(base, scenario.ConflictPolicyConflictCopy), snap, true)
	if res.Verdict != VerdictDuplicate {
		t.Fatalf("expected DUPLICATE for a resurrected delete with no fault, got %s (%s)", res.Verdict, res.Reason)
	}

	withFault := append(append([]journal.Entry{}, base...),
		journal.Entry{OpIndex: 3, Op: "fault_fire", FaultFired: "sigkill"})
	res = Evaluate(oracle.Compute(withFault, scenario.ConflictPolicyConflictCopy), snap, true)
	if res.Verdict != VerdictClean {
		t.Fatalf("a fault may legitimately leave the file, expected CLEAN, got %s (%s)", res.Verdict, res.Reason)
	}

	clean := map[string]string{"keep.txt": "k1"}
	res = Evaluate(oracle.Compute(base, scenario.ConflictPolicyConflictCopy),
		StateSnapshot{SideA: clean, SideB: clean, Remote: clean}, true)
	if res.Verdict != VerdictClean {
		t.Fatalf("expected CLEAN when the delete propagated, got %s (%s)", res.Verdict, res.Reason)
	}
}
