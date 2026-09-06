package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMachineCLIProcess(t *testing.T) {
	if os.Getenv("BLOCKSEARCH_CLI_TEST") != "1" {
		return
	}
	for index, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"blocksearch"}, os.Args[index+1:]...)
			main()
			return
		}
	}
	t.Fatalf("missing helper argument separator")
}

func runMachineCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestMachineCLIProcess$", "--"}, args...)...)
	command.Env = append(os.Environ(), "BLOCKSEARCH_CLI_TEST=1", "HOME="+t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if ctx.Err() != nil {
		t.Fatalf("CLI timed out: %v; stderr: %s", args, stderr.String())
	}
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return stdout.String(), stderr.String(), exit.ExitCode()
		}
		t.Fatalf("run CLI: %v", err)
	}
	return stdout.String(), stderr.String(), 0
}

func machineRecords(t *testing.T, text string) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("invalid NDJSON %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

func TestMachineCLIOutputOptions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a\nb\tc\\d.txt")
	if err := os.WriteFile(path, []byte("xx hit yy hit\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	for _, testcase := range []struct {
		name  string
		flags []string
	}{
		{"default", []string{"-j"}},
		{"matches", []string{"--matches"}},
		{"json files", []string{"-j", "-L"}},
		{"null files", []string{"-0", "-L"}},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			stdout, stderr, code := runMachineCLI(t, append(testcase.flags, "hit", path)...)
			if code != 0 || stderr != "" {
				t.Fatalf("exit = %d, stderr = %q", code, stderr)
			}
			if testcase.name == "null files" {
				if stdout != path+"\x00" {
					t.Fatalf("NUL filenames = %q", stdout)
				}
				return
			}
			records := machineRecords(t, stdout)
			if len(records) != 1 || records[0]["filename"] != path {
				t.Fatalf("records = %#v", records)
			}
			record := records[0]
			if testcase.name == "json files" {
				if len(record) != 1 {
					t.Fatalf("files record = %#v", record)
				}
			} else if testcase.name == "matches" {
				matches, ok := record["matches"].([]any)
				if !ok || len(matches) != 2 {
					t.Fatalf("matches = %#v", record["matches"])
				}
			} else if _, exists := record["matches"]; exists {
				t.Fatalf("default schema contains matches: %#v", record)
			}
		})
	}
}

func TestMachineCLIRejectsConflictsAndInvalidLimits(t *testing.T) {
	for _, args := range [][]string{
		{"--null"}, {"--null", "--files", "--json"},
		{"--files", "--matches"}, {"--files", "--stream", "cat"},
		{"--files", "--stream-persistent", "cat"},
		{"--stream", "cat", "--stream-persistent", "cat"},
		{"--overlap", "arbitrary"}, {"--max-file-bytes", "-1"},
		{"--max-block-lines", "-1"}, {"--max-block-bytes", "-1"},
		{"--max-matches", "-1"}, {"--max-blocks", "-1"},
		{"--max-output-bytes", "-1"}, {"--max-matches", "abc"},
		{"--max-file-bytes", "99999999999999999999999999"},
		{"--unknown-option"}, {"--overlap="}, {"--stream="}, {"--stream-persistent="},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			argv := append([]string{"--diagnostics=json"}, args...)
			stdout, stderr, code := runMachineCLI(t, append(argv, "hit", "unused")...)
			if code != 2 || stdout != "" {
				t.Fatalf("exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
			}
			records := machineRecords(t, stderr)
			if len(records) != 2 || records[0]["kind"] != "argument" || records[1]["results_partial"] != true {
				t.Fatalf("diagnostics = %#v", records)
			}
		})
	}
}

func TestMachineCLIDiagnosticsAndCompletion(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "good.txt")
	missing := filepath.Join(root, "missing.txt")
	if err := os.WriteFile(good, []byte("hit\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	for _, testcase := range []struct {
		name    string
		args    []string
		code    int
		kind    string
		partial bool
		output  bool
	}{
		{"success", []string{"hit", good}, 0, "", false, true},
		{"input", []string{"hit", missing, good}, 1, "input", true, true},
		{"glob", []string{"hit", filepath.Join(root, "*.none"), good}, 1, "input", true, true},
		{"query", []string{"[", good}, 2, "query", true, false},
		{"literal", []string{"--literal", "[", good}, 0, "", false, false},
		{"limit", []string{"--max-file-bytes=1", "hit", good}, 1, "limit", true, false},
		{"output limit", []string{"--max-output-bytes=1", "hit", good}, 1, "limit", true, false},
		{"stream", []string{"--stream-persistent", "exit 7", "hit", good}, 1, "stream", true, false},
		{"exit policy", []string{"--exit-code=7", "hit", good}, 7, "", false, true},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			stdout, stderr, code := runMachineCLI(t, append([]string{"--diagnostics=json", "-j"}, testcase.args...)...)
			if code != testcase.code || (stdout != "") != testcase.output {
				t.Fatalf("exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
			}
			records := machineRecords(t, stderr)
			if len(records) == 0 {
				t.Fatalf("missing completion")
			}
			completion := records[len(records)-1]
			if completion["type"] != "completion" || completion["results_partial"] != testcase.partial || completion["success"] != !testcase.partial || completion["exit_code"] != float64(testcase.code) {
				t.Fatalf("completion = %#v", completion)
			}
			if testcase.kind != "" {
				if records[0]["kind"] != testcase.kind {
					t.Fatalf("diagnostic kind = %#v, want %q", records, testcase.kind)
				}
				if testcase.kind == "query" && records[0]["query"] != "[" {
					t.Fatalf("query diagnostic = %#v", records[0])
				}
				if testcase.name == "input" && records[0]["path"] != missing {
					t.Fatalf("path diagnostic = %#v", records[0])
				}
			}
		})
	}
}

