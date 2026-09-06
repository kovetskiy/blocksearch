# Development

## Build and tests

The tree-sitter bindings require a C toolchain, so every build MUST
enable CGO:

    CGO_ENABLED=1 go build -o blocksearch ./cmd/blocksearch
    go test ./...        # Go unit and behavior tests
    ./tests/run_tests    # integration suite

## Shell test harness

The integration suite is vendored under `vendor.bash/`.
`tests/setup.sh` builds the binary once per session into a temp
tree and exports `BLOCKSEARCH_BIN`; testcases MUST call it through
`blocksearch:run`. A failed build MUST fail setup and print the
build log, rather than leaving a missing binary for testcases to trip
over with a confusing "no such file".

`tests:clone` (in the vendored test harness) prepends
`$_tests_base_dir/` to every source path. Setup, teardown, and
testcase-dir paths MUST be relative to the repo root, and the
runner MUST `cd` to the repo root before calling `test-runner:run`.
Absolute paths would produce double-prefixed paths under the temp
directory and fail silently. The harness requires GNU `readlink` with
`-f` support.

## Shell documentation

`tests/setup.sh` is a sourced library that exposes the
`blocksearch:run` helper. It MUST carry file-level shdoc
annotations (`@file`, `@brief`, `@description`) and function-level
annotations (`@description`, `@arg`, `@exitcode`) so `shdoc` can
render the contract. Before finishing shell docs work, MUST run
`shdoc tests/setup.sh` and inspect the output.

## Shell testcase conventions

Testcase files under `tests/testcases/` are Bash fragments
executed by the `tests.sh` runtime. They do not need a shebang,
`set -euo pipefail`, or `# shellcheck` directives: the runner
owns the shell dialect.

When counting output lines, MUST wrap `wc -l` in arithmetic expansion
to strip leading whitespace portably:

    blocks=$(($(wc -l <"$(tests:get-stdout-file)")))
    tests:assert-equals 1 "$blocks"

`wc -l` pads with spaces on macOS/BSD; arithmetic expansion
normalizes the value regardless of platform.
