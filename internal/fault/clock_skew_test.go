package fault

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Vishv07/syncfault/internal/sandbox"
)

func TestClockSkewInjectorShiftsAndTruncatesMtime(t *testing.T) {
	sb, err := sandbox.New("test-clock-skew", false)
	if err != nil {
		t.Fatalf("sandbox.New: %v", err)
	}
	defer sb.Teardown()

	fPath := filepath.Join(sb.SideB, "doc.txt")
	if err := os.WriteFile(fPath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	beforeInfo, err := os.Stat(fPath)
	if err != nil {
		t.Fatalf("stat before: %v", err)
	}

	inj, err := New("clock_skew", map[string]string{"side": "B", "skew_s": "-300", "granularity_s": "2"}, 11)
	if err != nil {
		t.Fatalf("fault.New(clock_skew): %v", err)
	}
	if err := inj.Arm(context.Background(), Trigger{OpIndex: 1}, sb); err != nil {
		t.Fatalf("Arm: %v", err)
	}
	if err := inj.Fire(); err != nil {
		t.Fatalf("Fire: %v", err)
	}

	afterInfo, err := os.Stat(fPath)
	if err != nil {
		t.Fatalf("stat after: %v", err)
	}
	if !afterInfo.ModTime().Before(beforeInfo.ModTime().Add(-200 * time.Second)) {
		t.Fatalf("expected mtime shifted back ~300s, got before=%v after=%v", beforeInfo.ModTime(), afterInfo.ModTime())
	}
	if afterInfo.ModTime().Second()%2 != 0 || afterInfo.ModTime().Nanosecond() != 0 {
		t.Fatalf("expected 2s granularity truncation, got %v", afterInfo.ModTime())
	}
	if err := inj.Heal(); err != nil {
		t.Fatalf("Heal: %v", err)
	}
}
