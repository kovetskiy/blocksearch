//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"
)

func TestFindBlocksCancelsBlockedPipeRead(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer reader.Close()
	defer writer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := findBlocksWithOptions(ctx, fmt.Sprintf("/proc/self/fd/%d", reader.Fd()), regexp.MustCompile("hit"), ParseOptions{})
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("blocked read cancellation = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("cancellation did not interrupt blocked pipe read")
	}
}
