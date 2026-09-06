package main

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestParseRejectsInvalidUTF8(t *testing.T) {
	for _, name := range []string{"source.py", "source.txt"} {
		for _, source := range []struct {
			name string
			text string
		}{
			{"reported", strings.Repeat("\xff", 4999) + "TARGET\n"},
			{"beyond header", "x = 'TARGET" + strings.Repeat("a", 1024) + "\xff'\n"},
			{"outside block", "x = 'TARGET'\n\n# \xff\n"},
			{"truncated", "x = 'TARGET'\n# \xe2\x82"},
			{"continuation", "x = '\x80TARGET'\n"},
			{"overlong", "x = '\xc0\xafTARGET'\n"},
			{"surrogate", "x = '\xed\xa0\x80TARGET'\n"},
			{"normalization", "\xef\xbb\xbf# \xe2\r\x82\xac\r\nx = 'TARGET'\n"},
		} {
			t.Run(name+"/"+source.name, func(t *testing.T) {
				contents := []byte(source.text)
				path := writeBlockFixture(t, name, contents)
				for _, pattern := range []string{"TARGET", "absent"} {
					for _, metadata := range []bool{false, true} {
						options := ParseOptions{Matches: metadata}
						query := regexp.MustCompile(pattern)
						blocks, err := findBlocksWithOptions(context.Background(), path, query, options)
						if err == nil || !strings.Contains(err.Error(), "invalid UTF-8") || blocks != nil {
							t.Fatalf("file query=%q metadata=%v: blocks=%#v error=%v", pattern, metadata, blocks, err)
						}
						blocks, err = parseBlocksWithOptions(context.Background(), contents, detectLanguage(name, contents), query, options)
						if err == nil || !strings.Contains(err.Error(), "invalid UTF-8") || blocks != nil {
							t.Fatalf("parse query=%q metadata=%v: blocks=%#v error=%v", pattern, metadata, blocks, err)
						}
					}
				}
			})
		}
	}
}

func TestParseUTF8ValidationPreservesInputPolicy(t *testing.T) {
	ctx := context.Background()
	query := regexp.MustCompile("TARGET")
	contents := []byte("TARGET\xff")
	options := ParseOptions{Limits: SearchLimits{FileBytes: 1}}
	path := writeBlockFixture(t, "source.txt", contents)
	_, err := findBlocksWithOptions(ctx, path, query, options)
	var limit *LimitError
	if !errors.As(err, &limit) || limit.Resource != "file_bytes" {
		t.Fatalf("file limit must precede encoding validation: %v", err)
	}
	_, err = parseBlocksWithOptions(ctx, contents, nil, query, options)
	if !errors.As(err, &limit) || limit.Resource != "file_bytes" {
		t.Fatalf("parse limit must precede encoding validation: %v", err)
	}
	path = writeBlockFixture(t, "binary.txt", []byte("\x00\xffTARGET"))
	blocks, err := findBlocksWithOptions(ctx, path, query, ParseOptions{Matches: true})
	if err != nil || blocks != nil {
		t.Fatalf("binary skip: blocks=%#v error=%v", blocks, err)
	}
}

func TestParseGenuineReplacementCharacterJSONCoordinates(t *testing.T) {
	for _, name := range []string{"source.py", "source.txt"} {
		for _, fixture := range []struct {
			name       string
			raw        string
			normalized string
			text       string
			pattern    string
			match      Match
		}{
			{
				name:       "normalized",
				raw:        "\ufeff# é\r\nx = '� TARGET'\r\n",
				normalized: "# é\nx = '� TARGET'\n",
				text:       "x = '� TARGET'",
				pattern:    "TARGET",
				match:      Match{ByteStart: 14, ByteEnd: 20, LineStart: 2, LineEnd: 2, ColumnStart: 10, ColumnEnd: 16},
			},
			{
				name:       "replacement match",
				raw:        "\ufeff# é\r\nx = '� TARGET'\r\n",
				normalized: "# é\nx = '� TARGET'\n",
				text:       "x = '� TARGET'",
				pattern:    "�",
				match:      Match{ByteStart: 10, ByteEnd: 13, LineStart: 2, LineEnd: 2, ColumnStart: 6, ColumnEnd: 9},
			},
			{
				name:       "long prefix",
				raw:        "x = '" + strings.Repeat("�", 4999) + "TARGET'\n",
				normalized: "x = '" + strings.Repeat("�", 4999) + "TARGET'\n",
				text:       "x = '" + strings.Repeat("�", 4999) + "TARGET'",
				pattern:    "TARGET",
				match:      Match{ByteStart: 15002, ByteEnd: 15008, LineStart: 1, LineEnd: 1, ColumnStart: 15003, ColumnEnd: 15009},
			},
		} {
			t.Run(name+"/"+fixture.name, func(t *testing.T) {
				blocks := parseOptionsFixture(t, name, fixture.raw, regexp.QuoteMeta(fixture.pattern), ParseOptions{Matches: true})
				if len(blocks) != 1 {
					t.Fatalf("blocks = %d, want 1", len(blocks))
				}
				encoded, err := blocks[0].EncodeJSON(name, false)
				if err != nil {
					t.Fatalf("encode: %v", err)
				}
				var record struct {
					Text      string  `json:"text"`
					LineStart int     `json:"line_start"`
					LineEnd   int     `json:"line_end"`
					Matches   []Match `json:"matches"`
				}
				if err := json.Unmarshal(encoded, &record); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if record.Text != fixture.text || record.LineStart != fixture.match.LineStart || record.LineEnd != fixture.match.LineEnd || !reflect.DeepEqual(record.Matches, []Match{fixture.match}) {
					t.Fatalf("JSON record disagrees with source coordinates: %+v", record)
				}
				match := record.Matches[0]
				if fixture.normalized[match.ByteStart:match.ByteEnd] != fixture.pattern || record.Text[match.ColumnStart-1:match.ColumnEnd-1] != fixture.pattern {
					t.Fatalf("file offsets and JSON byte columns disagree: %+v", match)
				}
			})
		}
	}
}
