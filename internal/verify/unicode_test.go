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
