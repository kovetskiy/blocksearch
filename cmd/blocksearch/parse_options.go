package main

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

type ParseOptions struct {
	Matches bool
	Overlap string
	Limits  SearchLimits
}

func findBlocksWithOptions(ctx context.Context, filename string, query *regexp.Regexp, options ParseOptions) (Blocks, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := openSearchInput(ctx, filename)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	defer file.Close()
	stopClosing := context.AfterFunc(ctx, func() { _ = file.Close() })
	defer stopClosing()

	contents, err := readParseInput(ctx, file, options.Limits.FileBytes)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	if !isTextContent(contents) {
		return nil, nil
	}
	return parseBlocksWithOptions(ctx, contents, detectLanguage(filename, contents), query, options)
}

type parseInputReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader parseInputReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}

func readParseInput(ctx context.Context, reader io.Reader, maximum int64) ([]byte, error) {
	reader = parseInputReader{ctx: ctx, reader: reader}
	if maximum > 0 && maximum < int64(^uint64(0)>>1) {
		reader = io.LimitReader(reader, maximum+1)
	}
	contents, err := io.ReadAll(reader)
	if canceled := ctx.Err(); canceled != nil {
		return nil, canceled
	}
	if err != nil {
		return nil, err
	}
	if maximum > 0 && int64(len(contents)) > maximum {
		return nil, &LimitError{Resource: "file_bytes", Maximum: maximum}
	}
	return contents, nil
}

