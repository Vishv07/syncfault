package journal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Entry represents a single append-only operation record in the run journal.
type Entry struct {
	RunID             string `json:"run_id"`
	OpIndex           int    `json:"op_index"`
	TsMonoNS          int64  `json:"ts_mono"`
	Side              string `json:"side,omitempty"`
	Op                string `json:"op"`
	Path              string `json:"path,omitempty"`
	TargetPath        string `json:"target_path,omitempty"`
	ContentHashBefore string `json:"content_hash_before,omitempty"`
	ContentHashAfter  string `json:"content_hash_after,omitempty"`
	Bytes             int64  `json:"bytes,omitempty"`
	FaultFired        string `json:"fault_fired,omitempty"`
	InFlightAtFault   bool   `json:"in_flight_at_fault,omitempty"`
}

// TapFunc is notified synchronously when an operation starts or completes.
type TapFunc func(entry Entry)

// Journal manages an append-only, fsynced JSONL run journal.
type Journal struct {
	runID   string
	path    string
	start   time.Time
	mu      sync.Mutex
	file    *os.File
	entries []Entry
	taps    []TapFunc
}

// Open creates or opens an append-only JSONL journal file at path.
func Open(runID, path string) (*Journal, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open journal %s: %w", path, err)
	}
	return &Journal{
		runID: runID,
		path:  path,
		start: time.Now(),
		file:  f,
	}, nil
}

// RegisterTap adds an observer callback invoked whenever an entry is appended.
func (j *Journal) RegisterTap(fn TapFunc) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.taps = append(j.taps, fn)
}

// Append writes an Entry to disk, flushes with fsync, and notifies registered taps.
func (j *Journal) Append(e Entry) error {
	j.mu.Lock()
	e.RunID = j.runID
	if e.TsMonoNS == 0 {
		e.TsMonoNS = time.Since(j.start).Nanoseconds()
	}
	raw, err := json.Marshal(e)
	if err != nil {
		j.mu.Unlock()
		return fmt.Errorf("marshal journal entry: %w", err)
	}
	raw = append(raw, '\n')
	if _, err := j.file.Write(raw); err != nil {
		j.mu.Unlock()
		return fmt.Errorf("write journal entry: %w", err)
	}
	if err := j.file.Sync(); err != nil {
		j.mu.Unlock()
		return fmt.Errorf("sync journal entry: %w", err)
	}
	j.entries = append(j.entries, e)
	taps := append([]TapFunc(nil), j.taps...)
	j.mu.Unlock()

	for _, tap := range taps {
		tap(e)
	}
	return nil
}

// Entries returns a copy of all recorded journal entries in order.
func (j *Journal) Entries() []Entry {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]Entry, len(j.entries))
	copy(out, j.entries)
	return out
}

// DeterministicJSONL returns the canonical JSONL representation with wall-clock TsMonoNS zeroed
// and RunID normalized so two runs of the same seed can be compared byte-for-byte.
func (j *Journal) DeterministicJSONL() ([]byte, error) {
	entries := j.Entries()
	var out []byte
	for _, e := range entries {
		e.RunID = "deterministic"
		e.TsMonoNS = 0
		line, err := json.Marshal(e)
		if err != nil {
			return nil, err
		}
		out = append(out, line...)
		out = append(out, '\n')
	}
	return out, nil
}

// Close closes the underlying journal file.
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.file == nil {
		return nil
	}
	err := j.file.Close()
	j.file = nil
	return err
}

// ReadFile parses all journal entries from an existing JSONL file on disk.
func ReadFile(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, sc.Err()
}
