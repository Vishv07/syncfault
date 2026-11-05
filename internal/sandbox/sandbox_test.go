package sandbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSandboxLifecycleAndKeepFlag(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	sb, err := New("test-run-lifecycle", false)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	pathA, err := sb.ConfineSide("A", "notes/hello.txt")
	if err != nil {
		t.Fatalf("ConfineSide failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(pathA), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathA, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	used, err := sb.UsedBytes()
	if err != nil || used != 11 {
		t.Fatalf("expected 11 used bytes, got %d (err=%v)", used, err)
	}

	root := sb.Root
	if err := sb.Teardown(); err != nil {
		t.Fatalf("Teardown failed: %v", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("expected sandbox root %s to be removed after Teardown", root)
	}

	sbKeep, err := New("test-run-keep", true)
	if err != nil {
		t.Fatalf("New keep failed: %v", err)
	}
	if err := sbKeep.Teardown(); err != nil {
		t.Fatalf("Teardown keep failed: %v", err)
	}
	if _, err := os.Stat(sbKeep.Root); err != nil {
		t.Fatalf("expected kept sandbox %s to still exist: %v", sbKeep.Root, err)
	}
}
