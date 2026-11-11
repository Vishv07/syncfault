package runner

import (
	"context"
	"fmt"
	"testing"

	"github.com/Vishv07/syncfault/internal/sandbox"
	"github.com/Vishv07/syncfault/internal/scenario"
)

type stallingMockAdapter struct {
	calls int
}

func (s *stallingMockAdapter) Name() string                                    { return "stalling" }
func (s *stallingMockAdapter) Version(_ context.Context) (string, error)       { return "0.1", nil }
func (s *stallingMockAdapter) Setup(_ context.Context, _ *sandbox.Sandbox) error { return nil }
func (s *stallingMockAdapter) PID() int                                        { return 0 }
func (s *stallingMockAdapter) CollectLogs(_ string) error                      { return nil }
func (s *stallingMockAdapter) Teardown(_ context.Context) error                { return nil }
func (s *stallingMockAdapter) ConflictPolicy() scenario.ConflictPolicy {
	return scenario.ConflictPolicyDefault
}
func (s *stallingMockAdapter) SyncCycle(_ context.Context) error {
	s.calls++
	return fmt.Errorf("stale lock wedge")
}

func TestWaitForQuiescenceDetectsStallAfterMaxCycles(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	sb, err := sandbox.New("quiescence-stall", false)
	if err != nil {
		t.Fatal(err)
	}
	defer sb.Teardown()

	mock := &stallingMockAdapter{}
	_, converged, err := WaitForQuiescence(context.Background(), mock, sb, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if converged {
		t.Fatal("expected converged=false when SyncCycle repeatedly fails")
	}
	if mock.calls != 3 {
		t.Fatalf("expected 3 recovery cycles attempted, got %d", mock.calls)
	}
}
