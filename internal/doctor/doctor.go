package doctor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// CheckResult captures the status of a single environment preflight check.
type CheckResult struct {
	Name    string
	Passed  bool
	Warning bool
	Detail  string
}

// Report summarizes the host environment capabilities for SyncFault.
type Report struct {
	OS                string
	Arch              string
	GoVersion         string
	UserNamespacesOK  bool
	DegradedSigkill   bool
	FreeDiskBytes     uint64
	Checks            []CheckResult
	OrphansRemoved    int
}

// SandboxRoot returns the base directory under which all SyncFault runs are confined.
func SandboxRoot() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "syncfault")
	}
	return filepath.Join(os.TempDir(), "syncfault")
}

// Inspect evaluates the current system environment and returns a Report.
func Inspect() Report {
	rep := Report{
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		GoVersion: runtime.Version(),
	}

	// 1. Go version check
	rep.Checks = append(rep.Checks, CheckResult{
		Name:   "go_runtime",
		Passed: true,
		Detail: fmt.Sprintf("%s (%s/%s)", rep.GoVersion, rep.OS, rep.Arch),
	})

	// 2. OS & unprivileged user namespaces check
	if runtime.GOOS == "linux" {
		if _, err := exec.LookPath("unshare"); err == nil {
			cmd := exec.Command("unshare", "-rn", "true")
			if err := cmd.Run(); err == nil {
				rep.UserNamespacesOK = true
				rep.Checks = append(rep.Checks, CheckResult{
					Name:   "user_namespaces",
					Passed: true,
					Detail: "unprivileged user and network namespaces enabled",
				})
			} else {
				rep.DegradedSigkill = true
				rep.Checks = append(rep.Checks, CheckResult{
					Name:    "user_namespaces",
					Passed:  true,
					Warning: true,
					Detail:  "unshare -rn failed (sysctl kernel.unprivileged_userns_clone=1 may be needed); degrading to sigkill-only mode",
				})
			}
		} else {
			rep.DegradedSigkill = true
			rep.Checks = append(rep.Checks, CheckResult{
				Name:    "user_namespaces",
				Passed:  true,
				Warning: true,
				Detail:  "unshare binary not found; degrading to sigkill-only mode",
			})
		}
	} else {
		rep.DegradedSigkill = true
		rep.Checks = append(rep.Checks, CheckResult{
			Name:    "user_namespaces",
			Passed:  true,
			Warning: true,
			Detail:  fmt.Sprintf("host OS is %s (not linux); running in degraded sigkill-only mode", runtime.GOOS),
		})
	}

	// 3. iproute2 (tc / ip)
	if _, err := exec.LookPath("tc"); err == nil {
		rep.Checks = append(rep.Checks, CheckResult{
			Name:   "iproute2_tc",
			Passed: true,
			Detail: "tc found on PATH",
		})
	} else {
		rep.Checks = append(rep.Checks, CheckResult{
			Name:    "iproute2_tc",
			Passed:  true,
			Warning: true,
			Detail:  "tc not found on PATH (required only for netslow injector on Linux)",
		})
	}

	// 4. rclone availability
	if path, err := exec.LookPath("rclone"); err == nil {
		out, _ := exec.Command(path, "version", "--check=false").Output()
		firstLine := strings.Split(strings.TrimSpace(string(out)), "\n")[0]
		if firstLine == "" {
			firstLine = path
		}
		rep.Checks = append(rep.Checks, CheckResult{
			Name:   "rclone",
			Passed: true,
			Detail: firstLine,
		})
	} else {
		rep.Checks = append(rep.Checks, CheckResult{
			Name:    "rclone",
			Passed:  true,
			Warning: true,
			Detail:  "rclone not found on PATH (needed for bisync target adapter)",
		})
	}

	// 5. Free disk space check on sandbox root parent
	baseDir := os.TempDir()
	var stat syscall.Statfs_t
	if err := syscall.Statfs(baseDir, &stat); err == nil {
		freeBytes := stat.Bavail * uint64(stat.Bsize)
		rep.FreeDiskBytes = freeBytes
		freeGB := float64(freeBytes) / (1024 * 1024 * 1024)
		rep.Checks = append(rep.Checks, CheckResult{
			Name:    "disk_space",
			Passed:  freeBytes >= 512*1024*1024,
			Warning: freeBytes < 10*1024*1024*1024,
			Detail:  fmt.Sprintf("%.2f GiB available in %s", freeGB, baseDir),
		})
	}

	return rep
}

// Sweep removes orphaned sandbox directories under SandboxRoot().
func Sweep() (int, error) {
	root := SandboxRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		target := filepath.Join(root, entry.Name())
		if err := os.RemoveAll(target); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

// Format writes a human-readable summary of the doctor report.
func (r Report) Format(w io.Writer) {
	fmt.Fprintf(w, "SyncFault Environment Preflight\n")
	fmt.Fprintf(w, "===============================\n")
	if r.DegradedSigkill {
		fmt.Fprintf(w, "BANNER: User namespaces unavailable on %s/%s — running in degraded sigkill-only mode.\n\n", r.OS, r.Arch)
	}
	for _, c := range r.Checks {
		status := "OK"
		if !c.Passed {
			status = "FAIL"
		} else if c.Warning {
			status = "WARN"
		}
		fmt.Fprintf(w, "[%-4s] %-18s %s\n", status, c.Name, c.Detail)
	}
	if r.OrphansRemoved > 0 {
		fmt.Fprintf(w, "\nSwept %d orphaned sandbox directory(s) under %s\n", r.OrphansRemoved, SandboxRoot())
	}
}

// Healthy returns true if all required baseline checks passed.
func (r Report) Healthy() bool {
	for _, c := range r.Checks {
		if !c.Passed {
			return false
		}
	}
	return true
}
