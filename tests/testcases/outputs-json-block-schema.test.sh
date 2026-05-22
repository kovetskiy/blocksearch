# JSON output is exactly one object per block with the documented fields.

tests:eval blocksearch:run --json 'def add' "$TESTDATA_DIR/utils.py"
tests:assert-success
tests:assert-stderr-empty

expected=$(
	cat <<EOF
{"filename":"$TESTDATA_DIR/utils.py","line_start":3,"line_end":4,"text":"def add(a, b):\\n  return a + b","line_hashes":["73","17"]}
EOF
)
tests:assert-no-diff "$expected" "$(cat "$(tests:get-stdout-file)")"

blocksearch:assert-block-count 1
