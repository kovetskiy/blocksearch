# A query hitting the function declaration extracts the entire node,
# body and closing brace included (function.go lines 3-5): exactly one
# block with the full text and no stderr noise.

tests:eval blocksearch:run --json 'func add' "$TESTDATA_DIR/function.go"
tests:assert-success
tests:assert-stderr-empty

expected=$(
	cat <<EOF
{"filename":"$TESTDATA_DIR/function.go","line_start":3,"line_end":5,"text":"func add(a int, b int) int {\\n\\treturn a + b\\n}","line_hashes":["7F","D2","7B"]}
EOF
)
tests:assert-no-diff "$expected" "$(cat "$(tests:get-stdout-file)")"

blocksearch:assert-block-count 1
