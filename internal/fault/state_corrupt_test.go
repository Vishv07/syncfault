package fault

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Vishv07/syncfault/internal/sandbox"
)

func TestStateCorruptInjectorCorruptsAdapterCacheWithoutTouchingJournal(t *testing.T) {
	sb, err := sandbox.New("test-state-corrupt", false)
	if err != nil {
		t.Fatalf("sandbox.New: %v", err)
	}
	defer sb.Teardown()

	journalPath := filepath.Join(sb.Work, "journal.jsonl")
	statePath := filepath.Join(sb.Work, "refgood_baseline.json")
	if err := os.WriteFile(journalPath, []byte(`{"op_index":1}`+"\n"), 0o644); err != nil {
		t.Fatalf("write journal: %v", err)
	}
	if err := os.WriteFile(statePath, []byte(`{"doc.txt":"h1"}`), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	inj, err := New("state_corrupt", map[string]string{"mode": "garbage"}, 21)
	if err != nil {
		t.Fatalf("fault.New(state_corrupt): %v", err)
	}
	if err := inj.Arm(context.Background(), Trigger{OpIndex: 1}, sb); err != nil {
		t.Fatalf("Arm: %v", err)
	}
	if err := inj.Fire(); err != nil {
		t.Fatalf("Fire: %v", err)
	}

	jBytes, _ := os.ReadFile(journalPath)
	if string(jBytes) != `{"op_index":1}`+"\n" {
		t.Fatalf("expected journal untouched, got %q", string(jBytes))
	}
	sBytes, _ := os.ReadFile(statePath)
	if string(sBytes) == `{"doc.txt":"h1"}` {
		t.Fatal("expected adapter state cache file to be corrupted")
	}
	if err := inj.Heal(); err != nil {
		t.Fatalf("Heal: %v", err)
	}
}
