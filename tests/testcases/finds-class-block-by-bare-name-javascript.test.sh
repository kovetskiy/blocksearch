# Regression: matching a bare class name in JavaScript resolves to the
# whole class node (app.js Server class spans lines 16-32), not just the
# declaration line. The name token is an identifier with field "name".

tests:eval blocksearch:run --json 'Server' "$TESTDATA_DIR/app.js"
tests:assert-success
tests:assert-stderr-empty
tests:assert-stdout-re '"line_start":16,"line_end":32'
