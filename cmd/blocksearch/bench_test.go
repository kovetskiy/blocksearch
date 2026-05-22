package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// corpusFile is a single sizeable Go source file used to exercise the
// parse-and-resolve pipeline without filesystem walking noise.
var corpusFile = filepath.Join("..", "..", "vendor", "golang.org", "x", "sys", "unix", "syscall_linux.go")

func loadCorpus(b *testing.B) []byte {
	b.Helper()
	contents, err := os.ReadFile(corpusFile)
	if err != nil {
		b.Skipf("corpus %s unavailable: %v", corpusFile, err)
	}
	return contents
}

// BenchmarkFindBlocks is the full per-file path: ReadFile, isText sniff,
// language detection, tree-sitter parse, per-line regex, node resolution,
// hash computation, and block construction.
func BenchmarkFindBlocks(b *testing.B) {
	query := regexp.MustCompile(`func`)
	path := corpusFile
	if _, err := os.Stat(path); err != nil {
		b.Skipf("corpus %s unavailable: %v", path, err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := findBlocks(path, query); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseBlocks isolates everything after the file is read: parse,
// search, resolve, hash, build. Removes os.ReadFile and content sniffing.
func BenchmarkParseBlocks(b *testing.B) {
	contents := loadCorpus(b)
	query := regexp.MustCompile(`func`)
	language := detectLanguage(corpusFile, contents)
	if language == nil {
		b.Fatal("no language for corpus")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := parseBlocks(contents, language, query); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHashlineHashes isolates the FNV line-hash computation, which runs
// over every line of every parsed file regardless of matches.
func BenchmarkHashlineHashes(b *testing.B) {
	contents := loadCorpus(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hashlineHashes(contents)
	}
}

// BenchmarkHashlineNormalize isolates the normalization (BOM strip, CRLF
// handling, string conversion) that runs at least twice per file: once for
// the parse input and once inside hashlineLines.
func BenchmarkHashlineNormalizeFile(b *testing.B) {
	contents := loadCorpus(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hashlineNormalizeFile(contents)
	}
}

// BenchmarkFormatColors measures the colored output path, which invokes
// chroma lexing + vim highlighting per block.
func BenchmarkFormatColors(b *testing.B) {
	blocks := syntheticBlocks(b, 64)
	options := FormatOptions{
		Filename:        "synthetic.go",
		ShowLineNumbers: true,
		UseColors:       true,
		Hashline:        true,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = blocks.Format(options)
	}
}

// syntheticBlocks builds n matching Go function blocks from an in-memory
// source so the colors benchmark does not depend on a corpus that yields
// resolvable named nodes.
func syntheticBlocks(b *testing.B, n int) Blocks {
	b.Helper()
	src := []byte("package main\n\n")
	for i := 0; i < n; i++ {
		src = append(src, []byte("func fn"+strconv.Itoa(i)+"() {\n\treturn\n}\n\n")...)
	}
	language := detectLanguage("synthetic.go", src)
	blocks, err := parseBlocks(src, language, regexp.MustCompile(`fn`))
	if err != nil {
		b.Fatalf("parse: %v", err)
	}
	if len(blocks) == 0 {
		b.Fatal("no blocks from synthetic source")
	}
	return blocks
}
