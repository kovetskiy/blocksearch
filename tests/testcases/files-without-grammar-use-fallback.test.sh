# Files without a tree-sitter grammar use indentation-based block extraction.

tests:eval blocksearch:run --json 'build:' "$TESTDATA_DIR/Makefile"
tests:assert-success
tests:assert-stderr-empty

expected=$(
	cat <<EOF
{"filename":"$TESTDATA_DIR/Makefile","line_start":10,"line_end":11,"text":"build:\\n\\t\$(CC) \$(CFLAGS) -o app main.c utils.c","line_hashes":["D9","B5"]}
EOF
)
tests:assert-no-diff "$expected" "$(cat "$(tests:get-stdout-file)")"

blocksearch:assert-block-count 1
