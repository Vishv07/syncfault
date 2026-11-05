package sandbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrPathEscape is returned when an operation attempts to access a path outside the sandbox root.
var ErrPathEscape = errors.New("sandbox path escape blocked")

// ResolveAndConfine verifies that targetPath (after resolving any existing symlinks in its ancestor chain)
// resides strictly inside rootDir. It returns the canonicalized absolute path.
func ResolveAndConfine(rootDir, targetPath string) (string, error) {
	if rootDir == "" {
		return "", fmt.Errorf("%w: empty sandbox root", ErrPathEscape)
	}
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return "", fmt.Errorf("%w: invalid root: %v", ErrPathEscape, err)
	}
	realRoot, err := evalExistingSymlinks(absRoot)
	if err != nil {
		return "", fmt.Errorf("%w: cannot resolve root symlinks: %v", ErrPathEscape, err)
	}

	var candidate string
	if filepath.IsAbs(targetPath) {
		candidate = filepath.Clean(targetPath)
	} else {
		candidate = filepath.Join(realRoot, targetPath)
	}

	realCandidate, err := evalExistingSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrPathEscape, err)
	}

	rel, err := filepath.Rel(realRoot, realCandidate)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrPathEscape, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("%w: %q resolves to %q outside sandbox %q", ErrPathEscape, targetPath, realCandidate, realRoot)
	}
	return realCandidate, nil
}

// evalExistingSymlinks walks up the path until an existing prefix is found, evaluates symlinks on it,
// and re-attaches the remaining non-existent suffix components after verifying none contain "..".
func evalExistingSymlinks(absPath string) (string, error) {
	cleaned := filepath.Clean(absPath)
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}

	parent := filepath.Dir(cleaned)
	if parent == cleaned {
		return cleaned, nil
	}
	resolvedParent, err := evalExistingSymlinks(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolvedParent, filepath.Base(cleaned)), nil
}
