//go:build unix

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestMachineCLIShutdownProcess(t *testing.T) {
	if os.Getenv("BLOCKSEARCH_SHUTDOWN_TEST") != "1" {
		return
	}
	for index, arg := range os.Args {
		if arg == "--" {
			signal.Ignore(syscall.SIGPIPE)
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
			fmt.Fprintln(os.Stderr, "ready")
			code := runCLI(ctx, os.Args[index+1:])
			stop()
			os.Exit(code)
		}
	}
	t.Fatalf("missing helper argument separator")
}

type shutdownCLI struct {
	command *exec.Cmd
	ctx     context.Context
	stdout  *os.File
	stderr  *bufio.Reader
}

func startShutdownCLI(t *testing.T, args ...string) shutdownCLI {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	command := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestMachineCLIShutdownProcess$", "--", "--diagnostics=json", "-j"}, args...)...)
	command.Env = append(os.Environ(), "BLOCKSEARCH_SHUTDOWN_TEST=1", "GOMAXPROCS=2", "HOME="+t.TempDir())
	stdout, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	t.Cleanup(func() { _ = stdout.Close() })
	defer writer.Close()
	command.Stdout = writer
	stderr, err := command.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	if err := command.Start(); err != nil {
		t.Fatalf("start CLI: %v", err)
	}
	t.Cleanup(func() {
		_ = command.Process.Kill()
		_ = command.Wait()
	})
	reader := bufio.NewReader(stderr)
	ready, err := reader.ReadString('\n')
	if err != nil || ready != "ready\n" {
		t.Fatalf("CLI ready = %q, %v", ready, err)
	}
	return shutdownCLI{command: command, ctx: ctx, stdout: stdout, stderr: reader}
}

func (cli shutdownCLI) expectFailure(t *testing.T, kind string) {
	t.Helper()
	diagnostics, readErr := io.ReadAll(cli.stderr)
	err := cli.command.Wait()
	if cli.ctx.Err() != nil || readErr != nil || err == nil || cli.command.ProcessState.ExitCode() != 1 {
		t.Fatalf("shutdown = %v, context = %v, stderr = %q, read = %v", err, cli.ctx.Err(), diagnostics, readErr)
	}
	records := machineRecords(t, string(diagnostics))
	if len(records) < 2 || records[0]["kind"] != kind || records[len(records)-1]["results_partial"] != true {
		t.Fatalf("shutdown diagnostics = %#v; want %s and partial completion", records, kind)
	}
}

func TestMachineCLICancelsUnconnectedFIFO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.txt")
	if err := unix.Mkfifo(path, 0600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	cli := startShutdownCLI(t, "hit", path)
	time.Sleep(50 * time.Millisecond)
	if err := cli.command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal CLI: %v", err)
	}
	cli.expectFailure(t, "canceled")
}

func TestMachineCLICancelsBlockedStdout(t *testing.T) {
	for _, cause := range []string{"signal", "worker limit"} {
		t.Run(cause, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "large.txt")
			if err := os.WriteFile(path, []byte("hit "+strings.Repeat("x", 1<<20)), 0600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			args := []string{"--max-matches=1", "hit", path}
			fifo := filepath.Join(root, "delayed.txt")
			if cause == "worker limit" {
				if err := unix.Mkfifo(fifo, 0600); err != nil {
					t.Fatalf("mkfifo: %v", err)
				}
				args = append(args, fifo)
			}
			cli := startShutdownCLI(t, args...)
			if _, err := cli.stdout.Read(make([]byte, 1)); err != nil {
				t.Fatalf("wait for first output byte: %v", err)
			}
			kind := "canceled"
			if cause == "signal" {
				if err := cli.command.Process.Signal(syscall.SIGTERM); err != nil {
					t.Fatalf("signal CLI: %v", err)
				}
			} else {
				kind = "limit"
				writer, err := os.OpenFile(fifo, os.O_WRONLY|unix.O_NONBLOCK, 0)
				for errors.Is(err, unix.ENXIO) && cli.ctx.Err() == nil {
					time.Sleep(time.Millisecond)
					writer, err = os.OpenFile(fifo, os.O_WRONLY|unix.O_NONBLOCK, 0)
				}
				if err != nil {
					t.Fatalf("open delayed input: %v", err)
				}
				_, writeErr := writer.WriteString("hit hit\n")
				closeErr := writer.Close()
				if writeErr != nil || closeErr != nil {
					t.Fatalf("send delayed input: %v, %v", writeErr, closeErr)
				}
			}
			cli.expectFailure(t, kind)
		})
	}
}
