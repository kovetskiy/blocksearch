package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type recordWriter struct {
	records   [][]byte
	failAfter int
	err       error
}

func (w *recordWriter) Write(record []byte) (int, error) {
	if w.err != nil && len(w.records) >= w.failAfter {
		return 0, w.err
	}
	w.records = append(w.records, append([]byte(nil), record...))
	return len(record), nil
}

func emissionBlocks(texts ...string) Blocks {
	var blocks Blocks
	for i, text := range texts {
		blocks = append(blocks, Block{{Line: i + 1, Text: text}})
	}
	return blocks
}

func TestEmissionJSONWritesOneRecordAtATime(t *testing.T) {
	writer := &recordWriter{}
	emitter := newBlockEmitter(OutputPolicy{JSON: true, Hashline: true})
	emitter.writer = writer
	blocks := emissionBlocks("first", "second", "third")
	if err := emitter.emit(blocks, "a\nb.go"); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if len(writer.records) != len(blocks) {
		t.Fatalf("writes = %d, want %d", len(writer.records), len(blocks))
	}
	for i, record := range writer.records {
		expected, err := blocks[i].EncodeJSON("a\nb.go", true)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		expected = append(expected, '\n')
		if !bytes.Equal(record, expected) {
			t.Errorf("record %d = %q, want %q", i, record, expected)
		}
	}
}

func TestEmissionOutputBudgetStopsBeforeNextRecord(t *testing.T) {
	block := emissionBlocks("hit")
	encoded, err := block[0].EncodeJSON("same", false)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	maximum := int64(len(encoded) + 1)
	writer := &recordWriter{}
	emitter := newBlockEmitter(OutputPolicy{JSON: true, OutputBytes: maximum})
	emitter.writer = writer
	if err := emitter.emit(block, "same"); err != nil {
		t.Fatalf("first emit: %v", err)
	}
	err = emitter.emit(block, "same")
	var limit *LimitError
	if !errors.As(err, &limit) || limit.Resource != "output_bytes" || limit.Maximum != maximum {
		t.Fatalf("second emit: %v, want output_bytes limit %d", err, maximum)
	}
	if len(writer.records) != 1 || emitter.outputBytes != maximum || emitter.emittedBlockCount != 1 {
		t.Fatalf("writes/count/bytes = %d/%d/%d", len(writer.records), emitter.emittedBlockCount, emitter.outputBytes)
	}
}

func TestEmissionFailureStopsWithinFile(t *testing.T) {
	failure := errors.New("sink failed")
	writer := &recordWriter{err: failure, failAfter: 1}
	emitter := newBlockEmitter(OutputPolicy{JSON: true})
	emitter.writer = writer
	results := make(chan searchResult, 2)
	results <- searchResult{seq: 0, blocks: emissionBlocks("one", "two", "three")}
	results <- searchResult{seq: 1, blocks: emissionBlocks("four")}
	close(results)
	outcome := emitOrdered(results, emitter)
	if !errors.Is(outcome.err, failure) || outcome.emittedBlockCount != 1 || len(writer.records) != 1 {
		t.Fatalf("outcome = %+v, writes = %d", outcome, len(writer.records))
	}
}

func TestEmissionOrderedAndOrdinaryErrorsContinue(t *testing.T) {
	results := make(chan searchResult, 4)
	results <- searchResult{seq: 3, path: "last", blocks: emissionBlocks("last")}
	results <- searchResult{seq: 1, path: "missing", err: os.ErrNotExist}
	results <- searchResult{seq: 2, path: "middle", blocks: emissionBlocks("middle")}
	results <- searchResult{seq: 0, path: "first", blocks: emissionBlocks("first")}
	close(results)
	var output bytes.Buffer
	emitter := newBlockEmitter(OutputPolicy{FilesOnly: true})
	emitter.writer = &output
	outcome := emitOrdered(results, emitter)
	var pathErr *SearchPathError
	if outcome.emittedBlockCount != 3 || !errors.As(outcome.err, &pathErr) || pathErr.Path != "missing" || !errors.Is(outcome.err, os.ErrNotExist) {
		t.Fatalf("outcome = %+v", outcome)
	}
	if output.String() != "first\nmiddle\nlast\n" {
		t.Fatalf("output = %q", output.String())
	}
}

