package alto

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Vishv07/syncfault/internal/adapter"
	"github.com/Vishv07/syncfault/internal/sandbox"
)

func TestAltoAndUnisonAdaptersSyncInsideSandbox(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	for _, name := range []string{"alto", "unison"} {
		sb, err := sandbox.New("adapter-"+name, false)
		if err != nil {
			t.Fatal(err)
		}

		ad, err := adapter.New(name)
		if err != nil {
			t.Fatalf("adapter.New(%s) failed: %v", name, err)
		}

		ctx := context.Background()
		if err := ad.Setup(ctx, sb); err != nil {
			t.Fatalf("Setup(%s) failed: %v", name, err)
		}

		if err := os.WriteFile(filepath.Join(sb.SideA, "test.txt"), []byte("payload-"+name), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := ad.SyncCycle(ctx); err != nil {
			t.Fatalf("SyncCycle(%s) failed: %v", name, err)
		}

		got, err := os.ReadFile(filepath.Join(sb.SideB, "test.txt"))
		if err != nil || string(got) != "payload-"+name {
			t.Fatalf("expected SideB to have payload-%s, got %q (err=%v)", name, string(got), err)
		}
		_ = ad.Teardown(ctx)
		_ = sb.Teardown()
	}
}
