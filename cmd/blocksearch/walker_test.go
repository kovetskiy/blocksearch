package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// An unreadable directory must surface as a non-nil error from Walk instead
// of looking like a successful empty search. Root bypasses file permissions,
// so the suite skips when it cannot construct a permission barrier.
func TestWalkReportsUnreadableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: chmod cannot create an unreadable directory")
	}

	dir := t.TempDir()
	inner := filepath.Join(dir, "locked")
	if err := os.Mkdir(inner, 0o755); err != nil {
		t.Fatalf("mkdir inner: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inner, "target.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write inner file: %v", err)
	}
	if err := os.Chmod(inner, 0o000); err != nil {
		t.Fatalf("chmod inner: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(inner, 0o755) })

	walker := NewFileWalker(dir, nil, nil)
	err := walker.Walk(dir, func(string) error { return nil })
	if err == nil {
		t.Fatalf("Walk: err = nil, want non-nil for an unreadable directory")
	}
}

// A broken symlink given as an explicit top-level path is an input the
// caller asked for, so Walk surfaces its resolution error instead of
// looking like a successful empty search.
func TestWalkReportsBrokenSymlinkAtTopLevel(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "missing")
	link := filepath.Join(dir, "dangling")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	walker := NewFileWalker(dir, nil, nil)
	err := walker.Walk(link, func(string) error { return nil })
	if err == nil {
		t.Fatalf("Walk: err = nil, want non-nil for a broken top-level symlink")
	}
}

// A broken symlink discovered while walking a directory is skipped, not
// reported: only explicit top-level inputs surface resolution errors.
func TestWalkSkipsBrokenSymlinkDuringTraversal(t *testing.T) {
	dir := t.TempDir()
	realFile := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(realFile, []byte("needle"), 0o644); err != nil {
		t.Fatalf("write real file: %v", err)
	}
	if err := os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, "dangling")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	var seen []string
	walker := NewFileWalker(dir, nil, nil)
	err := walker.Walk(dir, func(path string) error {
		seen = append(seen, path)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: err = %v, want nil when a broken symlink is only traversed", err)
	}
	if len(seen) != 1 || seen[0] != realFile {
		t.Fatalf("seen = %v, want only %s", seen, realFile)
	}
}

// NewFileWalker must not read the environment: the global ignore matcher
// stays nil until the CLI path injects it, so tests are reproducible across
// machines with different ~/.gitignore_global files.
func TestNewFileWalkerHasNoGlobalIgnoreByDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	walker := NewFileWalker(t.TempDir(), nil, nil)
	if walker.globalIgnoreMatcher != nil {
		t.Fatalf("globalIgnoreMatcher = %v, want nil without explicit injection", walker.globalIgnoreMatcher)
	}
}

func contains(slice []string, want string) bool {
	for _, s := range slice {
		if s == want {
			return true
		}
	}
	return false
}

// Bug: .gitignore files in searched directories were ignored because the
// walker only loaded a single ignore file (the base directory's). A pattern
// in a nested .gitignore must suppress matching files during the walk.
func TestFileWalkerLoadsGitignoreFromSearchedDirectory(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	if err := os.Mkdir(proj, 0o755); err != nil {
		t.Fatalf("%v", err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".gitignore"), []byte("ignored.go\n"), 0o644); err != nil {
		t.Fatalf("%v", err)
	}
	if err := os.WriteFile(filepath.Join(proj, "ignored.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("%v", err)
	}
	if err := os.WriteFile(filepath.Join(proj, "kept.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("%v", err)
	}

	walker := NewFileWalker(root, nil, nil)
	var seen []string
	if err := walker.Walk(proj, func(path string) error {
		seen = append(seen, filepath.Base(path))
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}

	sort.Strings(seen)

	if !contains(seen, "kept.go") {
		t.Fatalf("seen = %v, want kept.go present", seen)
	}
	if contains(seen, "ignored.go") {
		t.Fatalf("seen = %v, want ignored.go absent (nested .gitignore must suppress ignored.go)", seen)
	}
}

// Bug: an empty HOME made ./<cwd>/.gitignore_global act as the global ignore
// file because filepath.Join("", ".gitignore_global") collapses to a relative
// path. An unset HOME must disable the global ignore file, not read one from
// the current directory.
func TestEmptyHomeDoesNotLoadGlobalGitignoreFromCwd(t *testing.T) {
	dir := t.TempDir()
	// A .gitignore_global sitting in the directory the CLI would treat as ".".
	if err := os.WriteFile(filepath.Join(dir, ".gitignore_global"), []byte("*.go\n"), 0o644); err != nil {
		t.Fatalf("%v", err)
	}
	t.Setenv("HOME", "")
	origWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("%v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	walker := newFileWalkerForCLI(nil, nil)
	if walker.globalIgnoreMatcher != nil {
		t.Fatalf("globalIgnoreMatcher = non-nil, want nil when HOME is unset")
	}
}
