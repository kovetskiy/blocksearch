package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

func parseOptionsFixture(t *testing.T, name, contents, pattern string, options ParseOptions) Blocks {
	t.Helper()
	path := writeBlockFixture(t, name, []byte(contents))
	blocks, err := findBlocksWithOptions(context.Background(), path, regexp.MustCompile(pattern), options)
	if err != nil {
		t.Fatalf("find blocks: %v", err)
	}
	return blocks
}

func TestParseMatchMetadataNormalizationAndByteColumns(t *testing.T) {
	for _, name := range []string{"source.py", "source.txt"} {
		t.Run(name, func(t *testing.T) {
			blocks := parseOptionsFixture(t, name, "\ufeffx = 'é hit hit'\r\ny = 'hit'\r", "hit", ParseOptions{Matches: true})
			want := [][]Match{
				{
					{ByteStart: 8, ByteEnd: 11, LineStart: 1, LineEnd: 1, ColumnStart: 9, ColumnEnd: 12},
					{ByteStart: 12, ByteEnd: 15, LineStart: 1, LineEnd: 1, ColumnStart: 13, ColumnEnd: 16},
				},
				{{ByteStart: 22, ByteEnd: 25, LineStart: 2, LineEnd: 2, ColumnStart: 6, ColumnEnd: 9}},
			}
			if len(blocks) != len(want) {
				t.Fatalf("blocks = %#v, want two", blocks)
			}
			for i, block := range blocks {
				if !reflect.DeepEqual(block[0].Matches, want[i]) {
					t.Errorf("block %d matches = %#v, want %#v", i, block[0].Matches, want[i])
				}
			}
		})
	}
}

func TestParseLiteralMatchMetadata(t *testing.T) {
	for _, name := range []string{"source.py", "source.txt"} {
		blocks := parseOptionsFixture(t, name, "x = 'a.b axb a.b'\n", regexp.QuoteMeta("a.b"), ParseOptions{Matches: true})
		want := []Match{
			{ByteStart: 5, ByteEnd: 8, LineStart: 1, LineEnd: 1, ColumnStart: 6, ColumnEnd: 9},
			{ByteStart: 13, ByteEnd: 16, LineStart: 1, LineEnd: 1, ColumnStart: 14, ColumnEnd: 17},
		}
		if len(blocks) != 1 || !reflect.DeepEqual(blocks[0][0].Matches, want) {
			t.Fatalf("%s blocks = %#v, want matches %#v", name, blocks, want)
		}
	}
}

func TestParseMultilineMetadata(t *testing.T) {
	for _, name := range []string{"source.py", "source.txt"} {
		blocks := parseOptionsFixture(t, name, "x = 'alpha'\ny = 'beta'\nz = 0\n", "alpha'[\\s\\S]*beta", ParseOptions{Matches: true})
		want := []Match{{ByteStart: 5, ByteEnd: 21, LineStart: 1, LineEnd: 2, ColumnStart: 6, ColumnEnd: 10}}
		if len(blocks) != 1 || !reflect.DeepEqual(blocks[0][0].Matches, want) {
			t.Fatalf("%s blocks = %#v, want matches %#v", name, blocks, want)
		}
		if blocks[0].LineStart() != 1 || (name == "source.py" && blocks[0].LineEnd() < 2) {
			t.Fatalf("%s multiline match range: %#v", name, blocks)
		}
		if name == "source.txt" && blocks[0].LineEnd() != 1 {
			t.Fatalf("fallback range changed: %#v", blocks)
		}
		for _, line := range blocks[0][1:] {
			if line.Matches != nil {
				t.Errorf("metadata populated beyond first line: %#v", line)
			}
		}
	}
}

