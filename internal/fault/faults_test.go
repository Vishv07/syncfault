package fault

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Vishv07/syncfault/internal/sandbox"
)

func TestNetcutNetslowAndEnospcFireAndHeal(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sb, err := sandbox.New("faults-unit", false)
	if err != nil {
		t.Fatal(err)
	}
	defer sb.Teardown()

	for _, kind := range []string{"netcut", "netslow", "enospc"} {
		inj, err := New(kind, map[string]string{"delay_ms": "50"}, 42)
		if err != nil {
			t.Fatalf("New(%s) failed: %v", kind, err)
		}
		if err := inj.Arm(context.Background(), Trigger{OpIndex: 1}, sb); err != nil {
			t.Fatalf("Arm(%s) failed: %v", kind, err)
		}
		if err := inj.Fire(); err != nil {
			t.Fatalf("Fire(%s) failed: %v", kind, err)
		}
		marker := filepath.Join(sb.Work, "fault-"+kind+".active")
		if _, err := os.Stat(marker); err != nil {
			t.Fatalf("expected active marker for %s: %v", kind, err)
		}
		if kind == "enospc" && sb.QuotaBytes <= 0 {
			t.Fatalf("expected positive QuotaBytes when enospc fired, got %d", sb.QuotaBytes)
		}
		if err := inj.Heal(); err != nil {
			t.Fatalf("Heal(%s) failed: %v", kind, err)
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("expected marker removed after Heal(%s)", kind)
		}
	}
}
