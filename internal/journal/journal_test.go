package journal

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestJournalAppendReadAndTap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.jsonl")
	j, err := Open("run-001", path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	tapped := 0
	j.RegisterTap(func(e Entry) {
		tapped = e.OpIndex
	})

	err = j.Append(Entry{
		OpIndex:          1,
		Side:             "A",
		Op:               "create",
		Path:             "notes/a.txt",
		ContentHashAfter: "abc123",
		Bytes:            42,
	})
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	if tapped != 1 {
		t.Fatalf("expected tap callback to see OpIndex 1, got %d", tapped)
	}

	det1, err := j.DeterministicJSONL()
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	entries, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if len(entries) != 1 || entries[0].Path != "notes/a.txt" {
		t.Fatalf("unexpected entries read back: %+v", entries)
	}
	if !bytes.Contains(det1, []byte(`"path":"notes/a.txt"`)) {
		t.Fatalf("unexpected deterministic JSONL: %s", string(det1))
	}
}
