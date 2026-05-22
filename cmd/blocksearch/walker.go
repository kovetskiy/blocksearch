package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/monochromegane/go-gitignore"
	"github.com/reconquest/karma-go"
)

// FileWalker walks the filesystem, honoring .gitignore patterns and include/
// exclude globs. A single gate — processFileIfAllowed — applies those rules
// to every file before it reaches the caller, so explicit paths and paths
// discovered during the walk cannot bypass the filters.
type FileWalker struct {
	ignoreMatcher       gitignore.IgnoreMatcher
	globalIgnoreMatcher gitignore.IgnoreMatcher
	includes            []string
	excludes            []string
	root                string
}

// NewFileWalker builds a walker for baseDir that honors its .gitignore plus
// the given include/exclude globs. It does not touch the environment:
// the optional global ignore matcher is injected by the CLI path through
// SetGlobalIgnore, so tests are not sensitive to HOME.
func NewFileWalker(baseDir string, includes, excludes []string) *FileWalker {
	fw := &FileWalker{
		includes: includes,
		excludes: excludes,
	}

	gitignorePath := filepath.Join(baseDir, ".gitignore")
	fw.ignoreMatcher, _ = gitignore.NewGitIgnore(gitignorePath)

	return fw
}

// SetGlobalIgnore installs a matcher for a user-level ignore file such as
// ~/.gitignore_global. The CLI path builds it from HOME; tests leave it nil
// so their results do not depend on the developer's environment.
func (fw *FileWalker) SetGlobalIgnore(matcher gitignore.IgnoreMatcher) {
	fw.globalIgnoreMatcher = matcher
}

func (fw *FileWalker) Walk(path string, processFile func(path string) error) error {
	fw.root = path
	stat, err := os.Lstat(path)
	if err != nil {
		return err
	}

	if isSymlink(stat) {
		targetStat, err := os.Stat(path)
		if err != nil {
			// A broken top-level path is an explicit input the caller asked
			// for, so its resolution error surfaces instead of looking like
			// a successful empty search. Broken symlinks found only while
			// walking a directory are still skipped (see walkDir).
			return err
		}
		if !targetStat.IsDir() {
			return fw.processFileIfAllowed(path, fw.baseMatchers(), processFile)
		}

		return fw.walkDir(path, processFile, map[string]struct{}{}, fw.baseMatchers())
	}

	if stat.IsDir() {
		return fw.walkDir(path, processFile, map[string]struct{}{}, fw.baseMatchers())
	}

	return fw.processFileIfAllowed(path, fw.baseMatchers(), processFile)
}

// processFileIfAllowed applies include/exclude/gitignore rules to a single
// file and, if it passes, hands it to processFile. Explicit regular files and
// symlinked files both go through here, so neither can skip the filters.
// matchers is the chain of ignore files accumulated from the root down to
// this file's directory.
func (fw *FileWalker) processFileIfAllowed(
	path string,
	matchers []gitignore.IgnoreMatcher,
	processFile func(path string) error,
) error {
	if !fw.shouldProcessFile(path, matchers) {
		return nil
	}

	return processFile(path)
}

// baseMatchers is the always-on ignore chain applied to top-level inputs:
// the base directory's .gitignore plus the user-level global ignore file.
func (fw *FileWalker) baseMatchers() []gitignore.IgnoreMatcher {
	var matchers []gitignore.IgnoreMatcher
	if fw.ignoreMatcher != nil {
		matchers = append(matchers, fw.ignoreMatcher)
	}
	if fw.globalIgnoreMatcher != nil {
		matchers = append(matchers, fw.globalIgnoreMatcher)
	}
	return matchers
}

// loadDirIgnore reads <dir>/.gitignore, if present, returning a matcher
// whose base is dir so patterns resolve against paths relative to it.
func loadDirIgnore(dir string) (gitignore.IgnoreMatcher, bool) {
	matcher, err := gitignore.NewGitIgnore(filepath.Join(dir, ".gitignore"))
	if err != nil {
		return nil, false
	}
	return matcher, true
}

// walkDir descends path, threading the ignore chain from the root. A
// directory's own ignore status is decided by the incoming matchers (its
// ancestors' .gitignore files); its own .gitignore is loaded and appended
// to the chain before visiting children, mirroring git's semantics.
func (fw *FileWalker) walkDir(
	path string,
	processFile func(path string) error,
	visited map[string]struct{},
	matchers []gitignore.IgnoreMatcher,
) error {
	if fw.shouldSkipDir(path, matchers) {
		return nil
	}

	realPath, err := filepath.EvalSymlinks(path)
	if err == nil {
		absPath, err := filepath.Abs(realPath)
		if err == nil {
			if _, ok := visited[absPath]; ok {
				return nil
			}

			visited[absPath] = struct{}{}
		}
	}

	if matcher, ok := loadDirIgnore(path); ok {
		matchers = append(matchers, matcher)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return karma.Describe("dir", path).Format(err, "read directory")
	}

	for _, entry := range entries {
		if err := fw.walkEntry(path, entry, processFile, visited, matchers); err != nil {
			return err
		}
	}

	return nil
}

