package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestStreamConsumerClosedStdinDoesNotHangWait(t *testing.T) {
	for _, persistent := range []bool{false, true} {
		name := "per-block"
		if persistent {
			name = "persistent"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			policy := OutputPolicy{}
			command := "exec 0<&-; sleep 30"
			if persistent {
				policy.PersistentStreamCommand = command
			} else {
				policy.StreamCommand = command
			}
			emitter := newBlockEmitter(policy)
			emitter.writer, emitter.ctx = io.Discard, ctx
			start := time.Now()
			err := emitter.emit(emissionBlocks(strings.Repeat("x", 1024*1024)), "file")
			_ = emitter.close()
			var stream *StreamError
			if !errors.As(err, &stream) {
				t.Fatalf("emit = %v, want StreamError", err)
			}
			if time.Since(start) > 2*time.Second {
				t.Fatalf("broken pipe cleanup took %s", time.Since(start))
			}
		})
	}
}

func TestStreamConsumerFailureCancelsWhileNoRecordsAreReady(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	consumer, err := startStreamConsumer(ctx, "exit 17", io.Discard, cancel)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
		consumer.abort(context.Canceled)
		t.Fatalf("exit did not cancel search")
	}
	var stream *StreamError
	if !errors.As(context.Cause(ctx), &stream) {
		t.Fatalf("cause = %v", context.Cause(ctx))
	}
	if err := consumer.close(); !errors.As(err, &stream) {
		t.Fatalf("close = %v", err)
	}
}

func TestStreamConsumerCancellationInterruptsBlockedWriteAndReaps(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	consumer, err := startStreamConsumer(ctx, "sleep 30", io.Discard, cancel)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	writing := make(chan error, 1)
	go func() {
		_, err := consumer.Write([]byte(strings.Repeat("x", 1024*1024)))
		writing <- err
	}()
	select {
	case err := <-writing:
		t.Fatalf("write should be blocked: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	cancel(context.Canceled)
	select {
	case err := <-writing:
		if err == nil {
			t.Fatalf("blocked write succeeded after cancellation")
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("write did not unblock")
	}
	closed := make(chan error, 1)
	go func() { closed <- consumer.close() }()
	select {
	case err := <-closed:
		if err == nil {
			t.Fatalf("canceled child exited successfully")
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("consumer was not reaped")
	}
	select {
	case <-consumer.done:
	default:
		t.Fatalf("close returned without waiting")
	}
}

func TestStreamConsumerCancellationInterruptsWaitAfterEOF(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	consumer, err := startStreamConsumer(ctx, "cat >/dev/null; sleep 30", io.Discard, func(error) {})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	start := time.Now()
	if err := consumer.close(); err == nil {
		t.Fatalf("close succeeded after cancellation")
	}
	if time.Since(start) > time.Second {
		t.Fatalf("wait cancellation took %s", time.Since(start))
	}
}

func TestStreamErrorUnwrap(t *testing.T) {
	cause := errors.New("consumer failure")
	err := &StreamError{Command: "command", Err: cause}
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "command") {
		t.Fatalf("StreamError = %v", err)
	}
}

func TestStreamConsumerStartFailureIsStructured(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	consumer, err := startStreamConsumer(context.Background(), "cat", io.Discard, func(error) {})
	var stream *StreamError
	if consumer != nil || !errors.As(err, &stream) || stream.Command != "cat" {
		t.Fatalf("start = %v, %v; want StreamError", consumer, err)
	}
}
