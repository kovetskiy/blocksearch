//go:build unix

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestCancellableIOUnixRegressions(t *testing.T) {
	const key = "BLOCKSEARCH_CANCELLABLE_IO_CASE"
	if scenario := os.Getenv(key); scenario != "" {
		switch scenario {
		case "fifo-unconnected", "fifo-text", "fifo-empty", "fifo-connected-cancel":
			cancellableIOCheckFIFO(t, scenario)
		case "stdin-cancel", "stdin-eof":
			cancellableIOCheckStdin(t, scenario)
		case "stdout-blocking", "stdout-nonblocking", "stdout-cleanup":
			cancellableIOCheckOutput(t, scenario)
		case "stdout-cleanup-races":
			cancellableIOCheckCleanupRaces(t)
		default:
			t.Fatalf("unknown child scenario: %s", scenario)
		}
		return
	}
	for _, scenario := range []string{
		"fifo-unconnected", "fifo-text", "fifo-empty", "fifo-connected-cancel", "stdin-cancel", "stdin-eof",
		"stdout-blocking", "stdout-nonblocking", "stdout-cleanup", "stdout-cleanup-races",
	} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCancellableIOUnixRegressions$", "-test.count=1")
			cmd.Env = append(os.Environ(), key+"="+scenario)
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatalf("stdin pipe: %v", err)
			}
			defer reader.Close()
			defer writer.Close()
			cmd.Stdin = reader
			if scenario != "stdin-cancel" {
				writer.Close()
			}
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("child failed (%v, timeout=%v):\n%s", err, ctx.Err(), out)
			}
		})
	}
}

