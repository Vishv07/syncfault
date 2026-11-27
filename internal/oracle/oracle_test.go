package oracle

import (
	"testing"

	"github.com/Vishv07/syncfault/internal/journal"
	"github.com/Vishv07/syncfault/internal/scenario"
)

func TestOracleFaultNoneCollapsesToFinalTree(t *testing.T) {
	entries := []journal.Entry{
		{OpIndex: 1, Side: "A", Op: "create", Path: "a.txt", ContentHashAfter: "h1"},
		{OpIndex: 2, Side: "A", Op: "edit", Path: "a.txt", ContentHashBefore: "h1", ContentHashAfter: "h2"},
		{OpIndex: 3, Side: "A", Op: "rename", Path: "a.txt", TargetPath: "b.txt", ContentHashBefore: "h2", ContentHashAfter: "h2"},
	}

	m := Compute(entries, scenario.ConflictPolicyConflictCopy)
	if m.FaultFired {
		t.Fatal("expected FaultFired=false for fault:none journal")
	}
	if _, ok := m.Expectations["a.txt"]; ok {
		t.Fatal("expected old renamed path a.txt removed from canonical expectations")
	}
	exp, ok := m.Expectations["b.txt"]
	if !ok || exp.Deleted || !exp.AcceptableHash["h2"] {
		t.Fatalf("unexpected expectation for b.txt: %+v", exp)
	}
	if len(exp.AlternatePaths) != 0 {
		t.Fatalf("expected no AlternatePaths on fault:none rename, got %v", exp.AlternatePaths)
	}
}

func TestOracleIdempotenceUnderFaultFreeSyncCycles(t *testing.T) {
	base := []journal.Entry{
		{OpIndex: 1, Side: "A", Op: "create", Path: "doc.txt", ContentHashAfter: "hash_doc"},
		{OpIndex: 2, Side: "A", Op: "synccycle"},
	}
	withExtraCycles := append(append([]journal.Entry{}, base...),
		journal.Entry{OpIndex: 3, Side: "A", Op: "synccycle"},
		journal.Entry{OpIndex: 4, Side: "A", Op: "synccycle"},
	)

	m1 := Compute(base, scenario.ConflictPolicyConflictCopy)
	m2 := Compute(withExtraCycles, scenario.ConflictPolicyConflictCopy)

	if len(m1.Expectations) != len(m2.Expectations) {
		t.Fatalf("expected identical expectation count after extra sync cycles")
	}
	if !m2.Expectations["doc.txt"].AcceptableHash["hash_doc"] {
		t.Fatalf("expected hash_doc to remain acceptable after extra sync cycles")
	}
}

func TestOracleTracksCaseOnlyRenameAlternatePath(t *testing.T) {
	entries := []journal.Entry{
		{OpIndex: 1, Side: "A", Op: "create", Path: "CaseDir/Report_001.txt", ContentHashAfter: "h1"},
		{OpIndex: 2, Side: "A", Op: "rename", Path: "CaseDir/Report_001.txt", TargetPath: "CaseDir/report_001.txt", ContentHashBefore: "h1", ContentHashAfter: "h1"},
	}

	m := Compute(entries, scenario.ConflictPolicyConflictCopy)
	exp, ok := m.Expectations["CaseDir/report_001.txt"]
	if !ok {
		t.Fatal("expected canonical expectation for CaseDir/report_001.txt")
	}
	if len(exp.AlternatePaths) != 1 || exp.AlternatePaths[0] != "CaseDir/Report_001.txt" {
		t.Fatalf("expected CaseDir/Report_001.txt in AlternatePaths, got %v", exp.AlternatePaths)
	}
}

func TestOracleTracksRemovedPathsUntilRecreated(t *testing.T) {
	entries := []journal.Entry{
		{OpIndex: 1, Side: "A", Op: "create", Path: "a.txt", ContentHashAfter: "h1"},
		{OpIndex: 2, Side: "A", Op: "create", Path: "b.txt", ContentHashAfter: "h2"},
		{OpIndex: 3, Side: "A", Op: "delete", Path: "a.txt", ContentHashBefore: "h1"},
		{OpIndex: 4, Side: "A", Op: "rename", Path: "b.txt", TargetPath: "c.txt", ContentHashBefore: "h2", ContentHashAfter: "h2"},
		{OpIndex: 5, Side: "A", Op: "create", Path: "a.txt", ContentHashAfter: "h3"},
	}
	m := Compute(entries, scenario.ConflictPolicyConflictCopy)
	if m.RemovedPaths["a.txt"] {
		t.Fatal("a.txt was recreated, so it must not stay in RemovedPaths")
	}
	if !m.RemovedPaths["b.txt"] {
		t.Fatal("expected renamed-away b.txt in RemovedPaths")
	}
	if m.RemovedPaths["c.txt"] {
		t.Fatal("rename target c.txt must not be in RemovedPaths")
	}
}
