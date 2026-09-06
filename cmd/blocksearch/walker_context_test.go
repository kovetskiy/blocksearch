package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type cancelingIgnoreMatcher struct {
	cancel     context.CancelFunc
	fileChecks int
}

func (matcher *cancelingIgnoreMatcher) Match(path string, isDir bool) bool {
	if isDir {
		return false
	}
	matcher.fileChecks++
	matcher.cancel()
	return true
}

func TestWalkerCancellationIncludesFilteredFiles(t *testing.T) {
	root := t.TempDir()
	for index := 0; index < 10; index++ {
		path := filepath.Join(root, fmt.Sprintf("%02d.txt", index))
		if err := os.WriteFile(path, []byte("hit"), 0600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	matcher := &cancelingIgnoreMatcher{cancel: cancel}
	walker := NewFileWalker(root, nil, nil)
	walker.SetGlobalIgnore(matcher)
	err := walker.WalkContext(ctx, root, func(string) error {
		t.Errorf("ignored file reached search callback")
		return nil
	})
	if !errors.Is(err, context.Canceled) || matcher.fileChecks != 1 {
		t.Fatalf("walk = %v, checks = %d; want cancellation before next filtered entry", err, matcher.fileChecks)
	}
}

func TestWalkerPreCanceledSkipsFilesystem(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	walker := NewFileWalker(t.TempDir(), nil, nil)
	err := walker.WalkContext(ctx, "/no/such/blocksearch/path", func(string) error {
		t.Errorf("canceled walk invoked callback")
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("walk = %v, want cancellation rather than a filesystem error", err)
	}
}
