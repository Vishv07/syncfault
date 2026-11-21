package fault

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Vishv07/syncfault/internal/journal"
	"github.com/Vishv07/syncfault/internal/sandbox"
)

func TestVolatileWriteInjectorMutatesAndLogs(t *testing.T) {
	sb, err := sandbox.New("test-volatile-write", false)
	if err != nil {
		t.Fatalf("sandbox.New: %v", err)
	}
	defer sb.Teardown()

	targetFile := filepath.Join(sb.SideA, "live.db")
	if err := os.WriteFile(targetFile, []byte("initial-content"), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}

	j, err := journal.Open("test-volatile-write", filepath.Join(sb.Work, "journal.jsonl"))
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	defer j.Close()

	inj, err := New("volatile_write", map[string]string{"side": "A"}, 42)
	if err != nil {
		t.Fatalf("fault.New(volatile_write): %v", err)
	}
	if err := inj.Arm(context.Background(), Trigger{OpIndex: 1, Journal: j}, sb); err != nil {
		t.Fatalf("Arm: %v", err)
	}
	if err := inj.Fire(); err != nil {
		t.Fatalf("Fire: %v", err)
	}

	updated, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("read mutated file: %v", err)
	}
	if string(updated) == "initial-content" {
		t.Fatal("expected volatile_write injector to mutate target file content")
	}
	if err := inj.Heal(); err != nil {
		t.Fatalf("Heal: %v", err)
	}
}
