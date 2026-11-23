package mutagen

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Vishv07/syncfault/internal/adapter"
	"github.com/Vishv07/syncfault/internal/sandbox"
)

func TestMutagenAndSyncthingAdaptersRegisterAndSync(t *testing.T) {
	for _, name := range []string{"mutagen", "syncthing"} {
		target, err := adapter.New(name)
		if err != nil {
			t.Fatalf("adapter.New(%s): %v", name, err)
		}

		sb, err := sandbox.New("test-"+name, false)
		if err != nil {
			t.Fatalf("sandbox.New: %v", err)
		}

		ctx := context.Background()
		if err := target.Setup(ctx, sb); err != nil {
			_ = sb.Teardown()
			t.Fatalf("Setup(%s): %v", name, err)
		}

		if err := os.WriteFile(filepath.Join(sb.SideA, "note.txt"), []byte("payload"), 0o644); err != nil {
			_ = sb.Teardown()
			t.Fatalf("write SideA: %v", err)
		}

		if err := target.SyncCycle(ctx); err != nil {
			_ = sb.Teardown()
			t.Fatalf("SyncCycle(%s): %v", name, err)
		}

		got, err := os.ReadFile(filepath.Join(sb.SideB, "note.txt"))
		if err != nil || string(got) != "payload" {
			_ = sb.Teardown()
			t.Fatalf("expected SideB/note.txt synced for %s, got %q (err=%v)", name, string(got), err)
		}

		_ = target.Teardown(ctx)
		_ = sb.Teardown()
	}
}
