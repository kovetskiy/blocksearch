# ^ in a query anchors to the start of every line (grep-like), not just
# byte 0 of the file. Without line anchoring, "^def " on utils.py would
# match nothing because the file's first line is a comment; with it,
# every top-level def (lines 3, 7, 11) is found. Indented methods inside
# the class are excluded since ^ requires the match at column 0.

tests:eval blocksearch:run --json '^def ' "$TESTDATA_DIR/utils.py"
tests:assert-success
tests:assert-stderr-empty

blocksearch:assert-block-count 3

tests:assert-stdout-re '"line_start":3,'
tests:assert-stdout-re '"line_start":7,'
tests:assert-stdout-re '"line_start":11,'
