package fault

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Vishv07/syncfault/internal/journal"
	"github.com/Vishv07/syncfault/internal/sandbox"
)

func TestBaseTapInjectorFiresAtAnchoredOpIndex(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sb, err := sandbox.New("fault-tap-test", false)
	if err != nil {
		t.Fatal(err)
	}
	defer sb.Teardown()

	j, err := journal.Open("fault-tap-test", filepath.Join(sb.Work, "journal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()

	firedCount := 0
	inj := &BaseTapInjector{
		FaultName: "custom",
		doFire: func() error {
			firedCount++
			return nil
		},
	}

	err = inj.Arm(context.Background(), Trigger{
		OpIndex:  2,
		JitterMS: 1,
		Journal:  j,
	}, sb)
	if err != nil {
		t.Fatal(err)
	}

	_ = j.Append(journal.Entry{OpIndex: 1, Op: "create"})
	if firedCount != 0 {
		t.Fatalf("expected not fired at op 1, got %d", firedCount)
	}
	_ = j.Append(journal.Entry{OpIndex: 2, Op: "edit"})
	if firedCount != 1 {
		t.Fatalf("expected fired once at op 2, got %d", firedCount)
	}
	// Idempotence check
	_ = inj.Fire()
	if firedCount != 1 {
		t.Fatalf("expected Fire() to be idempotent, got %d", firedCount)
	}
}
