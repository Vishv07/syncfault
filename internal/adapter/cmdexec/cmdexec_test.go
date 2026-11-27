package cmdexec

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Vishv07/syncfault/internal/sandbox"
)

func newSandbox(t *testing.T, name string) *sandbox.Sandbox {
	t.Helper()
	sb, err := sandbox.New(name, false)
	if err != nil {
		t.Fatalf("sandbox.New: %v", err)
	}
	t.Cleanup(func() { _ = sb.Teardown() })
	return sb
}

func TestSyncCycleSubstitutesPlaceholdersAndCopies(t *testing.T) {
	sb := newSandbox(t, "cmdexec-copy")
	a := New("cp -R {sideA}/. {sideB}/ && cp -R {sideB}/. {remote}/")
	ctx := context.Background()
	if err := a.Setup(ctx, sb); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sb.SideA, "note.txt"), []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.SyncCycle(ctx); err != nil {
		t.Fatalf("SyncCycle: %v", err)
	}
	for _, root := range []string{sb.SideB, sb.Remote} {
		got, err := os.ReadFile(filepath.Join(root, "note.txt"))
		if err != nil || string(got) != "payload" {
			t.Fatalf("expected note.txt in %s, got %q err=%v", root, got, err)
		}
	}
	if a.PID() != 0 {
		t.Fatalf("expected PID 0 after the command exits, got %d", a.PID())
	}
}

func TestSyncCycleExportsEnvironment(t *testing.T) {
	sb := newSandbox(t, "cmdexec-env")
	a := New(`printf '%s' "$SYNCFAULT_SIDE_B" > "$SYNCFAULT_WORK/seen.txt"`)
	ctx := context.Background()
	if err := a.Setup(ctx, sb); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := a.SyncCycle(ctx); err != nil {
		t.Fatalf("SyncCycle: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(sb.Work, "seen.txt"))
	if string(got) != sb.SideB {
		t.Fatalf("expected SYNCFAULT_SIDE_B=%s, got %q", sb.SideB, got)
	}
}

func TestSyncCycleReportsFailureAndKeepsLog(t *testing.T) {
	sb := newSandbox(t, "cmdexec-fail")
	a := New("echo boom >&2; exit 3")
	ctx := context.Background()
	if err := a.Setup(ctx, sb); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := a.SyncCycle(ctx); err == nil {
		t.Fatal("expected a non-zero exit to surface as an error")
	}
	dst := filepath.Join(t.TempDir(), "logs")
	if err := a.CollectLogs(dst); err != nil {
		t.Fatalf("CollectLogs: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dst, "cmd.log"))
	if !strings.Contains(string(data), "boom") {
		t.Fatalf("expected stderr in cmd.log, got %q", data)
	}
}

func TestTeardownKillsRunningProcessGroup(t *testing.T) {
	sb := newSandbox(t, "cmdexec-kill")
	a := New("sleep 30")
	ctx := context.Background()
	if err := a.Setup(ctx, sb); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- a.SyncCycle(ctx) }()

	deadline := time.Now().Add(5 * time.Second)
	for a.PID() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if a.PID() == 0 {
		t.Fatal("expected a running PID while the command executes")
	}
	_ = a.Teardown(ctx)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected the killed command to return an error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SyncCycle did not return after Teardown killed the process group")
	}
}

func TestPathsWithQuotesAreShellSafe(t *testing.T) {
	if got := shellQuote("a'b"); got != `'a'\''b'` {
		t.Fatalf("unexpected quoting: %s", got)
	}
}

func TestValidate(t *testing.T) {
	for _, ok := range []string{"rsync -a {sideA}/ {sideB}/", "echo ${HOME} {a,b}", "true"} {
		if err := Validate(ok); err != nil {
			t.Errorf("Validate(%q) unexpected error: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "   ", "cp {sideC}/ {sideB}/", "cp {src} {dst}"} {
		if err := Validate(bad); err == nil {
			t.Errorf("Validate(%q) expected an error", bad)
		}
	}
}
