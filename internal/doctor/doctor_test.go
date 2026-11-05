package doctor

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectProducesReport(t *testing.T) {
	rep := Inspect()
	if len(rep.Checks) == 0 {
		t.Fatal("expected at least one preflight check")
	}
	var buf bytes.Buffer
	rep.Format(&buf)
	out := buf.String()
	if !strings.Contains(out, "SyncFault Environment Preflight") {
		t.Fatalf("unexpected formatted output: %s", out)
	}
}

func TestSweepRemovesOrphanDirectories(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", tmp)

	orphanDir := filepath.Join(SandboxRoot(), "orphan-run-001")
	if err := os.MkdirAll(orphanDir, 0o755); err != nil {
		t.Fatalf("mkdir orphan: %v", err)
	}

	removed, err := Sweep()
	if err != nil {
		t.Fatalf("sweep failed: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected 1 removed orphan, got %d", removed)
	}
}