func TestFallbackMetadataExclusiveNewlineAndEOF(t *testing.T) {
	for _, test := range []struct {
		text    string
		pattern string
		want    []Match
	}{
		{"é\nx\n", "(?m)^", []Match{
			{ByteStart: 0, ByteEnd: 0, LineStart: 1, LineEnd: 1, ColumnStart: 1, ColumnEnd: 1},
			{ByteStart: 3, ByteEnd: 3, LineStart: 2, LineEnd: 2, ColumnStart: 1, ColumnEnd: 1},
			{ByteStart: 5, ByteEnd: 5, LineStart: 3, LineEnd: 3, ColumnStart: 1, ColumnEnd: 1},
		}},
		{"é", "$", []Match{{ByteStart: 2, ByteEnd: 2, LineStart: 1, LineEnd: 1, ColumnStart: 3, ColumnEnd: 3}}},
		{"é\nx\n", "é\\nx\\n", []Match{{ByteStart: 0, ByteEnd: 5, LineStart: 1, LineEnd: 3, ColumnStart: 1, ColumnEnd: 1}}},
		{"", "^", []Match{{ByteStart: 0, ByteEnd: 0, LineStart: 1, LineEnd: 1, ColumnStart: 1, ColumnEnd: 1}}},
	} {
		blocks, err := parseBlocksWithOptions(context.Background(), []byte(test.text), nil, regexp.MustCompile(test.pattern), ParseOptions{Matches: true})
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		var got []Match
		for _, block := range blocks {
			got = append(got, block[0].Matches...)
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%q / %q matches = %#v, want %#v", test.text, test.pattern, got, test.want)
		}
		if test.pattern == "é\\nx\\n" && blocks[0].LineEnd() != 1 {
			t.Errorf("fallback first-line range changed: %#v", blocks)
		}
	}
}

func TestParsedZeroWidthEOFMetadata(t *testing.T) {
	for _, text := range []string{"x = 1", "x = 1\n"} {
		blocks := parseOptionsFixture(t, "source.py", text, "\\z", ParseOptions{Matches: true})
		line, column := 1, 6
		if strings.HasSuffix(text, "\n") {
			line, column = 2, 1
		}
		want := []Match{{ByteStart: len(text), ByteEnd: len(text), LineStart: line, LineEnd: line, ColumnStart: column, ColumnEnd: column}}
		if len(blocks) != 1 || !reflect.DeepEqual(blocks[0][0].Matches, want) {
			t.Fatalf("%q blocks = %#v, want matches %#v", text, blocks, want)
		}
	}
}

func TestParseJSONMetadataOptIn(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		blocks := parseOptionsFixture(t, "source.py", "x = 'hit hit'\n", "hit", ParseOptions{Matches: enabled})
		encoded, err := blocks[0].EncodeJSON("source.py", false)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &object); err != nil {
			t.Fatalf("decode: %v", err)
		}
		_, present := object["matches"]
		if present != enabled {
			t.Errorf("matches present = %v, enabled = %v: %s", present, enabled, encoded)
		}
		if !enabled && (len(object) != 4 || blocks[0][0].Matches != nil) {
			t.Errorf("default schema or storage changed: %s", encoded)
		}
		if enabled {
			var matches []map[string]int
			if err := json.Unmarshal(object["matches"], &matches); err != nil {
				t.Fatalf("decode matches: %v", err)
			}
			if len(matches) != 2 || len(matches[0]) != 6 || matches[0]["byte_start"] != 5 || matches[1]["column_end"] != 13 {
				t.Errorf("match schema = %s", object["matches"])
			}
		}
	}
}

func TestParseOverlapCausalMatches(t *testing.T) {
	text := "def target():\n    target = target + 1\n    return target\n"
	for _, test := range []struct {
		policy string
		ranges []blockLineRange
		counts []int
	}{
		{"", []blockLineRange{{1, 3}, {2, 2}, {3, 3}}, []int{1, 2, 1}},
		{"all", []blockLineRange{{1, 3}, {2, 2}, {3, 3}}, []int{1, 2, 1}},
		{"outermost", []blockLineRange{{1, 3}}, []int{1}},
		{"innermost", []blockLineRange{{2, 2}, {3, 3}}, []int{2, 1}},
	} {
		blocks := parseOptionsFixture(t, "source.py", text, "target", ParseOptions{Matches: true, Overlap: test.policy})
		var ranges []blockLineRange
		var counts []int
		for _, block := range blocks {
			ranges = append(ranges, blockLineRange{block.LineStart(), block.LineEnd()})
			counts = append(counts, len(block[0].Matches))
			for _, match := range block[0].Matches {
				if text[match.ByteStart:match.ByteEnd] != "target" {
					t.Errorf("invalid causal match: %#v", match)
				}
			}
		}
		if !reflect.DeepEqual(ranges, test.ranges) || !reflect.DeepEqual(counts, test.counts) {
			t.Errorf("%s ranges/counts = %v/%v, want %v/%v", test.policy, ranges, counts, test.ranges, test.counts)
		}
	}
}

