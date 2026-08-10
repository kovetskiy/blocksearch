package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func parseArgumentsForTest(t *testing.T, argv ...string) Arguments {
	t.Helper()

	args, err := parseArgumentsArgs(argv)
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}

	return args
}

func TestParseArgumentsAfterDoubleDash(t *testing.T) {
	args := parseArgumentsForTest(t, "-x", "go", "--", "--help")

	if args.ValueQuery != "--help" {
		t.Fatalf("query = %q, want %q", args.ValueQuery, "--help")
	}

	if len(args.ValueFiles) != 0 {
		t.Fatalf("files = %q, want none", args.ValueFiles)
	}

	if len(args.ValueIncludes) != 1 || args.ValueIncludes[0] != "go" {
		t.Fatalf("includes = %q, want %q", args.ValueIncludes, []string{"go"})
	}
}

func TestOutputFlagsKeepRawMeaning(t *testing.T) {
	defaults := parseArgumentsForTest(t, "query")
	if defaults.FlagNoLine || defaults.FlagHashline {
		t.Fatalf("output flags = (%v, %v), want both false by default", defaults.FlagNoLine, defaults.FlagHashline)
	}

	given := parseArgumentsForTest(t, "--no-line", "--hashline", "query")
	if !given.FlagNoLine || !given.FlagHashline {
		t.Fatalf("output flags = (%v, %v), want both true when present", given.FlagNoLine, given.FlagHashline)
	}
}

func TestOutputPolicyHashlineEnablesHashline(t *testing.T) {
	search := buildSearchForTest(t, "--hashline", "query", "unused")
	if !search.output.Hashline {
		t.Fatalf("Hashline = false, want true when --hashline is given")
	}
}

type symlinkWalkFixture struct {
	root        string
	symlinkFile string
	symlinkDir  string
}

func newSymlinkWalkFixture(t *testing.T) symlinkWalkFixture {
	t.Helper()

	root := t.TempDir()
	realFile := filepath.Join(root, "real.txt")
	if err := os.WriteFile(realFile, []byte("real"), 0o644); err != nil {
		t.Fatalf("write real file: %v", err)
	}

	realDir := filepath.Join(root, "real-dir")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatalf("mkdir real dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "nested.txt"), []byte("nested"), 0o644); err != nil {
		t.Fatalf("write nested file: %v", err)
	}

	symlinkFile := filepath.Join(root, "symlink-file.txt")
	if err := os.Symlink(realFile, symlinkFile); err != nil {
		if errors.Is(err, os.ErrPermission) {
			t.Skip("symlink creation is not permitted")
		}
		t.Fatalf("symlink file: %v", err)
	}

	symlinkDir := filepath.Join(root, "symlink-dir")
	if err := os.Symlink(realDir, symlinkDir); err != nil {
		t.Fatalf("symlink dir: %v", err)
	}

	return symlinkWalkFixture{
		root:        root,
		symlinkFile: symlinkFile,
		symlinkDir:  symlinkDir,
	}
}

func TestFileWalkerHandlesSymlinkPathForms(t *testing.T) {
	fixture := newSymlinkWalkFixture(t)
	walker := NewFileWalker(fixture.root, nil, nil)

	t.Run("directory walk", func(t *testing.T) {
		var walked []string
		if err := walker.Walk(fixture.root, func(path string) error {
			rel, err := filepath.Rel(fixture.root, path)
			if err != nil {
				t.Fatalf("relative path: %v", err)
			}
			walked = append(walked, filepath.ToSlash(rel))
			return nil
		}); err != nil {
			t.Fatalf("walk: %v", err)
		}

		sort.Strings(walked)
		want := []string{"real-dir/nested.txt", "real.txt", "symlink-file.txt"}
		if !reflect.DeepEqual(walked, want) {
			t.Fatalf("walked = %q, want %q", walked, want)
		}
	})

	t.Run("explicit symlink file", func(t *testing.T) {
		called := false
		if err := walker.Walk(fixture.symlinkFile, func(string) error {
			called = true
			return nil
		}); err != nil {
			t.Fatalf("walk symlink file: %v", err)
		}
		if !called {
			t.Fatalf("explicit symlink file was not processed")
		}
	})

	t.Run("explicit symlink directory", func(t *testing.T) {
		var walked []string
		if err := walker.Walk(fixture.symlinkDir, func(path string) error {
			rel, err := filepath.Rel(fixture.symlinkDir, path)
			if err != nil {
				t.Fatalf("relative path: %v", err)
			}
			walked = append(walked, filepath.ToSlash(rel))
			return nil
		}); err != nil {
			t.Fatalf("walk symlink dir: %v", err)
		}

		if !reflect.DeepEqual(walked, []string{"nested.txt"}) {
			t.Fatalf("walked symlink dir = %q, want nested.txt", walked)
		}
	})
}

