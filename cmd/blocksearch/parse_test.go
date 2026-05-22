package main

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"
)

func TestFindBlocksExtractsMultilineGoCall(t *testing.T) {
	path := writeBlockFixture(t, "call.go", []byte("package main\n\nfunc f() {\n\ttransform(\n\t\ta,\n\t\tb,\n\t)\n}\n"))

	blocks := findBlocksForTest(t, path, "transform")

	want := Block{
		{Line: 4, Text: "\ttransform("},
		{Line: 5, Text: "\t\ta,"},
		{Line: 6, Text: "\t\tb,"},
		{Line: 7, Text: "\t)"},
	}
	if !reflect.DeepEqual(blocks, Blocks{want}) {
		t.Fatalf("blocks = %#v, want %#v", blocks, Blocks{want})
	}
}

func TestFindBlocksUsesSmallestNamedGoNode(t *testing.T) {
	path := writeBlockFixture(t, "smallest.go", []byte("package main\n\nfunc f() int {\n\treturn value\n}\n"))

	returnBlocks := findBlocksForTest(t, path, "return")
	wantReturn := Blocks{
		{
			{Line: 4, Text: "\treturn value"},
		},
	}
	if !reflect.DeepEqual(returnBlocks, wantReturn) {
		t.Fatalf("return blocks = %#v, want %#v", returnBlocks, wantReturn)
	}

	funcBlocks := findBlocksForTest(t, path, "func")
	wantFunc := Blocks{
		{
			{Line: 3, Text: "func f() int {"},
			{Line: 4, Text: "\treturn value"},
			{Line: 5, Text: "}"},
		},
	}
	if !reflect.DeepEqual(funcBlocks, wantFunc) {
		t.Fatalf("func blocks = %#v, want %#v", funcBlocks, wantFunc)
	}
}

func TestFindBlocksDeduplicatesNodeMatches(t *testing.T) {
	path := writeBlockFixture(t, "dedupe.go", []byte("package main\n\nfunc f() {\n\tmessage := `alpha beta`\n\t_ = message\n}\n"))

	blocks := findBlocksForTest(t, path, "alpha|beta")

	want := Blocks{
		{
			{Line: 4, Text: "\tmessage := `alpha beta`"},
		},
	}
	if !reflect.DeepEqual(blocks, want) {
		t.Fatalf("blocks = %#v, want %#v", blocks, want)
	}
}

// Several matches that resolve to distinct identifier nodes on the
// same line have different byte ranges but identical line spans, so the
// resulting single-line block must appear once, not once per match.
func TestFindBlocksDeduplicatesSameLineDistinctNodes(t *testing.T) {
	path := writeBlockFixture(t, "sameline.go", []byte("package main\n\nfunc f() {\n\tx := foo + bar\n}\n"))

	blocks := findBlocksForTest(t, path, "foo|bar")

	want := Blocks{
		{
			{Line: 4, Text: "\tx := foo + bar"},
		},
	}
	if !reflect.DeepEqual(blocks, want) {
		t.Fatalf("blocks = %#v, want %#v", blocks, want)
	}
}

// A query that matches both an enclosing construct name and a distinct
// nested statement must keep both blocks; containment alone no longer
// suppresses a separate node.
func TestFindBlocksKeepsEnclosingNameAndNestedStatement(t *testing.T) {
	path := writeBlockFixture(t, "nested.go", []byte("package main\n\nfunc add(a int, b int) int {\n\treturn a + b\n}\n"))

	blocks := findBlocksForTest(t, path, "add|return")

	want := Blocks{
		{
			{Line: 3, Text: "func add(a int, b int) int {"},
			{Line: 4, Text: "\treturn a + b"},
			{Line: 5, Text: "}"},
		},
		{
			{Line: 4, Text: "\treturn a + b"},
		},
	}
	if !reflect.DeepEqual(blocks, want) {
		t.Fatalf("blocks = %#v, want %#v", blocks, want)
	}
}

func TestFindBlocksSkipsFilesWithoutGrammar(t *testing.T) {
	for _, fixture := range []struct {
		name     string
		contents string
	}{
		{name: "plain.txt", contents: "needle\n"},
		{name: "notes", contents: "needle\n"},
	} {
		path := writeBlockFixture(t, fixture.name, []byte(fixture.contents))
		blocks := findBlocksForTest(t, path, "no_match")
		if len(blocks) != 0 {
			t.Fatalf("%s blocks = %#v, want none", fixture.name, blocks)
		}
	}
}

