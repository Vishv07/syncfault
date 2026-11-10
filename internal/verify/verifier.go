package verify

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Vishv07/syncfault/internal/oracle"
	"github.com/Vishv07/syncfault/internal/sandbox"
	"github.com/Vishv07/syncfault/internal/scenario"
)

// Verdict represents the normative 5-level severity taxonomy (§7.6).
type Verdict string

const (
	VerdictClean      Verdict = "CLEAN"
	VerdictStall      Verdict = "STALL"
	VerdictDuplicate  Verdict = "DUPLICATE"
	VerdictWrongBytes Verdict = "WRONG_BYTES"
	VerdictDataLoss   Verdict = "DATA_LOSS"
)

// Severity returns the exit-code rank of the verdict (40 > 30 > 20 > 10 > 0).
func (v Verdict) Severity() int {
	switch v {
	case VerdictDataLoss:
		return 40
	case VerdictWrongBytes:
		return 30
	case VerdictDuplicate:
		return 20
	case VerdictStall:
		return 10
	case VerdictClean:
		return 0
	default:
		return 2
	}
}

// StateSnapshot holds the observed path -> sha256 map for replicas A, B, and remote.
type StateSnapshot struct {
	SideA  map[string]string
	SideB  map[string]string
	Remote map[string]string
}

// Result contains the classified verdict, explanation, and optional flags.
type Result struct {
	Verdict Verdict
	Reason  string
	Flags   []string
}

// WalkSandbox walks SideA, SideB, and Remote inside sb and returns a StateSnapshot.
func WalkSandbox(sb *sandbox.Sandbox) (StateSnapshot, error) {
	a, err := walkDir(sb.SideA)
	if err != nil {
		return StateSnapshot{}, err
	}
	b, err := walkDir(sb.SideB)
	if err != nil {
		return StateSnapshot{}, err
	}
	r, err := walkDir(sb.Remote)
	if err != nil {
		return StateSnapshot{}, err
	}
	return StateSnapshot{SideA: a, SideB: b, Remote: r}, nil
}

