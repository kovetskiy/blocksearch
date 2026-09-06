package main

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"testing"
)

func TestParseUnicodeMatchSpans(t *testing.T) {
	for _, name := range []string{"source.py", "source.txt"} {
		blocks := parseOptionsFixture(t, name, "x = 'é界é'\r\n", "é|界", ParseOptions{Matches: true})
		want := []Match{
			{ByteStart: 5, ByteEnd: 7, LineStart: 1, LineEnd: 1, ColumnStart: 6, ColumnEnd: 8},
			{ByteStart: 7, ByteEnd: 10, LineStart: 1, LineEnd: 1, ColumnStart: 8, ColumnEnd: 11},
			{ByteStart: 10, ByteEnd: 12, LineStart: 1, LineEnd: 1, ColumnStart: 11, ColumnEnd: 13},
		}
		if len(blocks) != 1 || !reflect.DeepEqual(blocks[0][0].Matches, want) {
			t.Fatalf("%s blocks = %#v, want matches %#v", name, blocks, want)
		}
	}
}

func TestParseZeroWidthMatchesDeduplicateAndCountNormalizedInput(t *testing.T) {
	text := []byte("\ufeffé\r\n")
	query := regexp.MustCompile("")
	blocks, err := parseBlocksWithOptions(context.Background(), text, nil, query, ParseOptions{Matches: true, Limits: SearchLimits{Matches: 3, Blocks: 2}})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []Match{
		{ByteStart: 0, ByteEnd: 0, LineStart: 1, LineEnd: 1, ColumnStart: 1, ColumnEnd: 1},
		{ByteStart: 2, ByteEnd: 2, LineStart: 1, LineEnd: 1, ColumnStart: 3, ColumnEnd: 3},
	}
	if len(blocks) != 2 || !reflect.DeepEqual(blocks[0][0].Matches, want) || len(blocks[1][0].Matches) != 1 || blocks[1][0].Matches[0].ByteStart != 3 {
		t.Fatalf("normalized zero-width matches = %#v", blocks)
	}
	_, err = parseBlocksWithOptions(context.Background(), text, nil, query, ParseOptions{Limits: SearchLimits{Matches: 2}})
	var limit *LimitError
	if !errors.As(err, &limit) || limit.Resource != "matches" || limit.Maximum != 2 {
		t.Fatalf("zero-width match limit = %v", err)
	}
}

func TestFallbackMultilinePreservesAnchoredRanges(t *testing.T) {
	text := "a\n  b\n  c\nd\n"
	for _, testcase := range []struct {
		policy string
		ranges []blockLineRange
		starts []int
	}{
		{"all", []blockLineRange{{1, 3}, {3, 3}}, []int{0, 8}},
		{"outermost", []blockLineRange{{1, 3}}, []int{0}},
		{"innermost", []blockLineRange{{3, 3}}, []int{8}},
	} {
		for _, metadata := range []bool{false, true} {
			blocks := parseOptionsFixture(t, "source.txt", text, "a\\n  b|c\\nd", ParseOptions{Matches: metadata, Overlap: testcase.policy})
			var ranges []blockLineRange
			for index, block := range blocks {
				ranges = append(ranges, blockLineRange{block.LineStart(), block.LineEnd()})
				if metadata && (len(block[0].Matches) != 1 || index >= len(testcase.starts) || block[0].Matches[0].ByteStart != testcase.starts[index]) {
					t.Errorf("%s causal matches = %#v", testcase.policy, block)
				}
			}
			if !reflect.DeepEqual(ranges, testcase.ranges) {
				t.Fatalf("%s metadata=%v ranges = %v, want %v", testcase.policy, metadata, ranges, testcase.ranges)
			}
		}
	}
}

func TestOverlapSweepAgreesWithContainment(t *testing.T) {
	spans := []blockLineRange{{1, 1}, {1, 2}, {1, 3}, {2, 2}, {2, 3}, {2, 4}, {3, 4}, {4, 4}}
	for mask := 0; mask < 1<<len(spans); mask++ {
		var candidates []blockCandidate
		for i, span := range spans {
			if mask&(1<<i) != 0 {
				candidates = append(candidates, blockCandidate{span: span})
			}
		}
		for _, policy := range []string{"outermost", "innermost"} {
			got, err := retainBlockRanges(context.Background(), candidates, policy)
			if err != nil {
				t.Fatalf("retain: %v", err)
			}
			for i, candidate := range candidates {
				want := true
				for j, other := range candidates {
					if i == j {
						continue
					}
					outer, inner := other.span, candidate.span
					if policy == "innermost" {
						outer, inner = inner, outer
					}
					if outer.start <= inner.start && inner.end <= outer.end {
						want = false
					}
				}
				if got[i] != want {
					t.Fatalf("%s mask %d index %d: retained = %v, want %v", policy, mask, i, got[i], want)
				}
			}
		}
	}
}
