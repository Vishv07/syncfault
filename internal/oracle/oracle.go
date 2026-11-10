package oracle

import (
	"path/filepath"
	"strings"

	"github.com/Vishv07/syncfault/internal/journal"
	"github.com/Vishv07/syncfault/internal/scenario"
)

// FileExpectation describes the acceptable end states for a logical file tracked by the journal.
type FileExpectation struct {
	CanonicalPath   string
	AlternatePaths  []string
	AcceptableHash  map[string]bool
	RequiredHashes  []string
	Deleted         bool
	RenamedAtFault  bool
	ConcurrentEdits bool
}

// Model is the pure output of the Oracle computed solely from the run journal and conflict policy (§7.7).
type Model struct {
	Policy           scenario.ConflictPolicy
	FaultFired       bool
	FaultOpIndex     int
	Expectations     map[string]*FileExpectation
	LegitimateHashes map[string]map[string]bool
	AllKnownHashes   map[string]bool
}

// Compute derives the acceptable end-state lattice purely from the journal and effective ConflictPolicy.
func Compute(entries []journal.Entry, policy scenario.ConflictPolicy) Model {
	if policy == "" || policy == scenario.ConflictPolicyDefault {
		policy = scenario.ConflictPolicyConflictCopy
	}

	m := Model{
		Policy:           policy,
		Expectations:     make(map[string]*FileExpectation),
		LegitimateHashes: make(map[string]map[string]bool),
		AllKnownHashes:   make(map[string]bool),
	}

	for _, e := range entries {
		if e.Op == "fault_fire" || e.FaultFired != "" {
			m.FaultFired = true
			m.FaultOpIndex = e.OpIndex
			break
		}
	}

	recordHash := func(p, h string) {
		if p == "" || h == "" {
			return
		}
		p = filepath.ToSlash(p)
		if m.LegitimateHashes[p] == nil {
			m.LegitimateHashes[p] = make(map[string]bool)
		}
		m.LegitimateHashes[p][h] = true
		m.AllKnownHashes[h] = true
	}

	// Track per-side latest hash on each path to detect concurrent edits
	sideLatest := map[string]map[string]string{
		"A": make(map[string]string),
		"B": make(map[string]string),
	}
	syncedHash := make(map[string]string)

	for _, e := range entries {
		side := e.Side
		if side == "" {
			side = "A"
		}
		p := filepath.ToSlash(e.Path)
		target := filepath.ToSlash(e.TargetPath)

		switch e.Op {
		case string(scenario.OpCreate), string(scenario.OpEdit):
			recordHash(p, e.ContentHashBefore)
			recordHash(p, e.ContentHashAfter)

			exp := getOrCreateExpectation(m.Expectations, p)
			exp.Deleted = false
			if e.ContentHashAfter != "" {
				exp.AcceptableHash[e.ContentHashAfter] = true
			}
			// If this op was at/around the fault anchor, pre-fault hash is also acceptable
			if m.FaultFired && absInt(e.OpIndex-m.FaultOpIndex) <= 1 && e.ContentHashBefore != "" {
				exp.AcceptableHash[e.ContentHashBefore] = true
			}

			if sideLatest[side] == nil {
				sideLatest[side] = make(map[string]string)
			}
			sideLatest[side][p] = e.ContentHashAfter

			otherSide := "B"
			if side == "B" {
				otherSide = "A"
			}
			otherH := sideLatest[otherSide][p]
			baseH := syncedHash[p]
			if otherH != "" && otherH != baseH && e.ContentHashAfter != otherH {
				exp.ConcurrentEdits = true
				exp.AcceptableHash[otherH] = true
				exp.AcceptableHash[e.ContentHashAfter] = true
				exp.RequiredHashes = uniqueStrings([]string{e.ContentHashAfter, otherH})
			} else if !exp.ConcurrentEdits {
				exp.RequiredHashes = []string{e.ContentHashAfter}
			}

		case string(scenario.OpRename):
			recordHash(p, e.ContentHashBefore)
			recordHash(target, e.ContentHashAfter)
			if e.ContentHashBefore != "" {
				recordHash(target, e.ContentHashBefore)
			}

			oldExp, hadOld := m.Expectations[p]
			delete(m.Expectations, p)

			newExp := getOrCreateExpectation(m.Expectations, target)
			newExp.Deleted = false
			if hadOld {
				for h := range oldExp.AcceptableHash {
					newExp.AcceptableHash[h] = true
					recordHash(target, h)
				}
				newExp.RequiredHashes = oldExp.RequiredHashes
			}
			if e.ContentHashAfter != "" {
				newExp.AcceptableHash[e.ContentHashAfter] = true
				if len(newExp.RequiredHashes) == 0 {
					newExp.RequiredHashes = []string{e.ContentHashAfter}
				}
			}
			if m.FaultFired {
				newExp.RenamedAtFault = true
				newExp.AlternatePaths = uniqueStrings(append(newExp.AlternatePaths, p))
			}
			delete(sideLatest[side], p)
			if e.ContentHashAfter != "" {
				sideLatest[side][target] = e.ContentHashAfter
			}

		case string(scenario.OpDelete):
			recordHash(p, e.ContentHashBefore)
			if exp, ok := m.Expectations[p]; ok {
				exp.Deleted = true
				if m.FaultFired && e.ContentHashBefore != "" {
					exp.AcceptableHash[e.ContentHashBefore] = true
				}
			}
			delete(sideLatest[side], p)

		case string(scenario.OpSyncCycle):
			// Mark single-side state as synced baseline when no concurrent divergence exists
			for pathKey, hA := range sideLatest["A"] {
				hB := sideLatest["B"][pathKey]
				if hB == "" || hB == hA {
					syncedHash[pathKey] = hA
					sideLatest["B"][pathKey] = hA
				}
			}
			for pathKey, hB := range sideLatest["B"] {
				hA := sideLatest["A"][pathKey]
				if hA == "" || hA == hB {
					syncedHash[pathKey] = hB
					sideLatest["A"][pathKey] = hB
				}
			}
		}
	}

	return m
}

// IsConflictOrTrashPath returns true if relPath matches a documented conflict copy or backup/trash path.
func IsConflictOrTrashPath(relPath string) bool {
	relPath = filepath.ToSlash(relPath)
	if strings.Contains(relPath, ".sync-conflict-") ||
		strings.Contains(relPath, ".conflict") ||
		strings.HasPrefix(relPath, ".trash/") ||
		strings.HasPrefix(relPath, ".syncfault-trash/") ||
		strings.HasPrefix(relPath, ".rclone-backup/") {
		return true
	}
	return false
}

// IsTrashOnlyPath returns true if relPath resides inside a backup/trash directory.
func IsTrashOnlyPath(relPath string) bool {
	relPath = filepath.ToSlash(relPath)
	return strings.HasPrefix(relPath, ".trash/") ||
		strings.HasPrefix(relPath, ".syncfault-trash/") ||
		strings.HasPrefix(relPath, ".rclone-backup/")
}

func getOrCreateExpectation(m map[string]*FileExpectation, path string) *FileExpectation {
	if exp, ok := m[path]; ok {
		return exp
	}
	exp := &FileExpectation{
		CanonicalPath:  path,
		AcceptableHash: make(map[string]bool),
	}
	m[path] = exp
	return exp
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
