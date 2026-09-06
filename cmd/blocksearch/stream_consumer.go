package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

type StreamError struct {
	Command string
	Err     error
}

func (e *StreamError) Error() string { return fmt.Sprintf("stream command %q: %v", e.Command, e.Err) }
func (e *StreamError) Unwrap() error { return e.Err }

type streamConsumer struct {
	command    string
	input      *os.File
	cancel     context.CancelCauseFunc
	done       chan struct{}
	waitErr    error
	stopClose  func() bool
	cancelDone chan struct{}
}

func startStreamConsumer(parent context.Context, command string, stdout io.Writer, fail context.CancelCauseFunc) (*streamConsumer, error) {
	ctx, cancel := context.WithCancelCause(parent)
	reader, writer, err := os.Pipe()
	if err != nil {
		cancel(err)
		return nil, &StreamError{Command: command, Err: err}
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Stdin = reader
	cmd.Stdout = stdout
	cmd.Stderr = os.Stderr
	cmd.WaitDelay = time.Second
	configureStreamProcess(cmd)
	if err := cmd.Start(); err != nil {
		reader.Close()
		writer.Close()
		cancel(err)
		return nil, &StreamError{Command: command, Err: err}
	}
	reader.Close()
	consumer := &streamConsumer{
		command:    command,
		input:      writer,
		cancel:     cancel,
		done:       make(chan struct{}),
		cancelDone: make(chan struct{}),
	}
	consumer.stopClose = context.AfterFunc(ctx, func() {
		defer close(consumer.cancelDone)
		writer.Close()
		_ = cmd.Cancel()
	})
	go func() {
		err := cmd.Wait()
		writer.Close()
		if err != nil {
			consumer.waitErr = &StreamError{Command: command, Err: err}
			consumer.abort(consumer.waitErr)
			fail(consumer.waitErr)
		}
		close(consumer.done)
	}()
	return consumer, nil
}

func (c *streamConsumer) Write(record []byte) (int, error) {
	n, err := c.input.Write(record)
	if err != nil {
		err = &StreamError{Command: c.command, Err: err}
		c.abort(err)
	}
	return n, err
}

func (c *streamConsumer) abort(err error) { c.cancel(err) }

func (c *streamConsumer) close() error {
	err := c.input.Close()
	if err != nil && !errors.Is(err, os.ErrClosed) {
		c.abort(err)
	} else {
		err = nil
	}
	<-c.done
	if !c.stopClose() {
		<-c.cancelDone
	}
	c.cancel(nil)
	if c.waitErr != nil {
		return c.waitErr
	}
	if err != nil {
		return &StreamError{Command: c.command, Err: err}
	}
	return nil
}
