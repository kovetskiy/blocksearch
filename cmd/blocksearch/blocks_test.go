package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func blocksForAddFunction(t *testing.T) Blocks {
	t.Helper()

	query, err := regexp.Compile("add")
	if err != nil {
		t.Fatalf("compile query: %v", err)
	}

	src := filepath.Join(t.TempDir(), "function.go")
	if err := os.WriteFile(src, []byte("package main\n\nfunc add() {}\n"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	blocks, err := findBlocks(src, query)
	if err != nil {
		t.Fatalf("find blocks: %v", err)
	}
	if len(blocks) == 0 {
		t.Fatalf("no blocks found in add function fixture")
	}
	return blocks
}

// failingConsumer writes a stub that exits non-zero so Stream can prove a
// failed consumer surfaces as an error instead of being swallowed.
func failingConsumer(t *testing.T) string {
	t.Helper()

	stub := filepath.Join(t.TempDir(), "reject-consumer")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
		t.Fatalf("write consumer stub: %v", err)
	}
	return stub
}

// A stream consumer exiting non-zero must surface as an error from Stream so
// main exits non-zero, instead of being swallowed while the consumer's stderr
// already leaked to the user.
func TestStreamReportsConsumerExitFailure(t *testing.T) {
	blocks := blocksForAddFunction(t)

	err := blocks.Stream(failingConsumer(t), "function.go", true)
	if err == nil {
		t.Fatalf("Stream: err = nil, want non-nil when the consumer exits non-zero")
	}
	if !strings.Contains(err.Error(), "stream block to consumer") {
		t.Fatalf("Stream: err = %q, want a stream-block-to-consumer error", err)
	}
}

// A stream command that cannot start must also surface as an error rather
// than being swallowed.
func TestStreamReportsMissingCommand(t *testing.T) {
	blocks := blocksForAddFunction(t)

	err := blocks.Stream("/no/such/blocksearch-consumer", "function.go", true)
	if err == nil {
		t.Fatalf("Stream: err = nil, want non-nil when the consumer cannot start")
	}
}
