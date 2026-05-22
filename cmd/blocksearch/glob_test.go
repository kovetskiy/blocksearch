package main

import (
	"path/filepath"
	"sort"
	"testing"
)

// collectGlobWalk returns sorted paths relative to the walk root.
func collectGlobWalk(t *testing.T, dir string, includes, excludes []string) []string {
	t.Helper()
	walker := NewFileWalker(dir, includes, excludes)
	var got []string
	err := walker.Walk(dir, func(path string) error {
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		got = append(got, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	sort.Strings(got)
	return got
}

func assertPathsEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("paths = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("paths = %v, want %v", got, want)
		}
	}
}

func mustContain(t *testing.T, got []string, want string) {
	t.Helper()
	for _, v := range got {
		if v == want {
			return
		}
	}
	t.Fatalf("got %v, want it to contain %q", got, want)
}

func mustNotContain(t *testing.T, got []string, want string) {
	t.Helper()
	for _, v := range got {
		if v == want {
			t.Fatalf("got %v, want it to NOT contain %q", got, want)
		}
	}
}

// --include with a basename glob (*.go) matches files at any depth, not
// just the top level.
func TestGlobIncludeBasenameMatchesAtAnyDepth(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"top.go":           "x",
		"top.txt":          "x",
		"src/nested.go":    "x",
		"src/nested.txt":   "x",
		"src/deep/deep.go": "x",
	})
	got := collectGlobWalk(t, dir, []string{"*.go"}, nil)
	assertPathsEqual(t, got, []string{"src/deep/deep.go", "src/nested.go", "top.go"})
}

// --include with a path glob (containing /) matches against the full path,
// so src/** selects only files under src/.
func TestGlobIncludePathPatternMatchesFull(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"top.go":       "x",
		"src/a.go":     "x",
		"src/sub/b.go": "x",
		"other/c.go":   "x",
	})
	got := collectGlobWalk(t, dir, []string{"src/**"}, nil)
	assertPathsEqual(t, got, []string{"src/a.go", "src/sub/b.go"})
}

// --exclude wins over --include when both match, mirroring the documented
// precedence.
func TestGlobExcludeWinsOverInclude(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"a.go":      "x",
		"b_test.go": "x",
		"c.go":      "x",
	})
	got := collectGlobWalk(t, dir, []string{"*.go"}, []string{"*_test.go"})
	assertPathsEqual(t, got, []string{"a.go", "c.go"})
}

// A glob with ** matches across directory boundaries.
func TestGlobDoubleStarCrossesDirectories(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"root.go":         "x",
		"a/b/c/d/deep.go": "x",
		"a/b/c/deep.go":   "x",
	})
	got := collectGlobWalk(t, dir, []string{"**/*.go"}, nil)
	assertPathsEqual(t, got, []string{"a/b/c/d/deep.go", "a/b/c/deep.go", "root.go"})
}

// An explicit single-file path still passes through the glob gate, so a
// mismatched --include excludes it.
func TestGlobExplicitFilePassesThroughGate(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "data.txt")
	writeTree(t, dir, map[string]string{"data.txt": "x"})

	walker := NewFileWalker(dir, []string{"*.go"}, nil)
	var called bool
	err := walker.Walk(file, func(path string) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if called {
		t.Fatalf("explicit file was processed despite not matching include glob")
	}
}

// A bad glob pattern is rejected before walking.
func TestValidateGlobPatternsRejectsBadPattern(t *testing.T) {
	err := validateGlobPatterns([]string{"["})
	if err == nil {
		t.Fatalf("validateGlobPatterns: err = nil, want error for malformed pattern")
	}
}

// A valid glob pattern passes validation.
func TestValidateGlobPatternsAcceptsValidPattern(t *testing.T) {
	for _, p := range []string{"*.go", "src/**", "**/*_test.go", "cmd/blocksearch/*.go"} {
		if err := validateGlobPatterns([]string{p}); err != nil {
			t.Fatalf("validateGlobPatterns(%q): err = %v, want nil", p, err)
		}
	}
}

