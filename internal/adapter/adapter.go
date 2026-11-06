package adapter

import (
	"context"
	"fmt"

	"github.com/Vishv07/syncfault/internal/sandbox"
	"github.com/Vishv07/syncfault/internal/scenario"
)

// Target is the normative per-tool adapter interface (§7.5).
type Target interface {
	Name() string
	Version(ctx context.Context) (string, error)
	// Setup creates whatever the tool needs inside the sandbox to sync sideA <-> sideB
	// (possibly via a remote). It must not touch anything outside sb paths.
	Setup(ctx context.Context, sb *sandbox.Sandbox) error
	// SyncCycle triggers one reconciliation and returns when the tool claims completion.
	SyncCycle(ctx context.Context) error
	// PID returns the process (group) to kill, if currently running; 0 if none.
	PID() int
	// CollectLogs copies tool logs into the run directory for reports (verifier never reads them).
	CollectLogs(dst string) error
	// Teardown stops processes; must be kill-safe (called after SIGKILL runs too).
	Teardown(ctx context.Context) error
	// ConflictPolicy declares how the tool documents concurrent-edit handling.
	ConflictPolicy() scenario.ConflictPolicy
}

// Factory constructs a fresh Target instance for a run.
type Factory func() Target

var registry = map[string]Factory{}

// Register registers an adapter factory by name.
func Register(name string, fn Factory) {
	registry[name] = fn
}

// New instantiates a registered Target adapter by name.
func New(name string) (Target, error) {
	fn, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown target adapter %q", name)
	}
	return fn(), nil
}
