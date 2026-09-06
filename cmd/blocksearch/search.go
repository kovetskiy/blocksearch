package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sync"

	"github.com/reconquest/pkg/log"
)

type OutputPolicy struct {
	ShowFilename            bool
	ShowLine                bool
	UseColors               bool
	FilesOnly               bool
	JSON                    bool
	Hashline                bool
	Null                    bool
	StreamCommand           string
	PersistentStreamCommand string
	OutputBytes             int64
}

type Search struct {
	query        *regexp.Regexp
	filters      []*BlockConditionMatcher
	files        []string
	output       OutputPolicy
	walker       *FileWalker
	parseOptions ParseOptions
}

type SearchPathError struct {
	Path string
	Err  error
}

func (e *SearchPathError) Error() string { return fmt.Sprintf("%s: %v", e.Path, e.Err) }
func (e *SearchPathError) Unwrap() error { return e.Err }

func (s *Search) Run() (int, error) { return s.RunContext(context.Background()) }

func (s *Search) RunContext(parent context.Context) (int, error) {
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
	if err := ctx.Err(); err != nil {
		return 0, context.Cause(ctx)
	}

	emitter := newBlockEmitter(s.output)
	emitter.ctx = ctx
	emitter.cancel = cancel
	if s.output.StreamCommand == "" && s.output.PersistentStreamCommand == "" {
		writer, cleanup, err := prepareCLIOutput(ctx, os.Stdout)
		if err != nil {
			return 0, &OutputError{Err: err}
		}
		defer cleanup()
		emitter.writer = writer
	}
	if err := emitter.start(); err != nil {
		return 0, err
	}

	jobs := make(chan searchJob)
	window := make(chan struct{}, runtime.GOMAXPROCS(0))
	results := s.startWorkersContext(ctx, cancel, jobs, cap(window))
	walked := make(chan error, 1)
	go func() { walked <- s.walkAndEnqueueFilesContext(ctx, jobs, window) }()

	outcome := emitOrderedContext(ctx, cancel, results, emitter, func() { <-window })
	closeErr := emitter.close()
	if closeErr != nil {
		cancel(closeErr)
	}
	walkErr := <-walked
	for range results {
	}
	return outcome.emittedBlockCount, errors.Join(walkErr, outcome.err, context.Cause(ctx))
}

func (s *Search) startWorkers(jobs <-chan searchJob) <-chan searchResult {
	return s.startWorkersContext(context.Background(), func(error) {}, jobs, runtime.GOMAXPROCS(0))
}

func (s *Search) startWorkersContext(ctx context.Context, cancel context.CancelCauseFunc, jobs <-chan searchJob, workers int) <-chan searchResult {
	results := make(chan searchResult)
	var pool sync.WaitGroup
	for i := 0; i < workers; i++ {
		pool.Add(1)
		go func() {
			defer pool.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-jobs:
					if !ok || ctx.Err() != nil {
						return
					}
					log.Debug("process: " + job.path)
					blocks, err := s.searchFileContext(ctx, job.path)
					var limit *LimitError
					if errors.As(err, &limit) {
						cancel(&SearchPathError{Path: job.path, Err: err})
					}
					select {
					case results <- searchResult{seq: job.seq, path: job.path, blocks: blocks, err: err}:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}
	go func() { pool.Wait(); close(results) }()
	return results
}

func (s *Search) walkAndEnqueueFiles(jobs chan<- searchJob) error {
	return s.walkAndEnqueueFilesContext(context.Background(), jobs, nil)
}

func (s *Search) walkAndEnqueueFilesContext(ctx context.Context, jobs chan<- searchJob, window chan struct{}) error {
	defer close(jobs)
	var walkErr error
	seq := 0
	for _, fileArg := range s.files {
		if ctx.Err() != nil {
			break
		}
		resolvedPaths, err := resolveFileArg(fileArg)
		if err != nil {
			walkErr = errors.Join(walkErr, &SearchPathError{Path: fileArg, Err: err})
			continue
		}
		for _, resolvedPath := range resolvedPaths {
			if ctx.Err() != nil {
				return walkErr
			}
			log.Debug("stat: " + resolvedPath)
			err := s.walker.WalkContext(ctx, resolvedPath, func(filePath string) error {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if window != nil {
					select {
					case window <- struct{}{}:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				select {
				case jobs <- searchJob{seq: seq, path: filePath}:
					seq++
					return nil
				case <-ctx.Done():
					if window != nil {
						<-window
					}
					return ctx.Err()
				}
			})
			if err != nil && ctx.Err() == nil {
				walkErr = errors.Join(walkErr, &SearchPathError{Path: resolvedPath, Err: err})
			}
		}
	}
	return walkErr
}

type searchJob struct {
	seq  int
	path string
}

type searchResult struct {
	seq    int
	path   string
	blocks Blocks
	err    error
}

type emitOutcome struct {
	emittedBlockCount int
	err               error
}

func emitOrdered(results <-chan searchResult, emitter *blockEmitter) emitOutcome {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	emitter.ctx, emitter.cancel = ctx, cancel
	if err := emitter.start(); err != nil {
		return emitOutcome{err: err}
	}
	outcome := emitOrderedContext(ctx, cancel, results, emitter, func() {})
	if err := emitter.close(); err != nil {
		cancel(err)
	}
	outcome.err = errors.Join(outcome.err, context.Cause(ctx))
	return outcome
}

func emitOrderedContext(ctx context.Context, cancel context.CancelCauseFunc, results <-chan searchResult, emitter *blockEmitter, release func()) emitOutcome {
	emittedBefore := emitter.emittedBlockCount
	var resultErr error
	pending := map[int]searchResult{}
	next := 0
	for {
		if ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
			return emitOutcome{emitter.emittedBlockCount - emittedBefore, resultErr}
		case received, ok := <-results:
			if !ok {
				return emitOutcome{emitter.emittedBlockCount - emittedBefore, resultErr}
			}
			var limit *LimitError
			if errors.As(received.err, &limit) {
				cancel(&SearchPathError{Path: received.path, Err: received.err})
				return emitOutcome{emitter.emittedBlockCount - emittedBefore, resultErr}
			}
			pending[received.seq] = received
			for {
				current, ok := pending[next]
				if !ok {
					break
				}
				delete(pending, next)
				next++
				if current.err != nil {
					resultErr = errors.Join(resultErr, &SearchPathError{Path: current.path, Err: current.err})
				} else if err := emitter.emit(current.blocks, current.path); err != nil {
					cancel(err)
					return emitOutcome{emitter.emittedBlockCount - emittedBefore, resultErr}
				}
				release()
			}
		}
	}
	return emitOutcome{emitter.emittedBlockCount - emittedBefore, resultErr}
}

func (s *Search) searchFile(path string) (Blocks, error) {
	return s.searchFileContext(context.Background(), path)
}

func (s *Search) searchFileContext(ctx context.Context, path string) (Blocks, error) {
	blocks, err := findBlocksWithOptions(ctx, path, s.query, s.parseOptions)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return filterSearchBlocks(ctx, blocks, s.filters)
}
