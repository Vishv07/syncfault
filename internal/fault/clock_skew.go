package fault

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func init() {
	Register("clock_skew", func(params map[string]string, seed uint64) Injector {
		return NewClockSkewInjector(params, seed)
	})
}

// ClockSkewInjector shifts or coarsens file modification timestamps (mtime) across
// a replica to simulate NTP clock drift or coarse filesystem mtime granularity
// (such as FAT32 2-second rounding or WebDAV 1-second server timestamps).
type ClockSkewInjector struct {
	BaseTapInjector
	Params map[string]string
	Seed   uint64
}

// NewClockSkewInjector constructs a clock-skew and mtime-granularity fault injector.
func NewClockSkewInjector(params map[string]string, seed uint64) *ClockSkewInjector {
	c := &ClockSkewInjector{Params: params, Seed: seed}
	c.FaultName = "clock_skew"
	c.doFire = func() error {
		if c.Sandbox == nil {
			return nil
		}
		skewSec := -120
		if s, ok := params["skew_s"]; ok {
			if parsed, err := strconv.Atoi(s); err == nil {
				skewSec = parsed
			}
		}
		granSec := 2
		if g, ok := params["granularity_s"]; ok {
			if parsed, err := strconv.Atoi(g); err == nil && parsed > 0 {
				granSec = parsed
			}
		}
		side := params["side"]
		if side == "" {
			side = "B"
		}
		root := c.Sandbox.SideB
		if side == "A" {
			root = c.Sandbox.SideA
		}

		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			shifted := info.ModTime().Add(time.Duration(skewSec) * time.Second)
			coarsened := shifted.Truncate(time.Duration(granSec) * time.Second)
			_ = os.Chtimes(path, coarsened, coarsened)
			return nil
		})

		marker := filepath.Join(c.Sandbox.Work, "fault-clockskew.active")
		return os.WriteFile(marker, []byte(fmt.Sprintf("side=%s skew_s=%d granularity_s=%d\n", side, skewSec, granSec)), 0o644)
	}
	c.doHeal = func() error {
		if c.Sandbox != nil {
			_ = os.Remove(filepath.Join(c.Sandbox.Work, "fault-clockskew.active"))
		}
		return nil
	}
	return c
}
