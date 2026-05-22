# Regression: matching the bare function name `foobar` must resolve to
# the whole function node across its multi-line parameter list and body
# (multiline.go lines 8-14), not just the signature line. This is the
# multi-line param-list case the tree-sitter extraction exists for.
#
# `foobar` also appears inside panic("foobar"). That match resolves to a
# distinct node (the call expression on line 13), so it is kept as a
# second block rather than suppressed — containment alone does not drop
# a separate match.

tests:eval blocksearch:run --json 'foobar' "$TESTDATA_DIR/multiline.go"
tests:assert-success
tests:assert-stderr-empty
tests:assert-stdout-re '"line_start":8,"line_end":14'
tests:assert-stdout-re '"line_start":13,"line_end":13'

blocksearch:assert-block-count 2
