package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func searchOutputFile(t *testing.T) *os.File {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatalf("create output: %v", err)
	}
	previous := os.Stdout
	os.Stdout = file
	t.Cleanup(func() { os.Stdout = previous; file.Close() })
	return file
}

func readSearchOutput(t *testing.T, file *os.File) []byte {
	t.Helper()
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("seek output: %v", err)
	}
	output, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	return output
}

func TestSearchResourceOrderedOutputAndRawFilenames(t *testing.T) {
	previous := runtime.GOMAXPROCS(4)
	defer runtime.GOMAXPROCS(previous)
	dir := t.TempDir()
	names := []string{"z\nlast.txt", "a\tfirst.txt", "c\\path.txt", "invalid-\xff.txt"}
	var paths []string
	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("add\n"), 0600); err != nil {
			t.Fatalf("write: %v", err)
		}
		paths = append(paths, path)
	}
	output := searchOutputFile(t)
	search := newSearchForTest(t, paths)
	search.output = OutputPolicy{FilesOnly: true, Null: true}
	count, err := search.Run()
	if err != nil || count != len(paths) {
		t.Fatalf("Run = %d, %v", count, err)
	}
	decoded := strings.Split(strings.TrimSuffix(string(readSearchOutput(t, output)), "\x00"), "\x00")
	if !reflect.DeepEqual(decoded, paths) {
		t.Fatalf("filenames = %q, want %q", decoded, paths)
	}
}

func TestSearchResourceOutputLimitIsGlobal(t *testing.T) {
	previous := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previous)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"a.txt": "add\n", "b.txt": "add\n"})
	paths := []string{filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")}
	maximum := int64(len(paths[0]) + 1)
	output := searchOutputFile(t)
	search := newSearchForTest(t, paths)
	search.output = OutputPolicy{FilesOnly: true, Null: true, OutputBytes: maximum}
	count, err := search.Run()
	var limit *LimitError
	if count != 1 || !errors.As(err, &limit) || limit.Resource != "output_bytes" {
		t.Fatalf("Run = %d, %v", count, err)
	}
	if got := readSearchOutput(t, output); !bytes.Equal(got, append([]byte(paths[0]), 0)) {
		t.Fatalf("output = %q", got)
	}
}

func TestSearchResourceInputLimitCancelsInsteadOfContinuing(t *testing.T) {
	previous := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previous)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"a.txt": "add add add\n", "b.txt": "add\n"})
	output := searchOutputFile(t)
	search := newSearchForTest(t, []string{filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")})
	search.parseOptions.Limits.FileBytes = 5
	count, err := search.Run()
	var limit *LimitError
	var pathErr *SearchPathError
	if count != 0 || !errors.As(err, &limit) || !errors.As(err, &pathErr) || pathErr.Path != filepath.Join(dir, "a.txt") {
		t.Fatalf("Run = %d, %v", count, err)
	}
	if got := readSearchOutput(t, output); len(got) != 0 {
		t.Fatalf("output = %q", got)
	}
}

func TestSearchResourcePreCanceledDoesNotStartConsumer(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "started")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	search := newSearchForTest(t, nil)
	search.output.PersistentStreamCommand = fmt.Sprintf("touch %q", marker)
	count, err := search.RunContext(ctx)
	if count != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("RunContext = %d, %v", count, err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("consumer started: %v", err)
	}
}

func TestSearchResourcePersistentStartsEvenWithNoMatches(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "started")
	search := newSearchForTest(t, nil)
	search.output.PersistentStreamCommand = fmt.Sprintf("printf start > %q; cat >/dev/null", marker)
	count, err := search.Run()
	if count != 0 || err != nil {
		t.Fatalf("Run = %d, %v", count, err)
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "start" {
		t.Fatalf("marker = %q, %v", data, err)
	}
}

func TestSearchResourceConsumerFailureIsStructured(t *testing.T) {
	search := newSearchForTest(t, nil)
	search.output.PersistentStreamCommand = "exit 23"
	_, err := search.Run()
	var stream *StreamError
	if !errors.As(err, &stream) || stream.Command != "exit 23" {
		t.Fatalf("Run = %v", err)
	}
}

type heldEmissionWriter struct {
	entered chan struct{}
	ctx     context.Context
}

func (writer *heldEmissionWriter) Write(record []byte) (int, error) {
	select {
	case writer.entered <- struct{}{}:
	default:
	}
	<-writer.ctx.Done()
	return 0, writer.ctx.Err()
}

func TestSearchResourceInFlightWindowIncludesBlockedEmission(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 20; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%02d.txt", i)), []byte("add\n"), 0600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	search := newSearchForTest(t, []string{dir})
	jobs := make(chan searchJob)
	window := make(chan struct{}, 2)
	walked := make(chan error, 1)
	go func() { walked <- search.walkAndEnqueueFilesContext(ctx, jobs, window) }()
	results := search.startWorkersContext(ctx, cancel, jobs, 2)
	writer := &heldEmissionWriter{entered: make(chan struct{}, 1), ctx: ctx}
	emitter := newBlockEmitter(OutputPolicy{JSON: true})
	emitter.writer, emitter.ctx, emitter.cancel = writer, ctx, cancel
	emitted := make(chan emitOutcome, 1)
	go func() { emitted <- emitOrderedContext(ctx, cancel, results, emitter, func() { <-window }) }()
	select {
	case <-writer.entered:
	case <-time.After(3 * time.Second):
		t.Fatalf("emitter never started")
	}
	select {
	case err := <-walked:
		t.Fatalf("walk ran ahead while first output was blocked: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	if len(window) != cap(window) {
		t.Fatalf("in flight = %d, want %d", len(window), cap(window))
	}
	cancel(context.Canceled)
	select {
	case <-emitted:
	case <-time.After(3 * time.Second):
		t.Fatalf("emitter did not cancel")
	}
	select {
	case <-walked:
	case <-time.After(3 * time.Second):
		t.Fatalf("walker did not cancel")
	}
	for range results {
	}
}

func TestSearchResourcePersistentLimitKeepsLimitDiagnostic(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"a.txt": "add\n"})
	search := newSearchForTest(t, []string{filepath.Join(dir, "a.txt")})
	search.output = OutputPolicy{PersistentStreamCommand: "cat >/dev/null", OutputBytes: 1}
	count, err := search.Run()
	var limit *LimitError
	if count != 0 || !errors.As(err, &limit) || limit.Resource != "output_bytes" {
		t.Fatalf("Run = %d, %v; want output limit, not killed-child status", count, err)
	}
}
