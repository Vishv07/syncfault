package journal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// StoredRun represents a completed run persisted in runs.db.
type StoredRun struct {
	RunID            string   `json:"run_id"`
	Adapter          string   `json:"adapter"`
	TargetVersion    string   `json:"target_version"`
	ScenarioName     string   `json:"scenario_name"`
	ScenarioYAML     string   `json:"scenario_yaml,omitempty"`
	Recipe           string   `json:"recipe"`
	FaultType        string   `json:"fault_type"`
	Seed             uint64   `json:"seed"`
	Verdict          string   `json:"verdict"`
	Reason           string   `json:"reason"`
	Flags            []string `json:"flags,omitempty"`
	Flaky            bool     `json:"flaky,omitempty"`
	DurationMS       int64    `json:"duration_ms"`
	BytesTransferred int64    `json:"bytes_transferred"`
	OSArch           string   `json:"os_arch"`
	CreatedAt        string   `json:"created_at"`
	Entries          []Entry  `json:"entries,omitempty"`
}

// Store persists completed runs into a SQLite database file (when sqlite3 is available)
// alongside a canonical JSONL stream for zero-dependency portability.
type Store struct {
	dbPath string
	mu     sync.Mutex
}

// OpenStore opens or initializes a Store at dbPath.
func OpenStore(dbPath string) (*Store, error) {
	if dbPath == "" {
		dbPath = "runs.db"
	}
	s := &Store{dbPath: dbPath}
	if hasSQLite3() {
		schema := `CREATE TABLE IF NOT EXISTS runs (
			run_id TEXT PRIMARY KEY,
			adapter TEXT,
			target_version TEXT,
			scenario_name TEXT,
			recipe TEXT,
			fault_type TEXT,
			seed INTEGER,
			verdict TEXT,
			reason TEXT,
			flaky INTEGER,
			duration_ms INTEGER,
			bytes_transferred INTEGER,
			os_arch TEXT,
			created_at TEXT,
			payload_json TEXT
		);`
		_ = exec.Command("sqlite3", dbPath, schema).Run()
	}
	return s, nil
}

// SaveRun inserts or updates a StoredRun record in the store.
func (s *Store) SaveRun(r StoredRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if r.OSArch == "" {
		r.OSArch = runtime.GOOS + "/" + runtime.GOARCH
	}
	if r.CreatedAt == "" {
		r.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	raw, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshal stored run: %w", err)
	}

	// Update canonical JSONL store file
	all, _ := s.loadAllLocked()
	replaced := false
	for i := range all {
		if all[i].RunID == r.RunID {
			all[i] = r
			replaced = true
			break
		}
	}
	if !replaced {
		all = append(all, r)
	}
	if err := s.writeAllLocked(all); err != nil {
		return err
	}

	if hasSQLite3() {
		flakyInt := 0
		if r.Flaky {
			flakyInt = 1
		}
		sql := fmt.Sprintf(
			"INSERT OR REPLACE INTO runs VALUES ('%s','%s','%s','%s','%s','%s',%d,'%s','%s',%d,%d,%d,'%s','%s','%s');",
			escapeSQL(r.RunID),
			escapeSQL(r.Adapter),
			escapeSQL(r.TargetVersion),
			escapeSQL(r.ScenarioName),
			escapeSQL(r.Recipe),
			escapeSQL(r.FaultType),
			r.Seed,
			escapeSQL(r.Verdict),
			escapeSQL(r.Reason),
			flakyInt,
			r.DurationMS,
			r.BytesTransferred,
			escapeSQL(r.OSArch),
			escapeSQL(r.CreatedAt),
			escapeSQL(string(raw)),
		)
		_ = exec.Command("sqlite3", s.dbPath, sql).Run()
	}
	return nil
}

// GetRun retrieves a StoredRun by its RunID.
func (s *Store) GetRun(runID string) (*StoredRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	all, err := s.loadAllLocked()
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].RunID == runID {
			return &all[i], nil
		}
	}
	return nil, fmt.Errorf("run %q not found in %s", runID, s.dbPath)
}

// ListRuns returns all stored runs in insertion order.
func (s *Store) ListRuns() ([]StoredRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadAllLocked()
}

func (s *Store) jsonlPath() string {
	return s.dbPath + ".jsonl"
}

func (s *Store) loadAllLocked() ([]StoredRun, error) {
	f, err := os.Open(s.jsonlPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var runs []StoredRun
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var r StoredRun
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, sc.Err()
}

func (s *Store) writeAllLocked(runs []StoredRun) error {
	tmp := s.jsonlPath() + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	for _, r := range runs {
		raw, err := json.Marshal(r)
		if err != nil {
			_ = f.Close()
			return err
		}
		if _, err := f.Write(append(raw, '\n')); err != nil {
			_ = f.Close()
			return err
		}
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.jsonlPath())
}

func hasSQLite3() bool {
	_, err := exec.LookPath("sqlite3")
	return err == nil
}

func escapeSQL(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
