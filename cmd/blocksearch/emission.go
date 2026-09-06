package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
)

type OutputError struct {
	Err error
}

func (e *OutputError) Error() string { return e.Err.Error() }
func (e *OutputError) Unwrap() error { return e.Err }

type blockEmitter struct {
	policy            OutputPolicy
	writer            io.Writer
	ctx               context.Context
	cancel            context.CancelCauseFunc
	consumer          *streamConsumer
	spaced            bool
	outputBytes       int64
	emittedBlockCount int
}

func newBlockEmitter(policy OutputPolicy) *blockEmitter {
	return &blockEmitter{policy: policy, writer: os.Stdout}
}

func (e *blockEmitter) start() error {
	if e.ctx == nil {
		e.ctx = context.Background()
	}
	if e.cancel == nil {
		e.cancel = func(error) {}
	}
	if e.policy.PersistentStreamCommand == "" || e.consumer != nil {
		return nil
	}
	consumer, err := startStreamConsumer(e.ctx, e.policy.PersistentStreamCommand, e.writer, e.cancel)
	if err != nil {
		return err
	}
	e.consumer = consumer
	return nil
}

func (e *blockEmitter) close() error {
	if e.consumer == nil {
		return nil
	}
	return e.consumer.close()
}

func (e *blockEmitter) emit(blocks Blocks, path string) error {
	if err := e.start(); err != nil {
		return err
	}
	if err := e.ctx.Err(); err != nil {
		return context.Cause(e.ctx)
	}
	if len(blocks) == 0 {
		return nil
	}
	if e.policy.FilesOnly {
		if err := e.emitFilename(path); err != nil {
			return err
		}
		e.emittedBlockCount += len(blocks)
		return nil
	}
	for _, block := range blocks {
		if err := e.ctx.Err(); err != nil {
			return context.Cause(e.ctx)
		}
		var err error
		switch {
		case e.consumer != nil, e.policy.StreamCommand != "", e.policy.JSON:
			err = e.emitJSON(block, path)
		default:
			err = e.emitText(block, path)
		}
		if err != nil {
			return err
		}
		e.emittedBlockCount++
	}
	return nil
}

func (e *blockEmitter) emitFilename(path string) error {
	var record []byte
	switch {
	case e.policy.Null:
		record = append([]byte(path), 0)
	case e.policy.JSON:
		encoded, err := json.Marshal(struct {
			Filename string `json:"filename"`
		}{path})
		if err != nil {
			return err
		}
		record = append(encoded, '\n')
	default:
		record = []byte(path + "\n")
	}
	return e.write(e.writer, record)
}

func (e *blockEmitter) emitJSON(block Block, path string) error {
	encoded, err := block.EncodeJSON(path, e.policy.Hashline)
	if err != nil {
		return err
	}
	if e.policy.StreamCommand != "" {
		if err := e.checkBudget(len(encoded)); err != nil {
			return err
		}
		consumer, err := startStreamConsumer(e.ctx, e.policy.StreamCommand, e.writer, e.cancel)
		if err != nil {
			return err
		}
		err = e.write(consumer, encoded)
		if err != nil {
			e.cancel(err)
			consumer.abort(err)
		}
		closeErr := consumer.close()
		if err != nil {
			return err
		}
		return closeErr
	}
	record := append(encoded, '\n')
	if e.consumer != nil {
		err := e.write(e.consumer, record)
		if err != nil {
			e.cancel(err)
			e.consumer.abort(err)
		}
		return err
	}
	return e.write(e.writer, record)
}

func (e *blockEmitter) emitText(block Block, path string) error {
	options := FormatOptions{
		Filename:        path,
		ShowFilename:    e.policy.ShowFilename,
		ShowLineNumbers: e.policy.ShowLine,
		UseColors:       e.policy.UseColors,
		Hashline:        e.policy.Hashline,
	}
	text := strings.Join(Blocks{block}.Format(options), "\n\n") + "\n"
	if e.spaced {
		text = "\n" + text
	}
	if err := e.write(e.writer, []byte(text)); err != nil {
		return err
	}
	e.spaced = true
	return nil
}

func (e *blockEmitter) checkBudget(size int) error {
	if e.policy.OutputBytes > 0 && int64(size) > e.policy.OutputBytes-e.outputBytes {
		return &LimitError{Resource: "output_bytes", Maximum: e.policy.OutputBytes}
	}
	return nil
}

func (e *blockEmitter) write(writer io.Writer, record []byte) error {
	if err := e.ctx.Err(); err != nil {
		return context.Cause(e.ctx)
	}
	if err := e.checkBudget(len(record)); err != nil {
		return err
	}
	n, err := writer.Write(record)
	e.outputBytes += int64(n)
	if err != nil {
		return &OutputError{Err: err}
	}
	if n != len(record) {
		return &OutputError{Err: io.ErrShortWrite}
	}
	return nil
}

func writeAll(writer io.Writer, buffer []byte) error {
	n, err := writer.Write(buffer)
	if err != nil {
		return err
	}
	if n != len(buffer) {
		return io.ErrShortWrite
	}
	return nil
}
