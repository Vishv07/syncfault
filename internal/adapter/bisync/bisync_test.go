package bisync

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Vishv07/syncfault/internal/sandbox"
)

func TestBisyncSetupConfinesRcloneConfigToSandbox(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sb, err := sandbox.New("bisync-unit", false)
	if err != nil {
		t.Fatal(err)
	}
	defer sb.Teardown()

	ad := &Adapter{}
	ctx := context.Background()
	if err := ad.Setup(ctx, sb); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(sb.Work, "rclone.conf"))
	if err != nil {
		t.Fatalf("expected sandbox rclone.conf to exist: %v", err)
	}
	if !strings.Contains(string(raw), "[sandboxremote]") {
		t.Fatalf("unexpected rclone.conf contents: %s", string(raw))
	}

	if err := os.WriteFile(filepath.Join(sb.SideA, "item.txt"), []byte("bisync-payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ad.SyncCycle(ctx); err != nil {
		t.Fatalf("SyncCycle failed: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(sb.SideB, "item.txt"))
	if err != nil || string(got) != "bisync-payload" {
		t.Fatalf("expected SideB to have bisync-payload, got %q (err=%v)", string(got), err)
	}
}
