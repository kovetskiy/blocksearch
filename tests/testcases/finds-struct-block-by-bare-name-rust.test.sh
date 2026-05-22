# Regression: matching a bare struct name in Rust resolves to the whole
# struct node (server.rs `pub struct Server` spans lines 5-7), not just
# the declaration line. The name token is a type_identifier with field
# "name".

tests:eval blocksearch:run --json 'Server' "$TESTDATA_DIR/server.rs"
tests:assert-success
tests:assert-stderr-empty
tests:assert-stdout-re '"line_start":5,"line_end":7'
