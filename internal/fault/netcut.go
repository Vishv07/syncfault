package fault

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func init() {
	Register("netcut", func(params map[string]string, _ uint64) Injector {
		return NewNetcutInjector(params)
	})
}

// NetcutInjector drops connectivity between the target replica and the remote endpoint,
// restoring the link when Heal() is called.
type NetcutInjector struct {
	BaseTapInjector
	Params map[string]string
}

// NewNetcutInjector creates a network partition fault injector.
func NewNetcutInjector(params map[string]string) *NetcutInjector {
	n := &NetcutInjector{Params: params}
	n.FaultName = "netcut"
	n.doFire = func() error {
		if n.Sandbox != nil {
			marker := filepath.Join(n.Sandbox.Work, "fault-netcut.active")
			if err := os.WriteFile(marker, []byte("cut\n"), 0o644); err != nil {
				return err
			}
		}
		if runtime.GOOS == "linux" {
			iface := params["iface"]
			if iface != "" {
				_ = exec.Command("ip", "link", "set", iface, "down").Run()
			}
		}
		return nil
	}
	n.doHeal = func() error {
		if n.Sandbox != nil {
			marker := filepath.Join(n.Sandbox.Work, "fault-netcut.active")
			_ = os.Remove(marker)
		}
		if runtime.GOOS == "linux" {
			iface := params["iface"]
			if iface != "" {
				_ = exec.Command("ip", "link", "set", iface, "up").Run()
			}
		}
		return nil
	}
	return n
}