// buildSearchForTest builds a Search from the given CLI args so output-policy
// tests assert the resolved OutputPolicy instead of re-running docopt.
func buildSearchForTest(t *testing.T, argv ...string) *Search {
	t.Helper()
	args, err := parseArgumentsArgs(argv)
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}
	search, err := buildSearch(args)
	if err != nil {
		t.Fatalf("build search: %v", err)
	}
	return search
}

func emittedTextForArgs(t *testing.T, argv ...string) string {
	t.Helper()
	argv = append([]string{"--color", "never"}, argv...)
	search := buildSearchForTest(t, argv...)

	var output bytes.Buffer
	emitter := &blockEmitter{policy: search.output, writer: &output}
	blocks := Blocks{{{Line: 7, Hash: "AB", Text: "needle"}}}
	if err := emitter.emit(blocks, "fixture.go"); err != nil {
		t.Fatalf("emit: %v", err)
	}
	return output.String()
}

func TestOutputFlagsSelectObservableTextFormat(t *testing.T) {
	for _, testcase := range []struct {
		name string
		flag string
		want string
	}{
		{name: "no line", flag: "--no-line", want: "fixture.go\nneedle\n"},
		{name: "filename per line", flag: "--file", want: "fixture.go:7:needle\n"},
		{name: "hashline", flag: "--hashline", want: "fixture.go\n7#AB│needle\n"},
		{name: "default", flag: "", want: "fixture.go\n7:needle\n"},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			argv := []string{}
			if testcase.flag != "" {
				argv = append(argv, testcase.flag)
			}
			got := emittedTextForArgs(t, append(argv, "query", "unused")...)
			if got != testcase.want {
				t.Fatalf("output = %q, want %q", got, testcase.want)
			}
		})
	}
}

// Bug: an invalid AWK program was accepted as long as no block matched,
// because the program was only parsed when a matched block was evaluated.
// Invalid filters must fail during search construction, before any search.
func TestBuildSearchRejectsInvalidAwk(t *testing.T) {
	_, err := buildSearch(Arguments{
		ValueAwkIfs: []string{"this is not awk ("},
		ValueQuery:  "x",
		ValueFiles:  []string{"unused"},
	})
	if err == nil {
		t.Fatalf("buildSearch: err = nil, want non-nil for an invalid AWK program")
	}
}

// A valid AWK program must still be accepted so the fix does not over-reject.
func TestBuildSearchAcceptsValidAwk(t *testing.T) {
	_, err := buildSearch(Arguments{
		ValueAwkIfs: []string{"/needle/"},
		ValueQuery:  "x",
		ValueFiles:  []string{"unused"},
	})
	if err != nil {
		t.Fatalf("buildSearch: err = %v, want nil for a valid AWK program", err)
	}
}

// Hashline anchors carry a line number (LINE#HASH), so --no-line selects
// the classic format and wins over --hashline.
func TestOutputPolicyNoLineWinsOverHashline(t *testing.T) {
	search := buildSearchForTest(t, "--hashline", "--no-line", "query", "unused")
	if search.output.Hashline {
		t.Fatalf("Hashline = true, want false when --no-line is given")
	}
	if search.output.ShowLine {
		t.Fatalf("ShowLine = true, want false when --no-line is given")
	}
}

// The classic path:line format cannot carry hashline anchors, so --file
// wins over --hashline.
func TestOutputPolicyFileWinsOverHashline(t *testing.T) {
	search := buildSearchForTest(t, "--hashline", "--file", "query", "unused")
	if search.output.Hashline {
		t.Fatalf("Hashline = true, want false when --file is given")
	}
	if !search.output.ShowFilename {
		t.Fatalf("ShowFilename = false, want true when --file is given")
	}
}

func TestOutputPolicyDefaultDropsHashline(t *testing.T) {
	search := buildSearchForTest(t, "query", "unused")
	if search.output.Hashline {
		t.Fatalf("Hashline = true, want false by default")
	}
}
