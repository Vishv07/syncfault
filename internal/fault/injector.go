package fault

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Vishv07/syncfault/internal/journal"
	"github.com/Vishv07/syncfault/internal/sandbox"
)

// Trigger specifies the deterministic operation-count anchor and seeded jitter for a fault.
type Trigger struct {
	OpIndex  int
	JitterMS int
	Journal  *journal.Journal
	Targetfn func() int
	OnFault  func(faultName string, opIndex int)
}

// Injector is the normative fault-injector interface (§7.4).
type Injector interface {
	Name() string
	Arm(ctx context.Context, trigger Trigger, sb *sandbox.Sandbox) error
	Fire() error
	Heal() error
}

// NoneInjector is a no-op fault injector for baseline fault:none runs.
type NoneInjector struct{}

func (n *NoneInjector) Name() string { return "none" }
func (n *NoneInjector) Arm(_ context.Context, _ Trigger, _ *sandbox.Sandbox) error {
	return nil
}
func (n *NoneInjector) Fire() error { return nil }
func (n *NoneInjector) Heal() error { return nil }

// BaseTapInjector provides shared operation-index tap arming and idempotent Fire() handling.
type BaseTapInjector struct {
	FaultName string
	Trigger   Trigger
	Sandbox   *sandbox.Sandbox

	mu     sync.Mutex
	fired  bool
	healed bool
	doFire func() error
	doHeal func() error
}

func (b *BaseTapInjector) Name() string { return b.FaultName }

func (b *BaseTapInjector) Arm(_ context.Context, trigger Trigger, sb *sandbox.Sandbox) error {
	b.mu.Lock()
	b.Trigger = trigger
	b.Sandbox = sb
	b.mu.Unlock()

	if trigger.Journal != nil && trigger.OpIndex > 0 {
		trigger.Journal.RegisterTap(func(e journal.Entry) {
			if e.OpIndex == trigger.OpIndex && e.FaultFired == "" {
				if trigger.JitterMS > 0 {
					time.Sleep(time.Duration(trigger.JitterMS) * time.Millisecond)
				}
				_ = b.Fire()
			}
		})
	}
	return nil
}

func (b *BaseTapInjector) Fire() error {
	b.mu.Lock()
	if b.fired {
		b.mu.Unlock()
		return nil
	}
	b.fired = true
	fn := b.doFire
	trig := b.Trigger
	b.mu.Unlock()

	if fn != nil {
		if err := fn(); err != nil {
			return err
		}
	}
	if trig.OnFault != nil {
		trig.OnFault(b.FaultName, trig.OpIndex)
	}
	if trig.Journal != nil {
		_ = trig.Journal.Append(journal.Entry{
			OpIndex:         trig.OpIndex,
			Op:              "fault_fire",
			FaultFired:      b.FaultName,
			InFlightAtFault: true,
		})
	}
	return nil
}

func (b *BaseTapInjector) Heal() error {
	b.mu.Lock()
	if b.healed {
		b.mu.Unlock()
		return nil
	}
	b.healed = true
	fn := b.doHeal
	b.mu.Unlock()

	if fn != nil {
		return fn()
	}
	return nil
}

type Factory func(params map[string]string, seed uint64) Injector

var injectorRegistry = map[string]Factory{}

// Register registers a fault injector constructor by name.
func Register(name string, fn Factory) {
	injectorRegistry[name] = fn
}

// New constructs an Injector for the given fault type and params.
func New(faultType string, params map[string]string, seed uint64) (Injector, error) {
	if faultType == "" || faultType == "none" {
		return &NoneInjector{}, nil
	}
	fn, ok := injectorRegistry[faultType]
	if !ok {
		return nil, fmt.Errorf("unknown fault injector %q", faultType)
	}
	return fn(params, seed), nil
}
