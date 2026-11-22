package fault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	Register("state_corrupt", func(params map[string]string, seed uint64) Injector {
		return NewStateCorruptInjector(params, seed)
	})
}

// StateCorruptInjector corrupts or truncates the sync engine's local state-listing
// cache inside the sandbox work directory (such as bisync .lst files or baseline JSON)
// to test whether the engine safely recovers or propagates mass deletions.
type StateCorruptInjector struct {
	BaseTapInjector
	Params map[string]string
	Seed   uint64
}

// NewStateCorruptInjector constructs a state-cache corruption fault injector.
func NewStateCorruptInjector(params map[string]string, seed uint64) *StateCorruptInjector {
	s := &StateCorruptInjector{Params: params, Seed: seed}
	s.FaultName = "state_corrupt"
	s.doFire = func() error {
		if s.Sandbox == nil {
			return nil
		}
		mode := params["mode"]
		if mode == "" {
			mode = "truncate"
		}

		_ = filepath.Walk(s.Sandbox.Work, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			base := filepath.Base(path)
			// Never corrupt the harness run journal itself
			if base == "journal.jsonl" || strings.HasPrefix(base, "fault-") {
				return nil
			}
			if strings.HasSuffix(base, ".json") || strings.HasSuffix(base, ".lst") || strings.HasSuffix(base, ".db") {
				if mode == "garbage" {
					_ = os.WriteFile(path, []byte("{corrupted-state-listing\x00"), 0o644)
				} else {
					_ = os.Truncate(path, 0)
				}
			}
			return nil
		})

		marker := filepath.Join(s.Sandbox.Work, "fault-statecorrupt.active")
		return os.WriteFile(marker, []byte(fmt.Sprintf("mode=%s seed=%d\n", mode, seed)), 0o644)
	}
	s.doHeal = func() error {
		if s.Sandbox != nil {
			_ = os.Remove(filepath.Join(s.Sandbox.Work, "fault-statecorrupt.active"))
		}
		return nil
	}
	return s
}
