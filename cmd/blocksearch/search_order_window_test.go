package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSearchWindowBoundsResultsAheadOfSlowFirstFile(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for i := 0; i < 20; i++ {
		path := filepath.Join(dir, fmt.Sprintf("%02d.txt", i))
		if err := os.WriteFile(path, []byte("add\n"), 0600); err != nil {
			t.Fatalf("write: %v", err)
		}
		paths = append(paths, path)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	searchCtx, fail := context.WithCancelCause(ctx)
	defer fail(nil)
	search := newSearchForTest(t, []string{dir})
	jobs := make(chan searchJob)
	window := make(chan struct{}, 2)
	walked := make(chan error, 1)
	go func() { walked <- search.walkAndEnqueueFilesContext(searchCtx, jobs, window) }()
	results := make(chan searchResult)
	started := make(chan searchJob, len(paths))
	go func() {
		defer close(results)
		for job := range jobs {
			started <- job
			if job.seq == 0 {
				continue
			}
			select {
			case results <- searchResult{seq: job.seq, path: job.path, blocks: emissionBlocks("add")}:
			case <-searchCtx.Done():
				return
			}
		}
	}()
	var output bytes.Buffer
	emitter := newBlockEmitter(OutputPolicy{FilesOnly: true})
	emitter.writer, emitter.ctx, emitter.cancel = &output, searchCtx, fail
	emitted := make(chan emitOutcome, 1)
	go func() { emitted <- emitOrderedContext(searchCtx, fail, results, emitter, func() { <-window }) }()
	for i := 0; i < cap(window); i++ {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatalf("initial window did not start")
		}
	}
	select {
	case job := <-started:
		t.Fatalf("file %d started beyond the window", job.seq)
	case <-time.After(30 * time.Millisecond):
	}
	select {
	case results <- searchResult{seq: 0, path: paths[0], blocks: emissionBlocks("add")}:
	case <-ctx.Done():
		t.Fatalf("could not release first file")
	}
	select {
	case outcome := <-emitted:
		if outcome.err != nil || outcome.emittedBlockCount != len(paths) {
			t.Fatalf("outcome = %+v", outcome)
		}
	case <-ctx.Done():
		t.Fatalf("emitter did not finish")
	}
	if err := <-walked; err != nil {
		t.Fatalf("walk: %v", err)
	}
	if output.String() != strings.Join(paths, "\n")+"\n" {
		t.Fatalf("output is not walk ordered: %q", output.String())
	}
}