func TestParseOverlapCrossConstruct(t *testing.T) {
	text := "def one():\n    alpha = 1\n\ndef two():\n    beta = 2\n"
	for _, test := range []struct {
		policy string
		want   []blockLineRange
	}{
		{"all", []blockLineRange{{1, 2}, {1, 5}}},
		{"outermost", []blockLineRange{{1, 5}}},
		{"innermost", []blockLineRange{{1, 2}}},
	} {
		blocks := parseOptionsFixture(t, "source.py", text, "one|alpha[\\s\\S]*beta", ParseOptions{Matches: true, Overlap: test.policy})
		var got []blockLineRange
		for _, block := range blocks {
			got = append(got, blockLineRange{block.LineStart(), block.LineEnd()})
			if len(block[0].Matches) != 1 {
				t.Errorf("suppressed matches transferred: %#v", block[0].Matches)
			}
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s ranges = %v, want %v", test.policy, got, test.want)
		}
	}
}

func TestOverlapRangesPartialEqualStable(t *testing.T) {
	spans := []blockLineRange{{4, 7}, {2, 5}, {2, 5}, {3, 3}, {2, 3}, {6, 7}, {9, 9}}
	for _, test := range []struct {
		policy string
		want   []blockLineRange
	}{
		{"all", []blockLineRange{{4, 7}, {2, 5}, {3, 3}, {2, 3}, {6, 7}, {9, 9}}},
		{"outermost", []blockLineRange{{4, 7}, {2, 5}, {9, 9}}},
		{"innermost", []blockLineRange{{3, 3}, {6, 7}, {9, 9}}},
	} {
		candidates := blockCandidates{source: newParseSource([]byte(strings.Repeat("x\n", 10))), options: ParseOptions{Matches: true, Overlap: test.policy}, seen: make(map[blockLineRange]int)}
		for _, span := range spans {
			if err := candidates.add(span, []int{0, 1}); err != nil {
				t.Fatalf("add: %v", err)
			}
		}
		blocks, err := candidates.blocks(context.Background())
		if err != nil {
			t.Fatalf("blocks: %v", err)
		}
		var got []blockLineRange
		for _, block := range blocks {
			got = append(got, blockLineRange{block.LineStart(), block.LineEnd()})
			wantCount := 1
			if block.LineStart() == 2 && block.LineEnd() == 5 {
				wantCount = 2
			}
			if len(block[0].Matches) != wantCount {
				t.Errorf("dedup matches = %#v, want count %d", block[0].Matches, wantCount)
			}
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s ranges = %v, want %v", test.policy, got, test.want)
		}
	}
}

func TestParsingLimits(t *testing.T) {
	text := "def hit():\n    hit = 'hit'\n    return hit\n"
	for _, name := range []string{"source.py", "source.txt"} {
		for _, test := range []struct {
			resource string
			maximum  int64
			limits   SearchLimits
		}{
			{"file_bytes", int64(len(text) - 1), SearchLimits{FileBytes: int64(len(text) - 1)}},
			{"matches", 3, SearchLimits{Matches: 3}},
			{"blocks", 2, SearchLimits{Blocks: 2}},
			{"block_lines", 2, SearchLimits{BlockLines: 2}},
			{"block_bytes", int64(len(text) - 2), SearchLimits{BlockBytes: int64(len(text) - 2)}},
		} {
			t.Run(name+"/"+test.resource, func(t *testing.T) {
				path := writeBlockFixture(t, name, []byte(text))
				blocks, err := findBlocksWithOptions(context.Background(), path, regexp.MustCompile("hit"), ParseOptions{Matches: true, Overlap: "outermost", Limits: test.limits})
				var limit *LimitError
				if !errors.As(err, &limit) || limit.Resource != test.resource || limit.Maximum != test.maximum {
					t.Fatalf("error = %v, want %s maximum %d", err, test.resource, test.maximum)
				}
				if blocks != nil {
					t.Errorf("limit returned partial blocks: %#v", blocks)
				}
			})
		}
		blocks := parseOptionsFixture(t, name, text, "hit", ParseOptions{Matches: true, Limits: SearchLimits{
			FileBytes: int64(len(text)), Matches: 4, Blocks: 3, BlockLines: 3, BlockBytes: int64(len(text) - 1),
		}})
		if len(blocks) != 3 {
			t.Errorf("exact limits: got %d blocks, want 3", len(blocks))
		}
	}
}

