# A ** glob recurses through the testdata tree and still resolves the .go
# files under src/.

tests:eval blocksearch:run --json 'func' "$TESTDATA_DIR/../**/*.go"
tests:assert-success
tests:assert-stderr-empty
tests:assert-stdout-re '"filename":"'"$TESTDATA_DIR"'/function.go"'
