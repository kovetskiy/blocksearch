package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCancellableIOAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	file, err := openSearchInput(ctx, filepath.Join(t.TempDir(), "missing"))
	if file != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("open canceled = %v, %v", file, err)
	}
	output, cleanup, err := prepareCLIOutput(ctx, nil)
	if output != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("prepare canceled = %v, %v", output, err)
	}
	cleanup()
}

func TestCancellableIORegularInputAndMissingPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.txt")
	want := "\xef\xbb\xbfhit\r\nunchanged bytes\x00"
	if err := os.WriteFile(path, []byte(want), 0600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	file, err := openSearchInput(context.Background(), path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer file.Close()
	got, err := io.ReadAll(file)
	if err != nil || string(got) != want {
		t.Fatalf("read = %q, %v; want %q", got, err, want)
	}
	file, err = openSearchInput(context.Background(), path+".missing")
	var pathErr *os.PathError
	if file != nil || !errors.Is(err, os.ErrNotExist) || !errors.As(err, &pathErr) || pathErr.Path != path+".missing" {
		t.Fatalf("missing input = %v, %v", file, err)
	}
}

func TestCancellableIORegularOutputOffsetAndOwnership(t *testing.T) {
	for _, appendMode := range []bool{false, true} {
		name := "seek"
		if appendMode {
			name = "append"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "output.txt")
			if err := os.WriteFile(path, []byte("abcdef"), 0600); err != nil {
				t.Fatalf("create: %v", err)
			}
			flags := os.O_WRONLY
			if appendMode {
				flags |= os.O_APPEND
			}
			original, err := os.OpenFile(path, flags, 0)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer original.Close()
			if _, err := original.Seek(2, io.SeekStart); err != nil {
				t.Fatalf("seek: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			output, cleanup, err := prepareCLIOutput(ctx, original)
			if err != nil {
				t.Fatalf("prepare: %v", err)
			}
			defer cleanup()
			if output != original {
				t.Fatalf("regular output unnecessarily replaced")
			}
			if _, err := output.WriteString("XY"); err != nil {
				t.Fatalf("write: %v", err)
			}
			cancel()
			cleanup()
			cleanup()
			if _, err := original.WriteString("Z"); err != nil {
				t.Fatalf("original closed or offset lost: %v", err)
			}
			got, err := os.ReadFile(path)
			want := "abXYZf"
			if appendMode {
				want = "abcdefXYZ"
			}
			if err != nil || string(got) != want {
				t.Fatalf("output = %q, %v; want %q", got, err, want)
			}
		})
	}
}

func TestCancellableIORejectsInvalidOutput(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "closed")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	file.Close()
	for _, original := range []*os.File{nil, file} {
		output, cleanup, err := prepareCLIOutput(context.Background(), original)
		if output != nil || err == nil {
			t.Fatalf("invalid output = %v, %v", output, err)
		}
		cleanup()
	}
}