func cancellableIOCheckFIFO(t *testing.T, scenario string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.fifo")
	if err := unix.Mkfifo(path, 0600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		file, err := openSearchInput(ctx, path)
		if err != nil {
			done <- result{err: err}
			return
		}
		defer file.Close()
		stop := context.AfterFunc(ctx, func() { _ = file.Close() })
		defer stop()
		text, err := readParseInput(ctx, file, 0)
		done <- result{string(text), err}
	}()
	select {
	case got := <-done:
		t.Fatalf("unconnected FIFO returned prematurely: %+v", got)
	case <-time.After(60 * time.Millisecond):
	}
	if scenario == "fifo-unconnected" {
		cancel()
		select {
		case got := <-done:
			if !errors.Is(got.err, context.Canceled) {
				t.Fatalf("FIFO cancellation = %+v", got)
			}
		case <-time.After(time.Second):
			t.Fatalf("unconnected FIFO open ignored cancellation")
		}
		return
	}

	fd := -1
	deadline := time.Now().Add(time.Second)
	for {
		var err error
		fd, err = unix.Open(path, unix.O_WRONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.ENXIO) || time.Now().After(deadline) {
			t.Fatalf("connect writer: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	want := ""
	if scenario == "fifo-text" || scenario == "fifo-connected-cancel" {
		want = "hit\r\nsecond line\n"
		n, err := unix.Write(fd, []byte(want))
		if err != nil || n != len(want) {
			unix.Close(fd)
			t.Fatalf("FIFO write = %d, %v", n, err)
		}
	}
	if scenario == "fifo-connected-cancel" {
		defer unix.Close(fd)
		select {
		case got := <-done:
			t.Fatalf("FIFO returned before writer EOF: %+v", got)
		case <-time.After(60 * time.Millisecond):
		}
		cancel()
		select {
		case got := <-done:
			if !errors.Is(got.err, context.Canceled) {
				t.Fatalf("connected FIFO cancellation = %+v", got)
			}
		case <-time.After(time.Second):
			t.Fatalf("connected FIFO read ignored cancellation")
		}
		return
	}
	if err := unix.Close(fd); err != nil {
		t.Fatalf("writer close: %v", err)
	}
	select {
	case got := <-done:
		if got.err != nil || got.text != want {
			t.Fatalf("FIFO result = %+v; want %q", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("connected FIFO did not reach EOF")
	}
}

func cancellableIOCheckStdin(t *testing.T, scenario string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	file, err := openSearchInput(ctx, "/dev/stdin")
	if err != nil {
		t.Fatalf("open inherited stdin: %v", err)
	}
	defer file.Close()
	stop := context.AfterFunc(ctx, func() { _ = file.Close() })
	defer stop()
	text, err := readParseInput(ctx, file, 0)
	if scenario == "stdin-cancel" {
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("stdin cancellation = %q, %v", text, err)
		}
	} else if err != nil || len(text) != 0 {
		t.Fatalf("exhausted inherited stdin = %q, %v; want EOF", text, err)
	}
	if _, err := os.Stdin.Stat(); err != nil {
		t.Fatalf("inherited stdin was closed: %v", err)
	}
}

func cancellableIOFlags(t *testing.T, file *os.File) int {
	t.Helper()
	raw, err := file.SyscallConn()
	if err != nil {
		t.Fatalf("syscall connection: %v", err)
	}
	var flags int
	var flagErr error
	if err := raw.Control(func(fd uintptr) {
		flags, flagErr = unix.FcntlInt(fd, unix.F_GETFL, 0)
	}); err != nil || flagErr != nil {
		t.Fatalf("get flags: %v, %v", err, flagErr)
	}
	return flags
}

func cancellableIOFillPipe(t *testing.T, file *os.File) {
	t.Helper()
	raw, err := file.SyscallConn()
	if err != nil {
		t.Fatalf("syscall connection: %v", err)
	}
	var writeErr error
	if err := raw.Control(func(fd uintptr) {
		buffer := make([]byte, 4096)
		for {
			_, writeErr = unix.Write(int(fd), buffer)
			if errors.Is(writeErr, unix.EINTR) {
				continue
			}
			if writeErr != nil {
				return
			}
		}
	}); err != nil || !errors.Is(writeErr, unix.EAGAIN) {
		t.Fatalf("fill pipe: %v, %v", err, writeErr)
	}
}

func cancellableIOCheckOutput(t *testing.T, scenario string) {
	t.Helper()
	reader, original, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer reader.Close()
	defer original.Close()
	if scenario != "stdout-nonblocking" {
		_ = original.Fd()
	}
	flags := cancellableIOFlags(t, original)
	stdout := os.Stdout
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output, cleanup, err := prepareCLIOutput(ctx, original)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	defer cleanup()
	if output == original || os.Stdout != stdout {
		t.Fatalf("helper changed ownership or global stdout")
	}
	if got := cancellableIOFlags(t, output); got&unix.O_NONBLOCK == 0 {
		t.Fatalf("duplicate is blocking: flags=%#x", got)
	}
	if scenario == "stdout-cleanup" {
		if _, err := output.WriteString("hit"); err != nil {
			t.Fatalf("write: %v", err)
		}
		cleanup()
		cancel()
	} else {
		cancellableIOFillPipe(t, output)
		done := make(chan error, 1)
		go func() {
			_, err := output.Write([]byte(strings.Repeat("hit", 1<<20)))
			done <- err
		}()
		select {
		case err := <-done:
			t.Fatalf("full pipe write returned before cancellation: %v", err)
		case <-time.After(60 * time.Millisecond):
		}
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, os.ErrClosed) {
				t.Fatalf("canceled write = %v; want closed descriptor", err)
			}
		case <-time.After(time.Second):
			t.Fatalf("full pipe write ignored cancellation")
		}
		cleanup()
	}
	cleanup()
	if got := cancellableIOFlags(t, original); got != flags {
		t.Fatalf("original flags = %#x; want %#x", got, flags)
	}
	if _, err := output.WriteString("closed"); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("duplicate remained open: %v", err)
	}
	buffer := make([]byte, 1<<20)
	if _, err := reader.Read(buffer); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if _, err := original.WriteString("ok"); err != nil {
		t.Fatalf("original output no longer writable: %v", err)
	}
}

func TestCancellableIOSIGTERMBackpressure(t *testing.T) {
	const key = "BLOCKSEARCH_CANCELLABLE_IO_SIGNAL_CHILD"
	if os.Getenv(key) == "1" {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
		defer stop()
		output, cleanup, err := prepareCLIOutput(ctx, os.Stdout)
		if err != nil {
			t.Fatalf("prepare stdout: %v", err)
		}
		defer cleanup()
		cancellableIOFillPipe(t, output)
		fmt.Fprintln(os.Stderr, "ready")
		_, err = output.Write([]byte(strings.Repeat("hit", 1<<20)))
		if !errors.Is(err, os.ErrClosed) || ctx.Err() == nil {
			t.Fatalf("SIGTERM write = %v, context=%v", err, ctx.Err())
		}
		cleanup()
		os.Exit(0)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCancellableIOSIGTERMBackpressure$", "-test.count=1")
	cmd.Env = append(os.Environ(), key+"=1")
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	defer reader.Close()
	defer writer.Close()
	cmd.Stdout = writer
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	marker := make([]byte, len("ready\n"))
	_, markerErr := io.ReadFull(stderr, marker)
	if markerErr == nil && string(marker) == "ready\n" {
		time.Sleep(60 * time.Millisecond)
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Errorf("signal: %v", err)
		}
	} else {
		t.Errorf("child readiness = %q, %v", marker, markerErr)
		cancel()
	}
	trailing, readErr := io.ReadAll(stderr)
	if err := cmd.Wait(); err != nil || readErr != nil {
		t.Fatalf("SIGTERM child failed: %v, read=%v, timeout=%v, stderr=%s", err, readErr, ctx.Err(), trailing)
	}
}

func cancellableIOCheckCleanupRaces(t *testing.T) {
	t.Helper()
	reader, original, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer reader.Close()
	defer original.Close()
	_ = original.Fd()
	flags := cancellableIOFlags(t, original)
	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		output, cleanup, err := prepareCLIOutput(ctx, original)
		if err != nil {
			cancel()
			t.Fatalf("prepare: %v", err)
		}
		var group sync.WaitGroup
		group.Add(3)
		go func() { defer group.Done(); cancel() }()
		go func() { defer group.Done(); cleanup() }()
		go func() { defer group.Done(); cleanup() }()
		group.Wait()
		if got := cancellableIOFlags(t, original); got != flags {
			t.Fatalf("restored flags = %#x; want %#x", got, flags)
		}
		if _, err := output.Stat(); !errors.Is(err, os.ErrClosed) {
			t.Fatalf("duplicate remained open: %v", err)
		}
	}
}