func (fw *FileWalker) walkEntry(
	dir string,
	entry os.DirEntry,
	processFile func(path string) error,
	visited map[string]struct{},
	matchers []gitignore.IgnoreMatcher,
) error {
	path := filepath.Join(dir, entry.Name())
	info, err := os.Lstat(path)
	if err != nil {
		return nil
	}

	if isSymlink(info) {
		targetInfo, err := os.Stat(path)
		if err != nil {
			return nil
		}
		if targetInfo.IsDir() {
			return fw.walkDir(path, processFile, visited, matchers)
		}
	}

	if info.IsDir() {
		return fw.walkDir(path, processFile, visited, matchers)
	}

	return fw.processFileIfAllowed(path, matchers, processFile)
}

func (fw *FileWalker) shouldSkipDir(path string, matchers []gitignore.IgnoreMatcher) bool {
	if filepath.Base(path) == ".git" {
		return true
	}

	for _, matcher := range matchers {
		if matcher.Match(path, true) {
			return true
		}
	}

	return false
}

func (fw *FileWalker) shouldProcessFile(path string, matchers []gitignore.IgnoreMatcher) bool {
	rel := path
	if fw.root != "" {
		if r, err := filepath.Rel(fw.root, path); err == nil {
			rel = r
		}
	}

	if len(fw.includes) != 0 && !matchesGlobAny(rel, fw.includes) {
		return false
	}

	if len(fw.excludes) != 0 && matchesGlobAny(rel, fw.excludes) {
		return false
	}

	for _, matcher := range matchers {
		if matcher.Match(path, false) {
			return false
		}
	}

	return true
}

func isSymlink(info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink != 0
}

// validateGlobPatterns rejects bad patterns before the walk begins.
func validateGlobPatterns(patterns []string) error {
	for _, p := range patterns {
		if !doublestar.ValidatePattern(p) {
			return karma.
				Describe("pattern", p).
				Format(nil, "invalid glob pattern")
		}
	}
	return nil
}

// matchesGlobAny reports whether path matches any of the glob patterns.
// A pattern without a path separator matches against the file's basename
// only, so --include '*.go' hits files at any depth; a pattern containing
// a separator matches against the full forward-slashed path. This mirrors
// the convention used by ripgrep and fd.
func matchesGlobAny(path string, patterns []string) bool {
	slashPath := filepath.ToSlash(path)
	base := filepath.Base(slashPath)
	for _, p := range patterns {
		target := slashPath
		if !strings.Contains(p, "/") {
			target = base
		}
		ok, err := doublestar.Match(p, target)
		if err == nil && ok {
			return true
		}
	}
	return false
}

// isGlobPattern reports whether arg contains an unescaped glob
// metacharacter — the set doublestar treats specially: * ? [ {. A literal
// backslash escapes the next byte, matching doublestar's own scanner.
// Filenames containing these characters are rare, and an unquoted argument
// carrying them is expanded by the shell first, so minimal escaping suffices.
func isGlobPattern(arg string) bool {
	escaped := false
	for i := 0; i < len(arg); i++ {
		c := arg[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == '*' || c == '?' || c == '[' || c == '{' {
			return true
		}
	}
	return false
}

// resolveFileArg expands a <file> argument into the concrete paths to walk.
//
// An argument without glob metacharacters is a literal: it is returned
// unchanged so the existing os.Lstat path in Walk still surfaces a missing
// path as an error, exactly as before.
//
// An argument with metacharacters is a glob. doublestar.FilepathGlob cleans
// the pattern, splits the literal base directory from the glob portion, and
// roots the search at that base, so absolute patterns (/x/*.go) and ./ or
// ../ relatives work despite doublestar.Glob rejecting leading "/", "./",
// "../". Each returned path is then handed to Walk — and therefore to
// processFileIfAllowed — so expanded paths cannot bypass include/exclude or
// gitignore. A match may be a file or a directory; Walk recurses directories
// as it already does.
//
// A glob that matches no files is reported as an error, not a silent no-op:
// a <file> argument names inputs the caller expects to exist, so matching
// nothing is treated like a missing literal path.
func resolveFileArg(arg string) ([]string, error) {
	if !isGlobPattern(arg) {
		return []string{arg}, nil
	}

	matches, err := doublestar.FilepathGlob(arg)
	if err != nil {
		return nil, fmt.Errorf("invalid glob pattern %q: %w", arg, err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no files match glob %q", arg)
	}

	return matches, nil
}
