# --version prints the program version and exits 0. It must not be silent
# (the docopt call must receive a non-empty version string).

tests:eval blocksearch:run --version
tests:assert-success
tests:assert-stdout-re 'v[0-9]+\.[0-9]'
