package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/reconquest/pkg/log"
)

// OutputPolicy carries the formatting and emission choices derived from the
// CLI flags, so the per-file processing routine reads positive names instead
// of re-deriving them from --no-* flags.
type OutputPolicy struct {
	ShowFilename  bool
	ShowLine      bool
	UseColors     bool
	FilesOnly     bool
	JSON          bool
	Hashline      bool
	StreamCommand string
}

type Search struct {
	query   *regexp.Regexp
	filters []*BlockConditionMatcher
	files   []string
	output  OutputPolicy
	walker  *FileWalker
}

// Run searches every input file through a worker pool and emits blocks in
// walk order. The walk is serial (it is cheap directory I/O), while each
// file's parse and match run on one of GOMAXPROCS workers — the pool tracks
// the default thread count, so the search uses every available core. Results
// carry the sequence number assigned at enqueue time, and a single emitter
// goroutine reassembles them in that order, so concurrent parsing cannot
// scramble the deterministic output the walk order guarantees.
func (s *Search) Run() (int, error) {
	jobs := make(chan searchJob)
	results := s.startWorkers(jobs)

	emitted := make(chan emitOutcome, 1)
	go func() {
		emitted <- emitOrdered(results, newBlockEmitter(s.output))
	}()

	walkErr := s.walkAndEnqueueFiles(jobs)
	outcome := <-emitted
	return outcome.emittedBlockCount, errors.Join(walkErr, outcome.err)
}

func (s *Search) startWorkers(jobs <-chan searchJob) <-chan searchResult {
	results := make(chan searchResult)
	var pool sync.WaitGroup
	for i := 0; i < runtime.GOMAXPROCS(0); i++ {
		pool.Add(1)
		go func() {
			defer pool.Done()
			for job := range jobs {
				log.Debug("process: " + job.path)
				blocks, err := s.searchFile(job.path)
				results <- searchResult{
					seq:    job.seq,
					path:   job.path,
					blocks: blocks,
					err:    err,
				}
			}
		}()
	}
	go func() {
		pool.Wait()
		close(results)
	}()
	return results
}

func (s *Search) walkAndEnqueueFiles(jobs chan<- searchJob) error {
	defer close(jobs)

	var walkErr error
	seq := 0
	for _, fileArg := range s.files {
		resolvedPaths, err := resolveFileArg(fileArg)
		if err != nil {
			walkErr = errors.Join(walkErr, fmt.Errorf("%s: %w", fileArg, err))
			continue
		}
		for _, resolvedPath := range resolvedPaths {
			log.Debug("stat: " + resolvedPath)
			err := s.walker.Walk(resolvedPath, func(filePath string) error {
				jobs <- searchJob{seq: seq, path: filePath}
				seq++
				return nil
			})
			if err != nil {
				walkErr = errors.Join(walkErr, fmt.Errorf("%s: %w", resolvedPath, err))
			}
		}
	}
	return walkErr
}

// searchJob is one file to search; seq is its position in walk order.
type searchJob struct {
	seq  int
	path string
}

// searchResult carries a searched file's blocks back to the emitter with
// the job's sequence number, so output can be reassembled in walk order no
// matter which worker finished first.
type searchResult struct {
	seq    int
	path   string
	blocks Blocks
	err    error
}

// emitOutcome is the emitter goroutine's final tally: blocks emitted and
// the joined per-file search and write errors.
type emitOutcome struct {
	emittedBlockCount int
	err               error
}

// emitOrdered consumes results as workers finish them, buffers any that
// arrive ahead of their walk-order position, and emits each file's blocks
// only once every earlier file has been emitted. A failing file does not
// stop the run: its error is joined and later files are still emitted,
// matching grep's behavior of reporting every input it could not read.
func emitOrdered(results <-chan searchResult, emitter *blockEmitter) emitOutcome {
	emittedBlockCount := 0
	var resultErr error
	pending := map[int]searchResult{}
	next := 0

	for receivedResult := range results {
		pending[receivedResult.seq] = receivedResult
		for {
			pendingResult, ok := pending[next]
			if !ok {
				break
			}
			delete(pending, next)
			next++

			if pendingResult.err != nil {
				resultErr = errors.Join(
					resultErr,
					fmt.Errorf("%s: %w", pendingResult.path, pendingResult.err),
				)
				continue
			}
			if len(pendingResult.blocks) == 0 {
				continue
			}
			if err := emitter.emit(pendingResult.blocks, pendingResult.path); err != nil {
				resultErr = errors.Join(resultErr, err)
				continue
			}
			emittedBlockCount += len(pendingResult.blocks)
		}
	}

	return emitOutcome{emittedBlockCount: emittedBlockCount, err: resultErr}
}

// searchFile runs the CPU-bound part of a search — read, parse, match, and
// filter — with no emission, so workers can run it concurrently while a
// single emitter owns stdout.
func (s *Search) searchFile(path string) (Blocks, error) {
	blocks, err := findBlocks(path, s.query)
	if err != nil {
		return nil, err
	}

	return filterBlocks(blocks, s.filters)
}

type blockEmitter struct {
	policy OutputPolicy
	writer io.Writer

	// spaced tracks whether a file's blocks have already been emitted, so
	// later files are separated by a blank line in the default format.
	spaced bool
}

func newBlockEmitter(policy OutputPolicy) *blockEmitter {
	return &blockEmitter{policy: policy, writer: os.Stdout}
}

func (e *blockEmitter) emit(blocks Blocks, path string) error {
	if e.policy.FilesOnly {
		return writeAll(e.writer, []byte(path+"\n"))
	}

	switch {
	case e.policy.StreamCommand != "":
		if err := blocks.Stream(e.policy.StreamCommand, path, e.policy.Hashline); err != nil {
			return err
		}
	case e.policy.JSON:
		buffer, err := blocks.EncodeJSON(path, e.policy.Hashline)
		if err != nil {
			return err
		}
		return writeAll(e.writer, buffer)
	default:
		if e.spaced {
			if err := writeAll(e.writer, []byte("\n")); err != nil {
				return err
			}
		}
		options := FormatOptions{
			Filename:        path,
			ShowFilename:    e.policy.ShowFilename,
			ShowLineNumbers: e.policy.ShowLine,
			UseColors:       e.policy.UseColors,
			Hashline:        e.policy.Hashline,
		}
		if err := writeAll(
			e.writer,
			[]byte(strings.Join(blocks.Format(options), "\n\n")+"\n"),
		); err != nil {
			return err
		}

		e.spaced = true
	}

	return nil
}

// writeAll writes the full buffer to w, failing if the write is short or the
// writer errors, so stdout failures surface as non-zero CLI exit codes.
func writeAll(w io.Writer, buffer []byte) error {
	n, err := w.Write(buffer)
	if err != nil {
		return err
	}
	if n < len(buffer) {
		return io.ErrShortWrite
	}
	return nil
}