func parseBlocksWithOptions(ctx context.Context, contents []byte, language *sitter.Language, query *regexp.Regexp, options ParseOptions) (Blocks, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch options.Overlap {
	case "", "all", "outermost", "innermost":
	default:
		return nil, fmt.Errorf("invalid overlap policy: %q", options.Overlap)
	}
	if maximum := options.Limits.FileBytes; maximum > 0 && int64(len(contents)) > maximum {
		return nil, &LimitError{Resource: "file_bytes", Maximum: maximum}
	}
	normalized := hashlineNormalizeFileBytes(contents)
	matchLimit := -1
	if maximum := options.Limits.Matches; maximum > 0 && maximum < int(^uint(0)>>1) {
		matchLimit = maximum + 1
	}
	matches := query.FindAllIndex(normalized, matchLimit)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if maximum := options.Limits.Matches; maximum > 0 && len(matches) > maximum {
		return nil, &LimitError{Resource: "matches", Maximum: int64(maximum)}
	}
	if len(matches) == 0 {
		return nil, nil
	}

	var root *sitter.Node
	if language != nil {
		if uint64(len(normalized)) > uint64(^uint32(0)) {
			return nil, &LimitError{Resource: "file_bytes", Maximum: int64(^uint32(0))}
		}
		parser := sitter.NewParser()
		defer parser.Close()
		parser.SetLanguage(language)
		tree, err := parser.ParseCtx(ctx, nil, normalized)
		if err != nil {
			return nil, fmt.Errorf("parse source: %w", err)
		}
		if tree == nil {
			return nil, fmt.Errorf("parse source: no tree")
		}
		defer tree.Close()
		root = tree.RootNode()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	source := newParseSource(normalized)
	candidates := blockCandidates{source: source, options: options, seen: make(map[blockLineRange]int)}
	for _, match := range matches {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var span blockLineRange
		if root == nil {
			var err error
			span, err = source.fallbackRange(ctx, match)
			if err != nil {
				return nil, err
			}
		} else {
			node := namedDescendantForByteRange(root, uint32(match[0]), uint32(match[1]))
			if node == nil || node.IsNull() {
				continue
			}
			var ok bool
			span, ok = blockRangeFromNode(blockNodeForMatch(node), len(source.lines))
			if !ok {
				continue
			}
		}
		if err := candidates.add(span, match); err != nil {
			return nil, err
		}
	}
	return candidates.blocks(ctx)
}

type parseSource struct {
	lines  []string
	starts []int
}

func newParseSource(contents []byte) parseSource {
	lines := strings.Split(string(contents), "\n")
	starts := make([]int, len(lines))
	for i := 1; i < len(lines); i++ {
		starts[i] = starts[i-1] + len(lines[i-1]) + 1
	}
	return parseSource{lines: lines, starts: starts}
}

func (source parseSource) position(offset int) (line, column int) {
	index := sort.Search(len(source.starts), func(i int) bool { return source.starts[i] > offset }) - 1
	return index + 1, offset - source.starts[index] + 1
}

func (source parseSource) match(span []int) Match {
	startLine, startColumn := source.position(span[0])
	endLine, endColumn := source.position(span[1])
	return Match{
		ByteStart: span[0], ByteEnd: span[1],
		LineStart: startLine, LineEnd: endLine,
		ColumnStart: startColumn, ColumnEnd: endColumn,
	}
}

func (source parseSource) fallbackRange(ctx context.Context, match []int) (blockLineRange, error) {
	start, _ := source.position(match[0])
	end := start
	baseIndent := leadingWhitespaceWidth(source.lines[start-1])
	for i := start; i < len(source.lines); i++ {
		if err := ctx.Err(); err != nil {
			return blockLineRange{}, err
		}
		if isBlankLine(source.lines[i]) || leadingWhitespaceWidth(source.lines[i]) <= baseIndent {
			break
		}
		end = i + 1
	}
	return blockLineRange{start, end}, nil
}

type blockCandidate struct {
	span    blockLineRange
	matches []Match
}

type blockCandidates struct {
	source  parseSource
	options ParseOptions
	seen    map[blockLineRange]int
	ordered []blockCandidate
}

func (candidates *blockCandidates) add(span blockLineRange, match []int) error {
	index, exists := candidates.seen[span]
	if !exists {
		limits := candidates.options.Limits
		if limits.Blocks > 0 && len(candidates.ordered) >= limits.Blocks {
			return &LimitError{Resource: "blocks", Maximum: int64(limits.Blocks)}
		}
		if limits.BlockLines > 0 && span.end-span.start+1 > limits.BlockLines {
			return &LimitError{Resource: "block_lines", Maximum: int64(limits.BlockLines)}
		}
		bytes := candidates.source.starts[span.end-1] + len(candidates.source.lines[span.end-1]) - candidates.source.starts[span.start-1]
		if limits.BlockBytes > 0 && int64(bytes) > limits.BlockBytes {
			return &LimitError{Resource: "block_bytes", Maximum: limits.BlockBytes}
		}
		index = len(candidates.ordered)
		candidates.seen[span] = index
		candidates.ordered = append(candidates.ordered, blockCandidate{span: span})
	}
	if candidates.options.Matches {
		candidates.ordered[index].matches = append(candidates.ordered[index].matches, candidates.source.match(match))
	}
	return nil
}

func (candidates *blockCandidates) blocks(ctx context.Context) (Blocks, error) {
	retained, err := retainBlockRanges(ctx, candidates.ordered, candidates.options.Overlap)
	if err != nil {
		return nil, err
	}
	hashes := newLazyHashlineHashes(candidates.source.lines)
	var blocks Blocks
	for i, candidate := range candidates.ordered {
		if !retained[i] {
			continue
		}
		span := candidate.span
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		block := make(Block, span.end-span.start+1)
		for line := span.start; line <= span.end; line++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			block[line-span.start] = BlockLine{Line: line, Hash: hashes.Hash(line - 1), Text: candidates.source.lines[line-1]}
		}
		block[0].Matches = candidate.matches
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func retainBlockRanges(ctx context.Context, candidates []blockCandidate, policy string) ([]bool, error) {
	retained := make([]bool, len(candidates))
	if policy == "" || policy == "all" {
		for i := range retained {
			retained[i] = true
		}
		return retained, ctx.Err()
	}
	order := make([]int, len(candidates))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool {
		left, right := candidates[order[i]].span, candidates[order[j]].span
		if left.start == right.start {
			return left.end > right.end
		}
		return left.start < right.start
	})
	if policy == "outermost" {
		endMax := 0
		for _, index := range order {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			end := candidates[index].span.end
			retained[index] = end > endMax
			if end > endMax {
				endMax = end
			}
		}
	} else {
		endMin := int(^uint(0) >> 1)
		for i := len(order) - 1; i >= 0; i-- {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			index := order[i]
			end := candidates[index].span.end
			retained[index] = end < endMin
			if end < endMin {
				endMin = end
			}
		}
	}
	return retained, nil
}