// Evaluate is a pure function classifying (model, snapshot, converged) into a Result (§7.6).
// Severity ordering: DATA_LOSS > WRONG_BYTES > DUPLICATE > STALL > CLEAN.
func Evaluate(model oracle.Model, snap StateSnapshot, converged bool) Result {
	var flags []string
	flagSet := make(map[string]bool)
	addFlag := func(f string) {
		if !flagSet[f] {
			flagSet[f] = true
			flags = append(flags, f)
		}
	}

	// Build global inventory of surviving hashes across A, B, and remote
	survivingCanonicalHashes := make(map[string]bool)
	survivingTrashHashes := make(map[string]bool)

	replicas := []struct {
		name string
		tree map[string]string
	}{
		{"sideA", snap.SideA},
		{"sideB", snap.SideB},
		{"remote", snap.Remote},
	}

	// 1. Check WRONG_BYTES across all replicas
	for _, rep := range replicas {
		paths := sortedKeys(rep.tree)
		for _, p := range paths {
			h := rep.tree[p]
			if oracle.IsConflictOrTrashPath(p) {
				if !model.AllKnownHashes[h] {
					return Result{
						Verdict: VerdictWrongBytes,
						Reason:  fmt.Sprintf("%s/%s has corrupted hash %s matching no journal entry", rep.name, p, shortHash(h)),
						Flags:   flags,
					}
				}
				if oracle.IsTrashOnlyPath(p) {
					survivingTrashHashes[h] = true
				} else {
					survivingCanonicalHashes[h] = true
				}
				continue
			}

			survivingCanonicalHashes[h] = true
			legitForPath := model.LegitimateHashes[p]
			if (legitForPath == nil || !legitForPath[h]) && !model.AllKnownHashes[h] {
				return Result{
					Verdict: VerdictWrongBytes,
					Reason:  fmt.Sprintf("%s/%s has torn or unknown content hash %s", rep.name, p, shortHash(h)),
					Flags:   flags,
				}
			}
		}
	}

	// 2. Check DATA_LOSS across all expected files
	expKeys := make([]string, 0, len(model.Expectations))
	for k := range model.Expectations {
		expKeys = append(expKeys, k)
	}
	sort.Strings(expKeys)

	var duplicateReasons []string

	for _, canonPath := range expKeys {
		exp := model.Expectations[canonPath]
		if exp.Deleted {
			continue
		}

		// Verify the file exists at its canonical path or an acceptable alternate path
		foundCanonical := false
		var observedHash string
		for _, rep := range replicas {
			if h, ok := rep.tree[canonPath]; ok {
				foundCanonical = true
				observedHash = h
				break
			}
		}
		foundAlt := false
		for _, alt := range exp.AlternatePaths {
			for _, rep := range replicas {
				if h, ok := rep.tree[alt]; ok {
					foundAlt = true
					if observedHash == "" {
						observedHash = h
					}
					break
				}
			}
		}

		// Check if both old and new name exist simultaneously after a rename without one being conflict/trash
		if foundCanonical && foundAlt {
			duplicateReasons = append(duplicateReasons,
				fmt.Sprintf("both old path %v and new path %s exist simultaneously", exp.AlternatePaths, canonPath))
		}

		if !foundCanonical && !foundAlt {
			// Check if any acceptable hash survived in trash
			inTrash := false
			for h := range exp.AcceptableHash {
				if survivingTrashHashes[h] {
					inTrash = true
					break
				}
			}
			if inTrash {
				addFlag("recovered_via_trash")
			} else {
				return Result{
					Verdict: VerdictDataLoss,
					Reason:  fmt.Sprintf("required file %s is missing from all replicas and trash", canonPath),
					Flags:   flags,
				}
			}
		}

		// Verify concurrent edit survival policy
		if exp.ConcurrentEdits && len(exp.RequiredHashes) > 1 {
			missingVersions := 0
			for _, reqH := range exp.RequiredHashes {
				if !survivingCanonicalHashes[reqH] && !survivingTrashHashes[reqH] {
					missingVersions++
				}
			}
			if missingVersions > 0 {
				if model.Policy == scenario.ConflictPolicyLWW && missingVersions < len(exp.RequiredHashes) {
					addFlag("lww_discard")
				} else {
					return Result{
						Verdict: VerdictDataLoss,
						Reason:  fmt.Sprintf("concurrent edit version lost for %s under policy %s", canonPath, model.Policy),
						Flags:   flags,
					}
				}
			}
		}

		// Verify observed hash at canonical path belongs to acceptable set
		if foundCanonical && observedHash != "" && len(exp.AcceptableHash) > 0 && !exp.AcceptableHash[observedHash] {
			return Result{
				Verdict: VerdictWrongBytes,
				Reason:  fmt.Sprintf("file %s has unexpected content hash %s", canonPath, shortHash(observedHash)),
				Flags:   flags,
			}
		}
	}

	// 3. Check extra/unexpected duplicate artifacts
	for _, rep := range replicas {
		for _, p := range sortedKeys(rep.tree) {
			if isUnacceptableDuplicateName(p) {
				duplicateReasons = append(duplicateReasons, fmt.Sprintf("unexpected duplicate artifact %s/%s", rep.name, p))
			}
		}
	}

	if len(duplicateReasons) > 0 {
		return Result{
			Verdict: VerdictDuplicate,
			Reason:  duplicateReasons[0],
			Flags:   flags,
		}
	}

	// 4. Check STALL (convergence failure or replica disagreement after recovery)
	if !converged {
		return Result{
			Verdict: VerdictStall,
			Reason:  "target failed to converge within quiescence window",
			Flags:   flags,
		}
	}
	if !mapsEqual(snap.SideA, snap.SideB) {
		return Result{
			Verdict: VerdictStall,
			Reason:  "sideA and sideB did not converge to identical file trees",
			Flags:   flags,
		}
	}

	return Result{
		Verdict: VerdictClean,
		Reason:  "converged state satisfies oracle acceptable set",
		Flags:   flags,
	}
}

func walkDir(root string) (map[string]string, error) {
	out := make(map[string]string)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if strings.HasPrefix(base, ".tmp-syncfault-") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = scenario.HashBytes(data)
		return nil
	})
	return out, err
}

func isUnacceptableDuplicateName(p string) bool {
	base := filepath.Base(p)
	return strings.Contains(base, " (1)") || strings.Contains(base, ".dup")
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}
