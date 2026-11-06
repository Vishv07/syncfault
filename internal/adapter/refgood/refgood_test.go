package refgood

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Vishv07/syncfault/internal/sandbox"
)

func TestRefGoodSyncsReplicasAndHandlesConcurrentEdits(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sb, err := sandbox.New("refgood-unit", false)
	if err != nil {
		t.Fatal(err)
	}
	defer sb.Teardown()

	ad := &Adapter{}
	ctx := context.Background()
	if err := ad.Setup(ctx, sb); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Create on A and sync
	fileA := filepath.Join(sb.SideA, "doc.txt")
	if err := os.WriteFile(fileA, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ad.SyncCycle(ctx); err != nil {
		t.Fatalf("SyncCycle 1 failed: %v", err)
	}

	gotB, err := os.ReadFile(filepath.Join(sb.SideB, "doc.txt"))
	if err != nil || string(gotB) != "v1" {
		t.Fatalf("expected SideB to have v1, got %q (err=%v)", string(gotB), err)
	}

	// Concurrent edits on both A and B
	if err := os.WriteFile(fileA, []byte("v2-from-A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sb.SideB, "doc.txt"), []byte("v2-from-B"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ad.SyncCycle(ctx); err != nil {
		t.Fatalf("SyncCycle 2 failed: %v", err)
	}

	conflictB, err := os.ReadFile(filepath.Join(sb.SideA, "doc.sync-conflict-B.txt"))
	if err != nil || string(conflictB) != "v2-from-B" {
		t.Fatalf("expected conflict copy with v2-from-B, got %q (err=%v)", string(conflictB), err)
	}
}
