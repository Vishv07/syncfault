package verify

import (
	"testing"
	"testing/quick"

	"github.com/Vishv07/syncfault/internal/journal"
	"github.com/Vishv07/syncfault/internal/oracle"
	"github.com/Vishv07/syncfault/internal/scenario"
)

func TestEvaluateTaxonomyAndSeverityOrder(t *testing.T) {
	entries := []journal.Entry{
		{OpIndex: 1, Side: "A", Op: "create", Path: "thesis.docx", ContentHashAfter: "good_hash"},
	}
	model := oracle.Compute(entries, scenario.ConflictPolicyConflictCopy)

	// 1. CLEAN
	cleanSnap := StateSnapshot{
		SideA:  map[string]string{"thesis.docx": "good_hash"},
		SideB:  map[string]string{"thesis.docx": "good_hash"},
		Remote: map[string]string{"thesis.docx": "good_hash"},
	}
	if res := Evaluate(model, cleanSnap, true); res.Verdict != VerdictClean {
		t.Fatalf("expected CLEAN, got %s (%s)", res.Verdict, res.Reason)
	}

	// 2. DATA_LOSS
	lossSnap := StateSnapshot{
		SideA:  map[string]string{},
		SideB:  map[string]string{},
		Remote: map[string]string{},
	}
	if res := Evaluate(model, lossSnap, true); res.Verdict != VerdictDataLoss {
		t.Fatalf("expected DATA_LOSS, got %s", res.Verdict)
	}

	// 3. WRONG_BYTES
	corruptSnap := StateSnapshot{
		SideA:  map[string]string{"thesis.docx": "torn_corrupted_hash"},
		SideB:  map[string]string{"thesis.docx": "torn_corrupted_hash"},
		Remote: map[string]string{"thesis.docx": "torn_corrupted_hash"},
	}
	if res := Evaluate(model, corruptSnap, true); res.Verdict != VerdictWrongBytes {
		t.Fatalf("expected WRONG_BYTES, got %s", res.Verdict)
	}

	// 4. DUPLICATE
	dupSnap := StateSnapshot{
		SideA:  map[string]string{"thesis.docx": "good_hash", "thesis (1).docx": "good_hash"},
		SideB:  map[string]string{"thesis.docx": "good_hash", "thesis (1).docx": "good_hash"},
		Remote: map[string]string{"thesis.docx": "good_hash", "thesis (1).docx": "good_hash"},
	}
	if res := Evaluate(model, dupSnap, true); res.Verdict != VerdictDuplicate {
		t.Fatalf("expected DUPLICATE, got %s", res.Verdict)
	}

	// 5. STALL
	if res := Evaluate(model, cleanSnap, false); res.Verdict != VerdictStall {
		t.Fatalf("expected STALL when converged=false, got %s", res.Verdict)
	}
}

func TestConflictPolicyMonotonicityLWWMoreLenientThanConflictCopy(t *testing.T) {
	entries := []journal.Entry{
		{OpIndex: 1, Side: "A", Op: "create", Path: "notes.txt", ContentHashAfter: "vA"},
		{OpIndex: 2, Side: "B", Op: "create", Path: "notes.txt", ContentHashAfter: "vB"},
	}
	// Only vB survives (LWW discarded vA)
	snap := StateSnapshot{
		SideA:  map[string]string{"notes.txt": "vB"},
		SideB:  map[string]string{"notes.txt": "vB"},
		Remote: map[string]string{"notes.txt": "vB"},
	}

	mConflict := oracle.Compute(entries, scenario.ConflictPolicyConflictCopy)
	resConflict := Evaluate(mConflict, snap, true)
	if resConflict.Verdict != VerdictDataLoss {
		t.Fatalf("expected DATA_LOSS under conflict_copy when one version vanishes, got %s", resConflict.Verdict)
	}

	mLWW := oracle.Compute(entries, scenario.ConflictPolicyLWW)
	resLWW := Evaluate(mLWW, snap, true)
	if resLWW.Verdict != VerdictClean {
		t.Fatalf("expected CLEAN under lww policy, got %s (%s)", resLWW.Verdict, resLWW.Reason)
	}
}

func TestVerifierPurityProperty(t *testing.T) {
	fn := func(hashSuffix string, converged bool) bool {
		if hashSuffix == "" {
			hashSuffix = "default"
		}
		entries := []journal.Entry{
			{OpIndex: 1, Side: "A", Op: "create", Path: "f.txt", ContentHashAfter: "h_" + hashSuffix},
		}
		model := oracle.Compute(entries, scenario.ConflictPolicyConflictCopy)
		snap := StateSnapshot{
			SideA:  map[string]string{"f.txt": "h_" + hashSuffix},
			SideB:  map[string]string{"f.txt": "h_" + hashSuffix},
			Remote: map[string]string{"f.txt": "h_" + hashSuffix},
		}
		r1 := Evaluate(model, snap, converged)
		r2 := Evaluate(model, snap, converged)
		return r1.Verdict == r2.Verdict && r1.Reason == r2.Reason
	}
	if err := quick.Check(fn, nil); err != nil {
		t.Fatalf("purity property failed: %v", err)
	}
}
