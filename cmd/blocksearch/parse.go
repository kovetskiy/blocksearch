package main

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/reconquest/karma-go"
	"github.com/reconquest/pkg/log"
	sitter "github.com/smacker/go-tree-sitter"
)

const contentTypeHeaderSize = 512

type blockLineRange struct {
	start int
	end   int
}

// fallbackBlocks extracts blocks using indentation heuristics when no
// tree-sitter grammar is available. A block is the matched line plus every
// subsequent line whose leading whitespace width exceeds the match line's.
// Blank/whitespace-only lines terminate the block.
func fallbackBlocks(contents []byte, query *regexp.Regexp) (Blocks, error) {
	normalized := []byte(hashlineNormalizeFile(contents))
	lines := strings.Split(string(normalized), "\n")
	hashes := hashlineHashes(contents)

	var blocks Blocks
	emitted := map[blockLineRange]struct{}{}

	for _, match := range query.FindAllIndex(normalized, -1) {
		matchLine := byteOffsetToLine(normalized, match[0])
		if matchLine >= len(lines) {
			continue
		}

		baseIndent := leadingWhitespaceWidth(lines[matchLine])

		endLine := matchLine
		for i := matchLine + 1; i < len(lines); i++ {
			if isBlankLine(lines[i]) {
				break
			}
			if leadingWhitespaceWidth(lines[i]) <= baseIndent {
				break
			}
			endLine = i
		}

		block := make(Block, 0, endLine-matchLine+1)
		for i := matchLine; i <= endLine; i++ {
			block = append(block, BlockLine{
				Line: i + 1,
				Hash: hashes[i],
				Text: lines[i],
			})
		}

		key := blockLineRange{block.LineStart(), block.LineEnd()}
		if _, ok := emitted[key]; ok {
			continue
		}
		emitted[key] = struct{}{}
		blocks = append(blocks, block)
	}

	return blocks, nil
}

func byteOffsetToLine(data []byte, offset int) int {
	line := 0
	for i := 0; i < offset && i < len(data); i++ {
		if data[i] == '\n' {
			line++
		}
	}
	return line
}

func leadingWhitespaceWidth(line string) int {
	for i, ch := range line {
		if ch != ' ' && ch != '\t' {
			return i
		}
	}
	return len(line)
}

func isBlankLine(line string) bool {
	for _, ch := range line {
		if ch != ' ' && ch != '\t' {
			return false
		}
	}
	return true
}

func findBlocks(filename string, query *regexp.Regexp) (Blocks, error) {
	contents, err := os.ReadFile(filename)
	if err != nil {
		return nil, karma.Format(err, "read file")
	}

	if !isTextContent(contents) {
		return nil, nil
	}

	language := detectLanguage(filename, contents)
	if language == nil {
		return fallbackBlocks(contents, query)
	}

	blocks, err := parseBlocks(contents, language, query)
	if err != nil {
		return nil, karma.Format(err, "parse blocks")
	}

	return blocks, nil
}

func isTextContent(contents []byte) bool {
	header := contents
	if len(header) > contentTypeHeaderSize {
		header = header[:contentTypeHeaderSize]
	}

	kind := http.DetectContentType(header)
	log.Debug("content type: " + kind)

	// Accept any text/* type. Code is text/plain, but HTML/XML are detected
	// as text/html and text/xml and must still be searchable.
	return strings.HasPrefix(kind, "text/")
}

func parseBlocks(
	contents []byte,
	language *sitter.Language,
	query *regexp.Regexp,
) (Blocks, error) {
	parser := sitter.NewParser()
	defer parser.Close()
	parser.SetLanguage(language)

	normalizedContents := []byte(hashlineNormalizeFile(contents))
	tree := parser.Parse(nil, normalizedContents)
	if tree == nil {
		return nil, fmt.Errorf("parse source")
	}
	defer tree.Close()

	lines := strings.Split(string(normalizedContents), "\n")
	hashes := hashlineHashes(contents)
	root := tree.RootNode()

	var blocks Blocks
	emitted := map[blockLineRange]struct{}{}
	for _, match := range query.FindAllIndex(normalizedContents, -1) {
		start := uint32(match[0])
		end := uint32(match[1])

		node := namedDescendantForByteRange(root, start, end)
		if node == nil || node.IsNull() {
			continue
		}

		node = blockNodeForMatch(node)
		block := blockFromNode(node, lines, hashes)
		if len(block) == 0 {
			continue
		}

		// A block is the range of lines its node spans, so the dedup
		// key is that line range, not the node's byte range: several
		// matches on one line resolve to distinct identifier nodes
		// with different byte ranges but identical line spans, and
		// emitting each would repeat the same block. Distinct nodes
		// that span different lines — such as a function name and a
		// separate statement in its body — keep different keys and
		// are both kept.
		key := blockLineRange{block.LineStart(), block.LineEnd()}
		if _, ok := emitted[key]; ok {
			continue
		}

		emitted[key] = struct{}{}
		blocks = append(blocks, block)
	}

	return blocks, nil
}

