package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func newSearchForTest(t *testing.T, files []string) *Search {
	t.Helper()

	query, err := regexp.Compile("add")
	if err != nil {
		t.Fatalf("compile query: %v", err)
	}

	return &Search{
		query:  query,
		files:  files,
		output: OutputPolicy{JSON: true, ShowLine: true, UseColors: false},
		walker: NewFileWalker(".", nil, nil),
	}
}

// A missing input path must surface as a non-nil error from Run so main
// can exit non-zero, rather than being logged and swallowed.
func TestSearchRunReportsMissingPath(t *testing.T) {
	search := newSearchForTest(t, []string{"/no/such/blocksearch/path.go"})

	emittedBlockCount, err := search.Run()
	if err == nil {
		t.Fatalf("Run: err = nil, want non-nil for a missing path")
	}
	if emittedBlockCount != 0 {
		t.Fatalf("Run: emitted block count = %d, want 0 when the path is missing", emittedBlockCount)
	}
}

// A stdout write failure must propagate through emit and the ordered
// emitter up to Run, instead of being silently dropped after logging.
func TestSearchRunReportsStdoutWriteFailure(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "function.go")
	if err := os.WriteFile(src, []byte("package main\n\nfunc add() {}\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	// /dev/full fails every write with ENOSPC, so a successful emit would
	// prove the write was never checked. Root bypasses file permissions,
	// so a read-only regular file is not a reliable sink in this suite.
	full, err := os.OpenFile("/dev/full", os.O_WRONLY, 0)
	if err != nil {
		t.Skipf("open /dev/full: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = full
	t.Cleanup(func() {
		os.Stdout = origStdout
		full.Close()
	})

	search := newSearchForTest(t, []string{src})
	_, err = search.Run()
	if err == nil {
		t.Fatalf("Run: err = nil, want non-nil when stdout write fails")
	}
}

// A short write must surface as io.ErrShortWrite, not as success: writeAll
// promises the full buffer is written, and a partial write to stdout would
// otherwise be silently dropped.
func TestWriteAllReportsShortWrite(t *testing.T) {
	err := writeAll(shortWriter{}, []byte("blocksearch"))
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("writeAll: err = %v, want io.ErrShortWrite", err)
	}
}

// shortWriter accepts every write but discards most of it, modeling a
// writer that does not guarantee full writes.
type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return 1, nil
}

// A glob argument expands and every matched file is searched end to end,
// through the JSON emit path.
func TestSearchRunGlobArgSearchesAllMatches(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"a.go": "package main\n\nfunc add() {}\n",
		"b.go": "package main\n\nfunc add() {}\n",
	})

	search := newSearchForTest(t, []string{filepath.Join(dir, "*.go")})

	emittedBlockCount, err := search.Run()
	if err != nil {
		t.Fatalf("Run: err = %v, want nil", err)
	}
	if emittedBlockCount != 2 {
		t.Fatalf("Run: emitted block count = %d, want 2", emittedBlockCount)
	}
}

// A glob matching a directory recurses through Walk, searching every child.
func TestSearchRunGlobMatchingDirectoryRecurses(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"pkg/inside.go": "package main\n\nfunc add() {}\n",
	})

	search := newSearchForTest(t, []string{filepath.Join(dir, "pk*")})

	emittedBlockCount, err := search.Run()
	if err != nil {
		t.Fatalf("Run: err = %v, want nil", err)
	}
	if emittedBlockCount != 1 {
		t.Fatalf("Run: emitted block count = %d, want 1", emittedBlockCount)
	}
}

// A glob-expanded directory recursion still flows through the filter gate,
// so an --exclude drops matching children of the recursed directory.
func TestSearchRunGlobDirectoryRecursesThroughFilterGate(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"pkg/a.go":      "package main\n\nfunc add() {}\n",
		"pkg/a_test.go": "package main\n\nfunc add() {}\n",
	})

	query, err := regexp.Compile("add")
	if err != nil {
		t.Fatalf("compile query: %v", err)
	}
	search := &Search{
		query:  query,
		files:  []string{filepath.Join(dir, "pk*")},
		output: OutputPolicy{JSON: true, ShowLine: true, UseColors: false},
		walker: NewFileWalker(dir, nil, []string{"*_test.go"}),
	}

	emittedBlockCount, err := search.Run()
	if err != nil {
		t.Fatalf("Run: err = %v, want nil", err)
	}
	if emittedBlockCount != 1 {
		t.Fatalf("Run: emitted block count = %d, want 1 (a.go kept, a_test.go excluded)", emittedBlockCount)
	}
}