func TestRequestsJSONDiagnosticsSkipsValuesAndTerminator(t *testing.T) {
	for _, args := range [][]string{
		{"--", "--diagnostics=json"},
		{"--stream", "--diagnostics=json", "hit"},
		{"--message", "--diagnostics=json", "hit"},
	} {
		if requestsJSONDiagnostics(args) {
			t.Errorf("request inferred from value: %q", args)
		}
	}
}

func TestMachineCLIVersionAndHelp(t *testing.T) {
	stdout, stderr, code := runMachineCLI(t, "--version")
	if code != 0 || stderr != "" || stdout != version+"\n" {
		t.Fatalf("version: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	stdout, stderr, code = runMachineCLI(t, "--help")
	if code != 0 || stderr != "" || !strings.HasPrefix(stdout, "blocksearch "+version+"\n") {
		t.Fatalf("help: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestMachineCLIInvalidUTF8Input(t *testing.T) {
	bad := writeBlockFixture(t, "bad.txt", []byte(strings.Repeat("\xff", 4999)+"TARGET\n"))
	good := writeBlockFixture(t, "good.txt", []byte("� TARGET\n"))
	for _, flags := range [][]string{
		{}, {"--json"}, {"--matches"}, {"--matches", "--literal"},
		{"--files"}, {"--files", "--null"}, {"--files", "--json"},
		{"--matches", "--stream", "cat"}, {"--matches", "--stream-persistent", "cat"},
	} {
		t.Run(strings.Join(flags, " "), func(t *testing.T) {
			args := append([]string{"--diagnostics=json"}, flags...)
			stdout, stderr, code := runMachineCLI(t, append(args, "TARGET", bad)...)
			if code != 1 || stdout != "" {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			records := machineRecords(t, stderr)
			if len(records) != 2 || records[0]["kind"] != "input" || records[0]["path"] != bad || !strings.Contains(records[0]["message"].(string), "invalid UTF-8") {
				t.Fatalf("diagnostics = %#v", records)
			}
			completion := records[1]
			if completion["type"] != "completion" || completion["success"] != false || completion["results_partial"] != true || completion["exit_code"] != float64(1) {
				t.Fatalf("completion = %#v", completion)
			}
		})
	}
	stdout, stderr, code := runMachineCLI(t, "--matches", "--diagnostics=json", "TARGET", bad, good)
	if code != 1 {
		t.Fatalf("mixed inputs: exit=%d stderr=%q", code, stderr)
	}
	records := machineRecords(t, stdout)
	if len(records) != 1 || records[0]["filename"] != good || records[0]["text"] != "� TARGET" {
		t.Fatalf("mixed input records = %#v", records)
	}
	want := []Match{{ByteStart: 4, ByteEnd: 10, LineStart: 1, LineEnd: 1, ColumnStart: 5, ColumnEnd: 11}}
	var record struct {
		Matches []Match `json:"matches"`
	}
	if err := json.Unmarshal([]byte(stdout), &record); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(record.Matches) != 1 || record.Matches[0] != want[0] {
		t.Fatalf("replacement character coordinates = %+v, want %+v", record.Matches, want)
	}
	diagnostics := machineRecords(t, stderr)
	if len(diagnostics) != 2 || diagnostics[0]["kind"] != "input" || diagnostics[0]["path"] != bad || diagnostics[1]["success"] != false || diagnostics[1]["results_partial"] != true || diagnostics[1]["exit_code"] != float64(1) {
		t.Fatalf("mixed input diagnostics = %#v", diagnostics)
	}
	stdout, stderr, code = runMachineCLI(t, "--matches", "TARGET", bad)
	if code != 1 || stdout != "" || !strings.Contains(stderr, bad) || !strings.Contains(stderr, "invalid UTF-8") {
		t.Fatalf("text diagnostic: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}