// isGlobPattern detects exactly doublestar's metacharacter set, with a
// backslash escaping the next byte. Closing brackets/braces alone are not
// detected, so a literal such as a]b.go stays a literal.
func TestIsGlobPattern(t *testing.T) {
	for _, testcase := range []struct {
		arg  string
		want bool
	}{
		{"*.go", true},
		{"src/**/*.go", true},
		{"{a,b}.py", true},
		{"file[0-9].txt", true},
		{"a?b", true},
		{"*", true},
		{"plain.go", false},
		{"dir/a.go", false},
		{"", false},
		{`a\*b.go`, false}, // escaped '*' is literal
	} {
		if got := isGlobPattern(testcase.arg); got != testcase.want {
			t.Errorf("isGlobPattern(%q) = %v, want %v", testcase.arg, got, testcase.want)
		}
	}
}

// A literal argument (no metacharacters) is returned untouched, with no
// filesystem access, so a missing path still surfaces its error from Walk's
// os.Lstat instead of from expansion.
func TestResolveFileArgLiteralPassthrough(t *testing.T) {
	paths, err := resolveFileArg("missing.go")
	if err != nil {
		t.Fatalf("resolveFileArg(missing.go): err = %v, want nil", err)
	}
	if len(paths) != 1 || paths[0] != "missing.go" {
		t.Fatalf("resolveFileArg(missing.go) = %v, want [missing.go]", paths)
	}
}

// A glob expands to the matching files only; .go files are returned and the
// .txt file is excluded by the pattern itself.
func TestResolveFileArgExpandsToFiles(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"a.go":  "x",
		"b.go":  "x",
		"c.txt": "x",
	})

	paths, err := resolveFileArg(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("resolveFileArg: err = %v, want nil", err)
	}
	got := make([]string, len(paths))
	for i, p := range paths {
		got[i] = filepath.Base(p)
	}
	mustContain(t, got, "a.go")
	mustContain(t, got, "b.go")
	mustNotContain(t, got, "c.txt")
	if len(got) != 2 {
		t.Fatalf("resolveFileArg: got %v, want exactly two .go files", got)
	}
}

// An absolute glob (leading /) is normalized by FilepathGlob so the literal
// base directory roots the search instead of being rejected.
func TestResolveFileArgAbsoluteGlobMatches(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"only.go": "x"})

	paths, err := resolveFileArg(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("resolveFileArg: err = %v, want nil", err)
	}
	if len(paths) != 1 {
		t.Fatalf("resolveFileArg: got %v, want one match", paths)
	}
	if want := filepath.Join(dir, "only.go"); paths[0] != want {
		t.Fatalf("resolveFileArg: got %v, want %v", paths[0], want)
	}
}

// A glob that matches a directory returns the directory path; recursion is
// exercised at the Run level (Walk recurses directories as it already does).
func TestResolveFileArgMatchesDirectory(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"pkg/inside.go": "x"})

	paths, err := resolveFileArg(filepath.Join(dir, "pk*"))
	if err != nil {
		t.Fatalf("resolveFileArg: err = %v, want nil", err)
	}
	if len(paths) != 1 {
		t.Fatalf("resolveFileArg: got %v, want one match", paths)
	}
	if want := filepath.Join(dir, "pkg"); paths[0] != want {
		t.Fatalf("resolveFileArg: got %v, want %v", paths[0], want)
	}
}

// A malformed glob pattern is reported as an error, mirroring the up-front
// validation used for --include/--exclude.
func TestResolveFileArgBadPatternIsError(t *testing.T) {
	if _, err := resolveFileArg("["); err == nil {
		t.Fatalf("resolveFileArg([): err = nil, want error")
	}
}

// A glob that matches nothing is an error, not a silent no-op, so a typo or
// wrong directory does not masquerade as a successful empty search.
func TestResolveFileArgZeroMatchesIsError(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"a.go": "x"})

	if _, err := resolveFileArg(filepath.Join(dir, "*.none")); err == nil {
		t.Fatalf("resolveFileArg(*.none): err = nil, want error for zero matches")
	}
}
