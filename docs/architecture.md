# Architecture

## Block resolution

A match resolves to the smallest named tree-sitter node covering it,
then is promoted to the construct it belongs to, in three stages:

1. A token reached through a naming field (`name`, `function`, `key`)
   resolves to the construct that field names.
2. Any other single-line token resolves to the smallest enclosing
   statement or declaration. A match already covering a multiline node
   keeps that node as its block.
3. A named construct wrapped by `decorated_definition` climbs to the
   wrapper so decorators are included.

## Parsing and hashing pitfalls

- Matching MUST run before parsing: the regexp runs on the normalized
  file and returns immediately when there is no match, so sparse searches
  never pay for tree-sitter on most files. Regex offsets, parsed positions,
  and metadata MUST refer to the same BOM/CR-normalized contents.
- `namedDescendantForByteRange` binary-searches the unique covering child
  for non-empty matches, but empty (zero-width) matches can be covered by
  several same-position children (error-recovery nodes). Empty matches
  MUST keep the original left-to-right linear scan; output for patterns
  like `^` depends on it.
- Line hashes MUST remain lazy (`lazyHashlineHashes`): hashing every line
  costs about three full-file scans, while only emitted block lines need
  hashes. `hashlineNormalizeFileBytes` MUST remain copy-free for files
  without BOM/CR, and it and `hashlineNormalizeFile` MUST share one
  normalization implementation so tests pin one behavior.
- tree-sitter positions a construct whose source ends in a trailing
  newline at column 0 of the *next* line, so block extraction drops
  that phantom line. The column-0 decrement in `parse.go` MUST remain;
  it is not an off-by-one bug.

## Parse policy

`ParseOptions` MUST carry metadata, overlap, and resource-limit policy
through both tree-sitter and fallback parsing. Optional metadata MUST
remain aligned with the emitted block after overlap handling; parser
policy MUST NOT depend on emission state or read CLI flags directly.

## Ordered emission and lifecycle

`Search.Run` walks serially and hands each file a sequence number to a
pool of `GOMAXPROCS` workers that only read/parse/match/filter. A single
emitter reassembles results by sequence number. Output writes and consumer
state MUST remain exclusive to that emitter; worker completion order MUST
NOT change output order. The in-flight window MUST bound queued, active, and
pending out-of-order files, not merely the worker count, so a slow early
file cannot cause unbounded buffering.

Fatal errors MUST cancel outstanding work and unblock pipeline sends and
receives so shutdown can finish. NDJSON MUST be encoded and written
incrementally rather than buffered for the whole result set. The ordered
emitter MUST own the persistent consumer's input and shutdown lifecycle,
including closing input and waiting for exit; it MUST NOT launch that
consumer once per block.

Normal CLI stdout uses a temporary pollable descriptor on Unix. Color
selection MUST precede that setup; while it is active, the emitter MUST
exclusively own stdout and its aliases. Calling `Fd` can restore blocking
mode and defeat cancellation. Stream commands MUST inherit the original
stdout instead; their process-group lifecycle handles blocked child I/O.

## Walking and filters

`--include`/`--exclude` patterns are globs (doublestar), not regexes.
A pattern without `/` matches the basename; with `/` it matches the
root-relative, forward-slashed path. The walk root MUST be set per `Walk`
call; matching MUST use `filepath.Rel(root, path)` so globs like `src/**`
work regardless of how the root was named. `newFileWalkerForCLI` MUST
remain the environment boundary for global gitignore configuration;
tests MUST inject matchers rather than depend on `HOME`.

`<file>` arguments may be globs; they are expanded with doublestar and
every match MUST still flow through `processFileIfAllowed`, so globs
cannot bypass include/exclude or gitignore. A zero-match glob MUST be an
error, like a missing literal path. Matches MUST be visited in sorted
order (doublestar and `os.ReadDir` both sort); arguments MUST be processed
in argv order with no cross-argument dedup, matching how repeated explicit
paths already behave.
