# Grammar recognition tests for the checked-in language fixture matrix.
# Parameterized over (testdata_file, query, line_start, line_end) — each
# row runs blocksearch:run --json with the given query and file, then
# asserts success, empty stderr, and the expected line_start/line_end range.

while IFS='|' read -r file query line_start line_end; do
	tests:eval blocksearch:run --json "$query" "$TESTDATA_DIR/$file"
	tests:assert-success
	tests:assert-stderr-empty
	tests:assert-stdout-re '"line_start":'"$line_start"',"line_end":'"$line_end"
done <<'TABLE'
program.cs|ComputeTotal|11|15
styles.css|invoice-card|3|7
schema.cue|Invoice|4|4
Dockerfile|registry.example.com|4|4
worker.ex|process_invoice|9|11
Main.elm|greet name|6|6
build.gradle|generateBillingSources|10|10
main.tf|billing_archive|6|6
page.html|invoice-summary|9|9
App.kt|ReceiptPrinter|5|10
module.lua|compute_total|2|7
README.md|main.py|9|9
parser.ml|print_float|8|8
Service.php|computeTotal|7|11
api.proto|message Invoice|6|11
App.scala|computeTotal|5|8
schema.sql|CREATE TABLE invoices|2|8
Widget.svelte|function add|5|9
App.swift|computeTotal|8|11
config.toml|server.tls|10|12
config.yaml|database|16|24
TABLE
