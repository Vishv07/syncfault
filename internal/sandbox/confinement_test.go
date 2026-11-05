package sandbox

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAndConfineAllowsValidSubpaths(t *testing.T) {
	root := t.TempDir()
	got, err := ResolveAndConfine(root, "sideA/docs/report.txt")
	if err != nil {
		t.Fatalf("expected valid path, got error: %v", err)
	}
	if filepath.Base(got) != "report.txt" {
		t.Fatalf("unexpected resolved path: %s", got)
	}
}

func TestResolveAndConfineBlocksDotDotEscape(t *testing.T) {
	root := t.TempDir()
	_, err := ResolveAndConfine(root, "../../etc/passwd")
	if !errors.Is(err, ErrPathEscape) {
		t.Fatalf("expected ErrPathEscape, got %v", err)
	}
}

func TestResolveAndConfineBlocksSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	linkPath := filepath.Join(root, "sideA", "evil_link")
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Fatal(err)
	}

	_, err := ResolveAndConfine(root, filepath.Join(linkPath, "secret.txt"))
	if !errors.Is(err, ErrPathEscape) {
		t.Fatalf("expected symlink escape to return ErrPathEscape, got %v", err)
	}
}