func namedDescendantForByteRange(
	node *sitter.Node,
	start uint32,
	end uint32,
) *sitter.Node {
	if node == nil || node.IsNull() || !nodeCoversByteRange(node, start, end) {
		return nil
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if descendant := namedDescendantForByteRange(child, start, end); descendant != nil {
			return descendant
		}
	}

	if !node.IsNamed() {
		return nil
	}

	return node
}

func nodeCoversByteRange(node *sitter.Node, start uint32, end uint32) bool {
	return node.StartByte() <= start && end <= node.EndByte()
}

// blockNodeForMatch resolves a matched node to the structural block it names or
// belongs to: the full function/class/statement, not just the matched token.
//
// Resolution proceeds in two stages, then a decorator step:
//  1. Name promotion: a token reached through a naming field ("name",
//     "function", "key") names the construct whose field it is. The token may
//     sit directly in the field or nested inside it (e.g. a YAML mapping
//     pair's key wraps a scalar), so we climb ancestors and resolve to the
//     nearest one whose naming-field child's subtree covers the match.
//  2. Statement promotion: a match that is not a name (an argument, an
//     if/loop condition, a return value, a declarator) belongs to the
//     smallest enclosing statement/declaration. We climb to the nearest
//     ancestor that is a direct child of a statement container (a block,
//     the translation unit, etc.) — that ancestor is the innermost complete
//     construct. This covers names declared through non-naming fields
//     (C declarators, OCaml patterns, Elixir call targets) and bare leaves
//     inside multiline statements.
//  3. Decorator promotion: if the resolved node is a function/type
//     definition wrapped by a decorated_definition, climb to the wrapper so
//     decorators are part of the block.
func blockNodeForMatch(node *sitter.Node) *sitter.Node {
	namingFields := []string{"name", "function", "key"}
	for ancestor := node.Parent(); ancestor != nil && !ancestor.IsNull(); ancestor = ancestor.Parent() {
		for _, field := range namingFields {
			child := ancestor.ChildByFieldName(field)
			if child != nil && nodeCoversNode(child, node) {
				return unwrapDecorated(ancestor)
			}
		}
	}

	// Statement promotion only applies to single-line leaves: a matched
	// token that sits inside a larger construct (an argument, an if/loop
	// condition, a declarator). A match that already resolves to a
	// multi-line node (e.g. an embedded raw-text blob) is itself the block.
	if node.StartPoint().Row != node.EndPoint().Row {
		return node
	}

	for cur := node; cur != nil && !cur.IsNull(); cur = cur.Parent() {
		parent := cur.Parent()
		if parent != nil && !parent.IsNull() && isStatementContainer(parent) {
			return unwrapDecorated(cur)
		}
	}

	return node
}

// unwrapDecorated climbs to the decorated_definition that wraps a
// function/type definition so a named construct includes its decorators.
func unwrapDecorated(node *sitter.Node) *sitter.Node {
	parent := node.Parent()
	if parent != nil && !parent.IsNull() && parent.Type() == "decorated_definition" {
		return parent
	}
	return node
}

// isStatementContainer reports whether a node holds a flat sequence of
// statements/declarations (a body, a block, or the root unit). The set is
// kept to the structural names tree-sitter grammars use for statement
// sequences; non-sequence containers like argument_list are deliberately
// excluded so a bare argument does not resolve to itself.
func isStatementContainer(node *sitter.Node) bool {
	switch node.Type() {
	case
		"program", "source", "module", "block",
		"stream", "document", "stylesheet", "chunk", "proto",
		"translation_unit", "compilation_unit", "source_file",
		"do_block", "statement_block", "compound_statement",
		"class_body", "module_body", "function_body",
		"enum_body", "interface_body", "struct_body", "impl_body",
		"namespace", "block_mapping", "block_sequence",
		"flow_mapping", "flow_sequence", "object", "array",
		"element", "list":
		return true
	}
	t := node.Type()
	switch {
	case strings.HasSuffix(t, "_block"),
		strings.HasSuffix(t, "_body"),
		strings.HasSuffix(t, "_unit"),
		strings.HasSuffix(t, "_mapping"),
		strings.HasSuffix(t, "_sequence"):
		return true
	}
	return false
}

func nodeCoversNode(outer, inner *sitter.Node) bool {
	return outer.StartByte() <= inner.StartByte() && inner.EndByte() <= outer.EndByte()
}
func blockFromNode(node *sitter.Node, lines []string, hashes []string) Block {
	startLine := int(node.StartPoint().Row)
	endLine := int(node.EndPoint().Row)
	// tree-sitter positions a node ending in a trailing newline at column 0 of
	// the next line, so drop that phantom line unless the node is single-line.
	if node.EndPoint().Column == 0 && endLine > startLine {
		endLine--
	}

	if startLine >= len(lines) {
		return nil
	}
	if endLine >= len(lines) {
		endLine = len(lines) - 1
	}

	block := make(Block, 0, endLine-startLine+1)
	for lineIndex := startLine; lineIndex <= endLine; lineIndex++ {
		block = append(block, BlockLine{
			Line: lineIndex + 1,
			Hash: hashes[lineIndex],
			Text: lines[lineIndex],
		})
	}

	return block
}