func TestFallbackBlocksIndentation(t *testing.T) {
	path := writeBlockFixture(t, "targets.txt", []byte(
		"header\n"+
			"\tindented line\n"+
			"\t\tdoubly indented\n"+
			"not indented\n"+
			"\tanother block\n",
	))

	blocks := findBlocksForTest(t, path, "header")
	want := Blocks{
		{
			{Line: 1, Text: "header"},
			{Line: 2, Text: "\tindented line"},
			{Line: 3, Text: "\t\tdoubly indented"},
		},
	}
	if !reflect.DeepEqual(blocks, want) {
		t.Fatalf("blocks = %#v, want %#v", blocks, want)
	}
}

func TestFallbackBlocksSingleLine(t *testing.T) {
	path := writeBlockFixture(t, "config.env", []byte("LOG_LEVEL=info\nPORT=8080\n"))

	blocks := findBlocksForTest(t, path, "PORT")
	want := Blocks{
		{
			{Line: 2, Text: "PORT=8080"},
		},
	}
	if !reflect.DeepEqual(blocks, want) {
		t.Fatalf("blocks = %#v, want %#v", blocks, want)
	}
}

func TestFallbackBlocksDeduplication(t *testing.T) {
	path := writeBlockFixture(t, "script.txt", []byte("# header\n\tindented\n"))

	// Two matches on the same line (indented) should produce one block
	blocks := findBlocksForTest(t, path, "indented|dent")
	if len(blocks) != 1 {
		t.Fatalf("blocks = %#v, want 1 block", blocks)
	}
}

func TestFindBlocksSkipsBinaryFiles(t *testing.T) {
	path := writeBlockFixture(t, "binary.go", []byte{0x7f, 'E', 'L', 'F', 0x00, 'n', 'e', 'e', 'd', 'l', 'e'})

	blocks := findBlocksForTest(t, path, "needle")
	if len(blocks) != 0 {
		t.Fatalf("blocks = %#v, want none", blocks)
	}
}

func writeBlockFixture(t *testing.T, name string, contents []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	return path
}

func findBlocksForTest(t *testing.T, path string, pattern string) Blocks {
	t.Helper()

	query, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("compile query: %v", err)
	}

	blocks, err := findBlocks(path, query)
	if err != nil {
		t.Fatalf("find blocks: %v", err)
	}

	return blocksWithoutHashes(blocks)
}

func blocksWithoutHashes(blocks Blocks) Blocks {
	result := make(Blocks, len(blocks))
	for i := 0; i < len(blocks); i++ {
		result[i] = make(Block, len(blocks[i]))
		for j := 0; j < len(blocks[i]); j++ {
			result[i][j] = blocks[i][j]
			result[i][j].Hash = ""
		}
	}
	return result
}

// requireBlockWithExactSpan fails unless a block has the requested start and end lines.
func requireBlockWithExactSpan(t *testing.T, blocks Blocks, wantStart, wantEnd int) {
	t.Helper()
	for _, b := range blocks {
		if b.LineStart() == wantStart && b.LineEnd() == wantEnd {
			return
		}
	}
	t.Fatalf("no block has exact span %d-%d; blocks = %#v", wantStart, wantEnd, blocks)
}

// Bug: a match inside an if/loop condition returned only the header line.
// The whole compound statement (the if ... fi) must be returned instead.
func TestFindBlocksIfConditionReturnsWholeCompoundStatement(t *testing.T) {
	path := writeBlockFixture(t, "ifcond.sh", []byte("#!/bin/bash\n"+
		"if ! declare -f import:use &>/dev/null; then\n"+
		"    import:use foo\n"+
		"fi\n"))

	blocks := findBlocksForTest(t, path, "import:use")
	requireBlockWithExactSpan(t, blocks, 2, 4) // the whole if ... fi
}

// Bug: a non-name match inside a multiline statement returned only the leaf
// line. The whole enclosing statement (the assignment/call) must be returned.
func TestFindBlocksNonNameMatchReturnsWholeMultilineStatement(t *testing.T) {
	path := writeBlockFixture(t, "call.go", []byte("package main\n\nfunc f() {\n"+
		"\tresult := transform(\n"+
		"\t\tinput,\n"+
		"\t\tother,\n"+
		"\t)\n"+
		"\t_ = result\n"+
		"}\n"))

	blocks := findBlocksForTest(t, path, "input")
	requireBlockWithExactSpan(t, blocks, 4, 7) // result := transform( ... )
}

