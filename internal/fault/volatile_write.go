package fault

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Vishv07/syncfault/internal/journal"
	"github.com/Vishv07/syncfault/internal/scenario"
)

func init() {
	Register("volatile_write", func(params map[string]string, seed uint64) Injector {
		return NewVolatileWriteInjector(params, seed)
	})
}

// VolatileWriteInjector mutates an active file inside the sandbox while a sync pass
// is in flight, testing whether the sync engine stages transfers atomically or
// propagates a torn half-old/half-new buffer.
type VolatileWriteInjector struct {
	BaseTapInjector
	Params map[string]string
	Seed   uint64
}

// NewVolatileWriteInjector constructs an in-flight volatile file mutation injector.
func NewVolatileWriteInjector(params map[string]string, seed uint64) *VolatileWriteInjector {
	v := &VolatileWriteInjector{Params: params, Seed: seed}
	v.FaultName = "volatile_write"
	v.doFire = func() error {
		if v.Sandbox == nil {
			return nil
		}
		side := params["side"]
		if side == "" {
			side = "A"
		}
		root := v.Sandbox.SideA
		if side == "B" {
			root = v.Sandbox.SideB
		}

		files := listRegularFiles(root)
		if len(files) > 0 {
			stream := scenario.NewStream(seed ^ 0x5151)
			idx := stream.IntRange(0, len(files)-1)
			rel := files[idx]
			absPath := filepath.Join(root, filepath.FromSlash(rel))

			oldBytes, _ := os.ReadFile(absPath)
			beforeHash := ""
			if len(oldBytes) > 0 {
				beforeHash = scenario.HashBytes(oldBytes)
			}
			newSize := int64(len(oldBytes)) + 256
			if newSize < 512 {
				newSize = 512
			}
			newBytes := scenario.ContentBytes(seed^0x9999, v.Trigger.OpIndex+1000, newSize)
			afterHash := scenario.HashBytes(newBytes)

			if err := os.WriteFile(absPath, newBytes, 0o644); err != nil {
				return err
			}
			if v.Trigger.Journal != nil {
				_ = v.Trigger.Journal.Append(journal.Entry{
					OpIndex:           v.Trigger.OpIndex,
					Side:              side,
					Op:                string(scenario.OpEdit),
					Path:              rel,
					ContentHashBefore: beforeHash,
					ContentHashAfter:  afterHash,
					Bytes:             newSize,
					FaultFired:        "volatile_write",
					InFlightAtFault:   true,
				})
			}
		}

		marker := filepath.Join(v.Sandbox.Work, "fault-volatile.active")
		return os.WriteFile(marker, []byte(fmt.Sprintf("side=%s seed=%d\n", side, seed)), 0o644)
	}
	v.doHeal = func() error {
		if v.Sandbox != nil {
			_ = os.Remove(filepath.Join(v.Sandbox.Work, "fault-volatile.active"))
		}
		return nil
	}
	return v
}

func listRegularFiles(root string) []string {
	var out []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err == nil {
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(out)
	return out
}
