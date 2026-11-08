package fault

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Vishv07/syncfault/internal/journal"
	"github.com/Vishv07/syncfault/internal/sandbox"
)

func TestSigkillInjectorKillsSubprocessAtAnchor(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sb, err := sandbox.New("sigkill-unit", false)
	if err != nil {
		t.Fatal(err)
	}
	defer sb.Teardown()

	j, err := journal.Open("sigkill-unit", filepath.Join(sb.Work, "journal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start sleep process: %v", err)
	}
	pid := cmd.Process.Pid

	inj, err := New("sigkill", nil, 1)
	if err != nil {
		t.Fatal(err)
	}

	err = inj.Arm(context.Background(), Trigger{
		OpIndex:  1,
		Journal:  j,
		Targetfn: func() int { return pid },
	}, sb)
	if err != nil {
		t.Fatal(err)
	}

	_ = j.Append(journal.Entry{OpIndex: 1, Op: "create"})
	waitErr := cmd.Wait()
	if waitErr == nil {
		t.Fatal("expected sleep process to be killed by SIGKILL")
	}
	if _, err := os.Stat(filepath.Join(sb.Work, "fault-sigkill.marker")); err != nil {
		t.Fatalf("expected fault-sigkill.marker to exist: %v", err)
	}
}
