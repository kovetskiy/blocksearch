package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
)

type diagnosticFailure struct {
	kind  string
	path  string
	query string
	err   error
}

func diagnosticError(kind, path, query string, err error) error {
	return &diagnosticFailure{kind: kind, path: path, query: query, err: err}
}

func (e *diagnosticFailure) Error() string { return e.err.Error() }
func (e *diagnosticFailure) Unwrap() error { return e.err }

type diagnosticRecord struct {
	Type     string `json:"type"`
	Kind     string `json:"kind"`
	Path     string `json:"path,omitempty"`
	Query    string `json:"query,omitempty"`
	Message  string `json:"message"`
	Resource string `json:"resource,omitempty"`
	Maximum  int64  `json:"maximum,omitempty"`
	Command  string `json:"command,omitempty"`
}

type completionRecord struct {
	Type           string `json:"type"`
	Success        bool   `json:"success"`
	ResultsPartial bool   `json:"results_partial"`
	ExitCode       int    `json:"exit_code"`
}

func reportCompletion(writer io.Writer, jsonMode bool, err error, exitCode int) {
	if !jsonMode {
		if err != nil {
			fmt.Fprintf(writer, "blocksearch: %s\n", err)
		}
		return
	}
	encoder := json.NewEncoder(writer)
	for _, failure := range splitFailures(err) {
		_ = encoder.Encode(diagnosticForError(failure))
	}
	_ = encoder.Encode(completionRecord{
		Type: "completion", Success: err == nil, ResultsPartial: err != nil, ExitCode: exitCode,
	})
}

func reportMatchMessage(writer io.Writer, jsonMode bool, message string) {
	if jsonMode {
		_ = json.NewEncoder(writer).Encode(diagnosticRecord{Type: "message", Kind: "match", Message: message})
		return
	}
	fmt.Fprintln(writer, message)
}

func splitFailures(err error) []error {
	if err == nil {
		return nil
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var failures []error
		for _, child := range joined.Unwrap() {
			failures = append(failures, splitFailures(child)...)
		}
		return failures
	}
	return []error{err}
}

func diagnosticForError(err error) diagnosticRecord {
	record := diagnosticRecord{Type: "diagnostic", Kind: "input", Message: err.Error()}
	var failure *diagnosticFailure
	if errors.As(err, &failure) {
		record.Kind = failure.kind
		record.Path = failure.path
		record.Query = failure.query
		return record
	}
	var pathFailure *SearchPathError
	if errors.As(err, &pathFailure) {
		record.Path = pathFailure.Path
	}
	var fileFailure *os.PathError
	if errors.As(err, &fileFailure) {
		if record.Path == "" {
			record.Path = fileFailure.Path
		}
		if fileFailure.Op == "write" {
			record.Kind = "output"
		}
	}
	var limit *LimitError
	var stream *StreamError
	var output *OutputError
	switch {
	case errors.As(err, &limit):
		record.Kind = "limit"
		record.Resource = limit.Resource
		record.Maximum = limit.Maximum
	case errors.As(err, &stream):
		record.Kind = "stream"
		record.Command = stream.Command
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		record.Kind = "canceled"
	case errors.As(err, &output), errors.Is(err, io.ErrShortWrite), errors.Is(err, syscall.EPIPE):
		record.Kind = "output"
	}
	return record
}

func requestsJSONDiagnostics(argv []string) bool {
	requested := false
	for index := 0; index < len(argv); index++ {
		arg := argv[index]
		if arg == "--" {
			break
		}
		if strings.HasPrefix(arg, "--diagnostics=") {
			requested = arg == "--diagnostics=json"
			continue
		}
		if arg == "--diagnostics" && index+1 < len(argv) {
			index++
			requested = argv[index] == "json"
			continue
		}
		switch arg {
		case "--color", "--overlap", "--stream-persistent", "--max-file-bytes",
			"--max-block-lines", "--max-block-bytes", "--max-matches", "--max-blocks",
			"--max-output-bytes", "-S", "--stream", "-a", "--awk", "-e", "--exit-code",
			"--message", "-x", "--include", "-X", "--exclude":
			index++
		}
	}
	return requested
}
