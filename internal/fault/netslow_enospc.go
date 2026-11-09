package fault

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/Vishv07/syncfault/internal/scenario"
)

func init() {
	Register("netslow", func(params map[string]string, _ uint64) Injector {
		return NewNetslowInjector(params)
	})
	Register("enospc", func(params map[string]string, seed uint64) Injector {
		return NewEnospcInjector(params, seed)
	})
}

// NetslowInjector degrades network latency and packet loss on the sandbox link.
type NetslowInjector struct {
	BaseTapInjector
	Params map[string]string
}

// NewNetslowInjector creates a network degradation fault injector.
func NewNetslowInjector(params map[string]string) *NetslowInjector {
	n := &NetslowInjector{Params: params}
	n.FaultName = "netslow"
	n.doFire = func() error {
		delayMS := params["delay_ms"]
		if delayMS == "" {
			delayMS = "80"
		}
		lossPct := params["loss_pct"]
		if lossPct == "" {
			lossPct = "10"
		}
		if n.Sandbox != nil {
			marker := filepath.Join(n.Sandbox.Work, "fault-netslow.active")
			payload := fmt.Sprintf("delay_ms=%s loss_pct=%s\n", delayMS, lossPct)
			if err := os.WriteFile(marker, []byte(payload), 0o644); err != nil {
				return err
			}
		}
		if runtime.GOOS == "linux" {
			iface := params["iface"]
			if iface != "" {
				_ = exec.Command("tc", "qdisc", "add", "dev", iface, "root", "netem",
					"delay", delayMS+"ms", "loss", lossPct+"%").Run()
			}
		}
		return nil
	}
	n.doHeal = func() error {
		if n.Sandbox != nil {
			marker := filepath.Join(n.Sandbox.Work, "fault-netslow.active")
			_ = os.Remove(marker)
		}
		if runtime.GOOS == "linux" {
			iface := params["iface"]
			if iface != "" {
				_ = exec.Command("tc", "qdisc", "del", "dev", iface, "root").Run()
			}
		}
		return nil
	}
	return n
}

// EnospcInjector constrains available storage on the sandbox remote/replica at a seeded threshold,
// lifting the quota when Heal() is invoked before recovery convergence.
type EnospcInjector struct {
	BaseTapInjector
	Params map[string]string
	Seed   uint64
}

// NewEnospcInjector creates a disk-full fault injector.
func NewEnospcInjector(params map[string]string, seed uint64) *EnospcInjector {
	e := &EnospcInjector{Params: params, Seed: seed}
	e.FaultName = "enospc"
	e.doFire = func() error {
		if e.Sandbox == nil {
			return nil
		}
		used, _ := e.Sandbox.UsedBytes()
		stream := scenario.NewStream(seed)
		headroom := stream.Int64Range(512, 4096)
		quota := used + headroom
		if qStr, ok := params["quota_bytes"]; ok {
			if parsed, err := scenario.ParseByteSize(qStr); err == nil && parsed > 0 {
				quota = parsed
			}
		}
		e.Sandbox.QuotaBytes = quota
		marker := filepath.Join(e.Sandbox.Work, "fault-enospc.active")
		return os.WriteFile(marker, []byte(fmt.Sprintf("quota_bytes=%d\n", quota)), 0o644)
	}
	e.doHeal = func() error {
		if e.Sandbox != nil {
			e.Sandbox.QuotaBytes = 0
			marker := filepath.Join(e.Sandbox.Work, "fault-enospc.active")
			_ = os.Remove(marker)
		}
		return nil
	}
	return e
}
