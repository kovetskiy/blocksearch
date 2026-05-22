# A quoted glob argument is expanded by blocksearch (not the shell) and
# every matched file is searched: multiple .go files under TESTDATA_DIR all
# contain a func declaration, so the result spans files.

tests:eval blocksearch:run --json 'func' "$TESTDATA_DIR/*.go"
tests:assert-success
tests:assert-stderr-empty
tests:assert-stdout-re '"filename":"'"$TESTDATA_DIR"'/function.go"'
tests:assert-stdout-re '"filename":"'"$TESTDATA_DIR"'/method.go"'

blocks=$(($(wc -l <"$(tests:get-stdout-file)")))
[ "$blocks" -ge 2 ] || tests:fail "expected at least 2 blocks, got $blocks"