func TestEmissionLimitAheadOfSlowFileIsFatal(t *testing.T) {
	results := make(chan searchResult, 1)
	results <- searchResult{seq: 1, path: "limited", err: &LimitError{Resource: "matches", Maximum: 1}}
	outcome := emitOrdered(results, newBlockEmitter(OutputPolicy{JSON: true}))
	var limit *LimitError
	if !errors.As(outcome.err, &limit) || outcome.emittedBlockCount != 0 {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestEmissionSafeFilenames(t *testing.T) {
	paths := []string{"a\nb.go", "a\tb.go", "a\\b.go", "café.go"}
	for _, null := range []bool{false, true} {
		t.Run(fmt.Sprintf("null=%v", null), func(t *testing.T) {
			names := append([]string(nil), paths...)
			if null {
				names = append(names, "invalid-\xff.go")
			}
			var output bytes.Buffer
			emitter := newBlockEmitter(OutputPolicy{FilesOnly: true, JSON: !null, Null: null})
			emitter.writer = &output
			for _, path := range names {
				if err := emitter.emit(emissionBlocks("hit", "hit"), path); err != nil {
					t.Fatalf("emit: %v", err)
				}
			}
			var decoded []string
			if null {
				decoded = strings.Split(strings.TrimSuffix(output.String(), "\x00"), "\x00")
			} else {
				for _, record := range bytes.Split(bytes.TrimSuffix(output.Bytes(), []byte("\n")), []byte("\n")) {
					var object map[string]string
					if err := json.Unmarshal(record, &object); err != nil {
						t.Fatalf("decode %q: %v", record, err)
					}
					if len(object) != 1 {
						t.Fatalf("not a filename-only object: %v", object)
					}
					decoded = append(decoded, object["filename"])
				}
			}
			if !reflect.DeepEqual(decoded, names) {
				t.Fatalf("decoded = %q, want %q", decoded, names)
			}
		})
	}
}

func TestEmissionTextMatchesLegacyFormatting(t *testing.T) {
	for _, policy := range []OutputPolicy{{}, {ShowFilename: true, ShowLine: true}, {Hashline: true}} {
		var output bytes.Buffer
		emitter := newBlockEmitter(policy)
		emitter.writer = &output
		blocks := emissionBlocks("one", "two")
		var expected []string
		for _, path := range []string{"first", "last"} {
			if err := emitter.emit(blocks, path); err != nil {
				t.Fatalf("emit: %v", err)
			}
			expected = append(expected, strings.Join(blocks.Format(FormatOptions{Filename: path, ShowFilename: policy.ShowFilename, ShowLineNumbers: policy.ShowLine, Hashline: policy.Hashline}), "\n\n")+"\n")
		}
		if output.String() != strings.Join(expected, "\n") {
			t.Fatalf("policy %+v output = %q, want %q", policy, output.String(), strings.Join(expected, "\n"))
		}
	}
}

func TestEmissionPersistentConsumerStartsOnceAndReceivesNDJSON(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "starts")
	command := fmt.Sprintf("printf 'start\\n' >> %q; while IFS= read -r record; do printf '%%s\\n' \"$record\"; done", marker)
	var output bytes.Buffer
	emitter := newBlockEmitter(OutputPolicy{PersistentStreamCommand: command})
	emitter.writer = &output
	var expected bytes.Buffer
	for _, path := range []string{"a\nb", "c\\d"} {
		blocks := emissionBlocks("hit", "another hit")
		if err := emitter.emit(blocks, path); err != nil {
			t.Fatalf("emit: %v", err)
		}
		for _, block := range blocks {
			encoded, err := block.EncodeJSON(path, false)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			expected.Write(encoded)
			expected.WriteByte('\n')
		}
	}
	if err := emitter.close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	starts, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read starts: %v", err)
	}
	if string(starts) != "start\n" {
		t.Fatalf("starts = %q", starts)
	}
	if !bytes.Equal(output.Bytes(), expected.Bytes()) {
		t.Fatalf("output = %q, want %q", output.Bytes(), expected.Bytes())
	}
	if emitter.outputBytes != int64(expected.Len()) {
		t.Fatalf("accounted bytes = %d, want %d", emitter.outputBytes, expected.Len())
	}
}

func TestEmissionPerBlockConsumerKeepsEOFWithoutNewline(t *testing.T) {
	var output bytes.Buffer
	emitter := newBlockEmitter(OutputPolicy{StreamCommand: "if IFS= read -r record; then printf 'newline'; else printf 'eof'; fi"})
	emitter.writer = &output
	if err := emitter.emit(emissionBlocks("hit", "hit"), "file"); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if output.String() != "eofeof" {
		t.Fatalf("output = %q", output.String())
	}
}

func TestEmissionPersistentBudgetCancelsConsumer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	emitter := newBlockEmitter(OutputPolicy{PersistentStreamCommand: "cat >/dev/null; sleep 30", OutputBytes: 1})
	emitter.ctx = ctx
	emitter.writer = io.Discard
	started := time.Now()
	err := emitter.emit(emissionBlocks("hit"), "file")
	var limit *LimitError
	if !errors.As(err, &limit) {
		t.Fatalf("emit = %v, want limit", err)
	}
	_ = emitter.close()
	if time.Since(started) > time.Second {
		t.Fatalf("consumer cleanup took %s", time.Since(started))
	}
}
