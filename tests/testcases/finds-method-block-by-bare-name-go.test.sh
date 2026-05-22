# Regression: matching a bare Go method name resolves to the whole method
# node (method.go Tally method spans lines 7-9), not just the signature
# line. The name token is a field_identifier with field "name"; without
# the field-based climb this collapsed to a single line. `Tally` is a
# distinct name that does not occur as a substring of any other token.

tests:eval blocksearch:run --json 'Tally' "$TESTDATA_DIR/method.go"
tests:assert-success
tests:assert-stderr-empty
tests:assert-stdout-re '"line_start":7,"line_end":9'