// A zero-match glob is an error with no results, mirroring a missing path.
func TestSearchRunZeroMatchGlobIsError(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"a.go": "x"})

	search := newSearchForTest(t, []string{filepath.Join(dir, "*.none")})

	emittedBlockCount, err := search.Run()
	if err == nil {
		t.Fatalf("Run: err = nil, want non-nil for a zero-match glob")
	}
	if emittedBlockCount != 0 {
		t.Fatalf("Run: emitted block count = %d, want 0", emittedBlockCount)
	}
}

// A malformed glob argument is an error.
func TestSearchRunBadPatternGlobIsError(t *testing.T) {
	search := newSearchForTest(t, []string{"["})

	_, err := search.Run()
	if err == nil {
		t.Fatalf("Run: err = nil, want non-nil for a malformed glob")
	}
}

// Literal paths keep working after glob expansion is wired in.
func TestSearchRunLiteralPathsStillWork(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"a.go":     "package main\n\nfunc add() {}\n",
		"sub/b.go": "package main\n\nfunc add() {}\n",
	})

	t.Run("file", func(t *testing.T) {
		search := newSearchForTest(t, []string{filepath.Join(dir, "a.go")})
		emittedBlockCount, err := search.Run()
		if err != nil {
			t.Fatalf("Run: err = %v, want nil", err)
		}
		if emittedBlockCount != 1 {
			t.Fatalf("Run: emitted block count = %d, want 1", emittedBlockCount)
		}
	})

	t.Run("directory", func(t *testing.T) {
		search := newSearchForTest(t, []string{filepath.Join(dir, "sub")})
		emittedBlockCount, err := search.Run()
		if err != nil {
			t.Fatalf("Run: err = %v, want nil", err)
		}
		if emittedBlockCount != 1 {
			t.Fatalf("Run: emitted block count = %d, want 1", emittedBlockCount)
		}
	})
}

// --literal escapes regex metacharacters, so "a.b.c" matches only the literal
// text and not "axbxc" (which the dot-as-wildcard regex would also match).
func TestSearchRunLiteralEscapesRegexMetacharacters(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"a.go": "package main\n\nfunc f() {\n\ta := axbxc\n\tb := a.b.c\n}\n",
	})

	regexSearch := buildSearchForTest(t, "a.b.c", filepath.Join(dir, "a.go"))
	emittedBlockCount, err := regexSearch.Run()
	if err != nil {
		t.Fatalf("Run(regex): err = %v, want nil", err)
	}
	if emittedBlockCount != 2 {
		t.Fatalf("Run(regex): emitted block count = %d, want 2 (wildcard . matches both)", emittedBlockCount)
	}

	literalSearch := buildSearchForTest(t, "--literal", "a.b.c", filepath.Join(dir, "a.go"))
	emittedBlockCount, err = literalSearch.Run()
	if err != nil {
		t.Fatalf("Run(literal): err = %v, want nil", err)
	}
	if emittedBlockCount != 1 {
		t.Fatalf("Run(literal): emitted block count = %d, want 1 (only the literal a.b.c)", emittedBlockCount)
	}
}

// --literal accepts a query that would be a malformed regex (an unbalanced
// paren) and matches it literally instead of failing to compile.
func TestSearchRunLiteralAcceptsMalformedRegex(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"a.go": "package main\n\nfunc f() { x := (group) }\n",
	})

	literalSearch := buildSearchForTest(t, "--literal", "(group", filepath.Join(dir, "a.go"))
	emittedBlockCount, err := literalSearch.Run()
	if err != nil {
		t.Fatalf("Run(literal): err = %v, want nil", err)
	}
	if emittedBlockCount != 1 {
		t.Fatalf("Run(literal): emitted block count = %d, want 1", emittedBlockCount)
	}
}

// The --files (FilesOnly) path counts every glob match once, unaffected by
// expansion.
func TestSearchRunGlobFilesOnlyCountsMatches(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"a.go": "package main\n\nfunc add() {}\n",
		"b.go": "package main\n\nfunc add() {}\n",
	})

	query, err := regexp.Compile("add")
	if err != nil {
		t.Fatalf("compile query: %v", err)
	}
	search := &Search{
		query: query,
		files: []string{filepath.Join(dir, "*.go")},
		output: OutputPolicy{
			FilesOnly: true,
			JSON:      true,
			ShowLine:  true,
			UseColors: false,
		},
		walker: NewFileWalker(dir, nil, nil),
	}

	emittedBlockCount, err := search.Run()
	if err != nil {
		t.Fatalf("Run: err = %v, want nil", err)
	}
	if emittedBlockCount != 2 {
		t.Fatalf("Run: emitted block count = %d, want 2", emittedBlockCount)
	}
}
