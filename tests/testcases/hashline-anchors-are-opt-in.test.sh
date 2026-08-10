# Hashline anchors are opt-in: default text output uses the plain
# LINE:text format and default JSON omits line_hashes; -H/--hashline
# switches text output to the LINE#HASH│ anchor format.

tests:eval blocksearch:run 'func add' "$TESTDATA_DIR/function.go"
tests:assert-success

expected=$(
	cat <<EOF
$TESTDATA_DIR/function.go
3:func add(a int, b int) int {
4:	return a + b
5:}
EOF
)
tests:assert-no-diff "$expected" "$(cat "$(tests:get-stdout-file)")"

tests:eval blocksearch:run --json 'func add' "$TESTDATA_DIR/function.go"
tests:assert-success

expected=$(
	cat <<EOF
{"filename":"$TESTDATA_DIR/function.go","line_start":3,"line_end":5,"text":"func add(a int, b int) int {\\n\\treturn a + b\\n}"}
EOF
)
tests:assert-no-diff "$expected" "$(cat "$(tests:get-stdout-file)")"

tests:eval blocksearch:run -H 'func add' "$TESTDATA_DIR/function.go"
tests:assert-success
short_flags="$(cat "$(tests:get-stdout-file)")"

tests:eval blocksearch:run --hashline 'func add' "$TESTDATA_DIR/function.go"
tests:assert-success
tests:assert-no-diff "$short_flags" "$(cat "$(tests:get-stdout-file)")"

expected=$(
	cat <<EOF
$TESTDATA_DIR/function.go
3#7F│func add(a int, b int) int {
4#D2│	return a + b
5#7B│}
EOF
)
tests:assert-no-diff "$expected" "$short_flags"
