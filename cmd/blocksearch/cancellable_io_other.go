//go:build !unix

package main

import (
	"context"
	"os"
)

func openSearchInput(ctx context.Context, filename string) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.Open(filename)
	if canceled := ctx.Err(); canceled != nil {
		if file != nil {
			_ = file.Close()
		}
		return nil, canceled
	}
	return file, err
}

func prepareCLIOutput(ctx context.Context, output *os.File) (*os.File, func(), error) {
	noop := func() {}
	if err := ctx.Err(); err != nil {
		return nil, noop, err
	}
	if _, err := output.Stat(); err != nil {
		return nil, noop, err
	}
	return output, noop, nil
}
