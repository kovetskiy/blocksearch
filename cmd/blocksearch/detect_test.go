package main

import (
	"path/filepath"
	"testing"
)

func TestEveryRegisteredLanguageLookupReturnsLanguage(t *testing.T) {
	for ext := range extensionLanguages {
		path := filepath.Join(t.TempDir(), "sample"+ext)
		if language := languageForExtension(path); language == nil {
			t.Errorf("extension %q produced no language", ext)
		}
	}

	for name := range filenameLanguages {
		path := filepath.Join(t.TempDir(), name)
		if language := languageForExtension(path); language == nil {
			t.Errorf("filename %q produced no language", name)
		}
	}
}

// Regression for the languages that were once unsupported: markdown and
// yaml must now resolve and return a block, not be silently skipped.
func TestMarkdownAndYamlAreSupported(t *testing.T) {
	for _, fixture := range []struct {
		name     string
		contents string
	}{
		{name: "notes.md", contents: "# Title\n\nneedle in a paragraph\n"},
		{name: "config.yaml", contents: "key: needle\n"},
	} {
		path := writeBlockFixture(t, fixture.name, []byte(fixture.contents))
		blocks := findBlocksForTest(t, path, "needle")
		if len(blocks) == 0 {
			t.Fatalf("%s returned no blocks", fixture.name)
		}
	}
}

// Bug: a BOM-prefixed, extensionless shebang script was skipped because the
// BOM was stripped only during hashline normalization, after language
// detection had already rejected the shebang. Detection must tolerate a BOM.
func TestDetectLanguageHandlesBOMPrefixedShebang(t *testing.T) {
	path := filepath.Join(t.TempDir(), "script")
	contents := []byte("\xef\xbb\xbf#!/usr/bin/env python3\ndef target():\n    return 1\n")

	if language := detectLanguage(path, contents); language == nil {
		t.Fatalf("detectLanguage = nil, want the python grammar for a BOM-prefixed shebang")
	}
}
