package fault

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func init() {
	Register("sigkill", func(_ map[string]string, _ uint64) Injector {
		return NewSigkillInjector()
	})
}

// SigkillInjector sends SIGKILL to the active target process group at the operation anchor.
type SigkillInjector struct {
	BaseTapInjector
}

// NewSigkillInjector returns an Injector that terminates the target process group via SIGKILL.
func NewSigkillInjector() *SigkillInjector {
	s := &SigkillInjector{}
	s.FaultName = "sigkill"
	s.doFire = func() error {
		if s.Sandbox != nil {
			marker := filepath.Join(s.Sandbox.Work, "fault-sigkill.marker")
			_ = os.WriteFile(marker, []byte(fmt.Sprintf("op_index=%d\n", s.Trigger.OpIndex)), 0o644)
		}
		if s.Trigger.Targetfn != nil {
			pid := s.Trigger.Targetfn()
			if pid > 0 {
				if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
					_ = syscall.Kill(pid, syscall.SIGKILL)
				}
			}
		}
		return nil
	}
	return s
}
