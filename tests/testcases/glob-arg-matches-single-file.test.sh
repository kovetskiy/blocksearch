# A glob matching exactly one file works and produces exactly one block
# (function.go holds `func add`). Also guards against crashes and bad
# ordering for a single match.

tests:eval blocksearch:run --json 'func add' "$TESTDATA_DIR/fu*.go"
tests:assert-success
tests:assert-stderr-empty
tests:assert-stdout-re '"filename":"'"$TESTDATA_DIR"'/function.go"'
tests:assert-stdout-re '"line_start":3,"line_end":5'

blocksearch:assert-block-count 1
