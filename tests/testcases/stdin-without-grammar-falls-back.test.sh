# Piped stdin without a shebang falls back to indentation-based block
# extraction, so a match still produces output instead of an error.

tests:put stdin <<'EOF'
package main
func Needle() {}
EOF

tests:eval blocksearch:run --json Needle <"$(tests:get-tmp-dir)/stdin"
tests:assert-success
tests:assert-stdout-re 'Needle'
