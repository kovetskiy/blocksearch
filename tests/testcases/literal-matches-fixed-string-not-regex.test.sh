# --literal treats the query as a fixed string: `(a int` is an unbalanced
# group (a malformed regex that the regex path rejects), but the literal
# path matches it verbatim in function.go line 3, inside the `add` signature,
# which resolves to the whole function (lines 3-5).

tests:eval blocksearch:run --json --literal '(a int' "$TESTDATA_DIR/function.go"
tests:assert-success
tests:assert-stderr-empty
tests:assert-stdout-re '"line_start":3,"line_end":5'

# Contrast: without --literal the same query is a malformed regex and fails.
tests:eval blocksearch:run --json '(a int' "$TESTDATA_DIR/function.go"
tests:assert-fail