// Bug: a bare C function name (declared through a declarator, not a `name`
// field) returned only the header line. The whole function_definition must
// be returned.
func TestFindBlocksBareCFunctionNameReturnsWholeDefinition(t *testing.T) {
	path := filepath.Join("..", "..", "tests", "testdata", "src", "handler.c")
	blocks := findBlocksForTest(t, path, "prepend")
	requireBlockWithExactSpan(t, blocks, 14, 22) // static node_t *prepend(...) { ... }
}

// Bug: a bare OCaml let binding name returned only the header line. The
// whole value_definition must be returned.
func TestFindBlocksBareOCamlNameReturnsWholeDefinition(t *testing.T) {
	path := filepath.Join("..", "..", "tests", "testdata", "src", "parser.ml")
	blocks := findBlocksForTest(t, path, "compute_total")
	requireBlockWithExactSpan(t, blocks, 3, 5) // let compute_total subtotal = ... +. tax
}

// Bug: a bare Elixir def name returned only the header line. The whole def
// clause (through its do/end block) must be returned.
func TestFindBlocksBareElixirNameReturnsWholeDefClause(t *testing.T) {
	path := filepath.Join("..", "..", "tests", "testdata", "src", "worker.ex")
	blocks := findBlocksForTest(t, path, "process_invoice")
	requireBlockWithExactSpan(t, blocks, 9, 11) // def process_invoice(invoice) do ... end
}

// Bug: a decorated Python function omitted its decorators. The named function
// construct must include the leading decorators.
func TestFindBlocksDecoratedPythonIncludesDecorators(t *testing.T) {
	path := writeBlockFixture(t, "decorated.py", []byte(
		"@route('/x')\n"+
			"@login_required\n"+
			"def handler(request):\n"+
			"    return ok(request)\n"))

	blocks := findBlocksForTest(t, path, "handler")
	requireBlockWithExactSpan(t, blocks, 1, 4) // @route ... def handler ... return
}

// Bug: a regex that spans a line boundary could not match because the query
// was applied to each line separately. It must be applied to the whole file
// so a multiline pattern resolves to the surrounding structural block.
func TestFindBlocksMultilineRegexMatches(t *testing.T) {
	path := writeBlockFixture(t, "multiline.go", []byte("package main\n\nfunc f() {\n"+
		"\ttransform(\n"+
		"\t\tinput,\n"+
		"\t)\n"+
		"}\n"))

	blocks := findBlocksForTest(t, path, "transform\\(\\n\\s+input")
	requireBlockWithExactSpan(t, blocks, 4, 6) // the transform(...) call spanning the match
}

// Bug: ^ in a query matched only at the start of the file (byte 0), not at
// the start of each line, because the regex runs against the whole file
// contents. buildSearch enables multiline mode so ^ and $ anchor to every
// line boundary (grep-like); \A remains the whole-file anchor.
func TestBuildSearchLineAnchorMatchesEveryLine(t *testing.T) {
	path := writeBlockFixture(t, "anchors.go", []byte("package main\n\n"+
		"func foo() {}\n\n"+
		"func bar() {}\n"))

	hatSearch := buildSearchForTest(t, "^func \\w+", path)
	blocks, err := findBlocks(path, hatSearch.query)
	if err != nil {
		t.Fatalf("find blocks: %v", err)
	}
	blocks = blocksWithoutHashes(blocks)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %#v, want 2 (foo and bar, each at a line start)", blocks)
	}
	if blocks[0].LineStart() != 3 || blocks[1].LineStart() != 5 {
		t.Fatalf("line starts = %d, %d, want 3, 5", blocks[0].LineStart(), blocks[1].LineStart())
	}

	// \A must still anchor to the whole file, so \Afunc on a file whose
	// first line is "package main" finds nothing.
	aSearch := buildSearchForTest(t, "\\Afunc", path)
	none, err := findBlocks(path, aSearch.query)
	if err != nil {
		t.Fatalf("find blocks: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("\\Afunc blocks = %#v, want none (file starts with package)", none)
	}
}
