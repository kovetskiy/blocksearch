# @file setup.sh
# @brief Shared setup for the blocksearch test suite.
# @description
#   Builds the binary once per session into the temp tree and exports
#   BLOCKSEARCH_BIN so testcases can invoke it without rebuilding. The
#   repo path arrives via BLOCKSEARCH_REPO_DIR (see run_tests).

:ensure-binary() {
	local build_dir="$BLOCKSEARCH_BUILD_DIR"
	local binary="$build_dir/blocksearch"
	local build_log="$build_dir/build.log"

	export BLOCKSEARCH_BIN="$binary"
	if [[ -x "$binary" ]]; then
		return
	fi

	(
		cd "$BLOCKSEARCH_REPO_DIR" || exit 1
		CGO_ENABLED=1 go build -o "$binary" ./cmd/blocksearch 2>"$build_log"
	) || {
		printf 'blocksearch: build failed; build log:\n' >&2
		cat "$build_log" >&2
		exit 1
	}
}

:ensure-binary

# @description Invoke the built blocksearch CLI with the given
# arguments. Testcases capture stdout, stderr, and exit code through
# the tests.sh assertion helpers.
#
# @arg $@ string Arguments to forward to blocksearch.
#
# @exitcode 0 If the CLI exits successfully.
# @exitcode >0 If the CLI fails.
blocksearch:run() {
	"$BLOCKSEARCH_BIN" "$@"
}

# @description Assert that the last captured JSON output contains the
# expected number of blocks. Blocksearch emits one JSON block per line.
#
# @arg $1 integer Expected block count.
#
# @exitcode 0 If the captured block count matches the expected count.
# @exitcode >0 If the counts differ.
blocksearch:assert-block-count() {
	local expected_count="$1"
	local block_count

	block_count=$(($(wc -l <"$(tests:get-stdout-file)")))
	tests:assert-equals "$expected_count" "$block_count"
}