func TestParsingLimitsUseRawInputAndNormalizedText(t *testing.T) {
	text := "\ufeffé hit\r\n"
	path := writeBlockFixture(t, "source.txt", []byte(text))
	blocks, err := findBlocksWithOptions(context.Background(), path, regexp.MustCompile("hit"), ParseOptions{Limits: SearchLimits{FileBytes: int64(len(text)), BlockBytes: 6, BlockLines: 1, Matches: 1, Blocks: 1}})
	if err != nil || len(blocks) != 1 {
		t.Fatalf("normalized byte bound: blocks = %#v, error = %v", blocks, err)
	}
	_, err = findBlocksWithOptions(context.Background(), path, regexp.MustCompile("absent"), ParseOptions{Limits: SearchLimits{FileBytes: 7}})
	var limit *LimitError
	if !errors.As(err, &limit) || limit.Resource != "file_bytes" {
		t.Fatalf("raw input limit before matching: %v", err)
	}
	_, err = parseBlocksWithOptions(context.Background(), []byte(text), nil, regexp.MustCompile("hit"), ParseOptions{Limits: SearchLimits{BlockBytes: 5}})
	if !errors.As(err, &limit) || limit.Resource != "block_bytes" {
		t.Fatalf("UTF-8 bytes rather than runes: %v", err)
	}
}

func TestReadParseInputUsesLimitPlusOne(t *testing.T) {
	reader := bytes.NewReader(bytes.Repeat([]byte("x"), 1000))
	_, err := readParseInput(context.Background(), reader, 10)
	var limit *LimitError
	if !errors.As(err, &limit) || limit.Resource != "file_bytes" || reader.Len() != 989 {
		t.Fatalf("read: err = %v, unread = %d, want file limit and 989", err, reader.Len())
	}
	for _, maximum := range []int64{0, 10, int64(^uint64(0) >> 1)} {
		got, err := readParseInput(context.Background(), strings.NewReader("0123456789"), maximum)
		if err != nil || string(got) != "0123456789" {
			t.Errorf("maximum %d: %q, %v", maximum, got, err)
		}
	}
}

type cancelParseReader struct{ cancel context.CancelFunc }

func (reader cancelParseReader) Read(buffer []byte) (int, error) {
	reader.cancel()
	copy(buffer, "x")
	return 1, nil
}

func TestParsingCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := findBlocksWithOptions(ctx, "missing", regexp.MustCompile("hit"), ParseOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("pre-canceled file read: %v", err)
	}
	_, err = parseBlocksWithOptions(ctx, []byte("hit"), nil, regexp.MustCompile("hit"), ParseOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("pre-canceled parse: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	_, err = readParseInput(ctx, cancelParseReader{cancel}, 0)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("canceled read: %v", err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	text := []byte(strings.Repeat("x = 1\n", 200000))
	_, err = parseBlocksWithOptions(ctx, text, detectLanguage("source.py", text), regexp.MustCompile("\\A"), ParseOptions{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("canceled source parse: %v", err)
	}
}

func TestParsingNoMatchesAndInvalidOverlap(t *testing.T) {
	blocks, err := parseBlocksWithOptions(context.Background(), []byte("plain"), nil, regexp.MustCompile("absent"), ParseOptions{Limits: SearchLimits{BlockBytes: 1}})
	if err != nil || len(blocks) != 0 {
		t.Fatalf("no matches should not parse or bound blocks: %#v, %v", blocks, err)
	}
	_, err = parseBlocksWithOptions(context.Background(), nil, nil, regexp.MustCompile("hit"), ParseOptions{Overlap: "invalid"})
	if err == nil {
		t.Fatalf("invalid overlap policy accepted")
	}
	_, err = readParseInput(context.Background(), io.MultiReader(strings.NewReader("hit"), errorParseReader{}), 0)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("read error was swallowed: %v", err)
	}
}

type errorParseReader struct{}

func (errorParseReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
