# Smoke test: the built binary runs and prints usage.

tests:eval blocksearch:run --help
tests:assert-success
tests:assert-stdout-re 'blocksearch'
tests:assert-stderr-empty
