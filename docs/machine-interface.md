# Machine interface

This contract describes v7.0, following the v6.0 release tag. Earlier
CLI builds reported a stale v1.5.0 string; historical probe references
below retain that reported version.

Read this document when integrating a wrapper or changing CLI parsing,
extraction, emission, or diagnostics. The docopt usage and this contract
MUST stay in sync.

## Output selection

`--json` (`-j`) emits one JSON object followed by LF per block (NDJSON).
Records are encoded and written individually, not accumulated as a complete
encoded file. Default text output and the default JSON block schema remain
unchanged. `--hashline` (`-H`) enables anchors in text and `line_hashes` in
JSON; `--no-line` or `--file` disables anchors.

`--files` (`-L`) emits each matching filename once per input-file visit.
The default separator is LF and is unsafe for filenames containing LF.
`--files --null` (`-L -0`) instead emits raw filename bytes followed by NUL,
preserving even non-UTF-8 bytes. `--files --json` emits NDJSON objects:

```json
{"filename":"a\nb\tc\\d.txt"}
```

JSON round-trips UTF-8 filenames, including LF, tabs, and backslashes.
Invalid UTF-8 bytes are replaced by the JSON encoder; use NUL output when
byte-exact filename preservation is required.

The following combinations are rejected as argument errors:

- `--null` without `--files`, or combined with `--json`.
- `--files` combined with `--matches` or either streaming mode.
- Both streaming modes together.

Streaming takes precedence over `--json`. `--matches` implies JSON and
also enriches records sent to either streaming mode. Color and classic
text-format flags do not turn JSON or streaming back into text.

## Exact matches

`--matches` adds `matches` to each block object. Without it, this field is
absent. For a file containing `xx hit yy hit`:

```json
{"filename":"one.txt","line_start":1,"line_end":1,"text":"xx hit yy hit","matches":[{"byte_start":3,"byte_end":6,"line_start":1,"line_end":1,"column_start":4,"column_end":7},{"byte_start":10,"byte_end":13,"line_start":1,"line_end":1,"column_start":11,"column_end":14}]}
```

Match offsets refer to the entire normalized file, not the exported block
text. Normalization removes a leading UTF-8 BOM and converts both CRLF
and standalone CR to LF. The regexp and tree-sitter receive these same
bytes. Offsets cannot be applied directly to unnormalized files.

- `byte_start` is zero-based and inclusive; `byte_end` is exclusive.
- Lines and columns are one-based. Columns count bytes, not Unicode code
  points, UTF-16 units, or display cells; tabs count as one byte.
- End coordinates identify the exclusive end position. An end just after
  LF is column 1 on the next line, including the empty EOF line.
- Empty matches have equal start/end offsets and coordinates.
- Multiline spans are one match, not separate per-line entries.

The regexp engine is Go's `regexp`, with multiline anchors enabled.
Matches follow its non-overlapping `FindAllIndex` semantics, including its
suppression of empty matches adjacent to a preceding match. `--literal`
quotes regexp metacharacters and uses precisely the same metadata schema.

Exact duplicate block line ranges merge all causal matches in regexp
order, including repeated hits on the same line. Metadata is collected
while resolving the original query, never by searching block text again.
A retained block lists only matches that resolved to that range, not every
match that happens to lie inside its text. When no grammar is available,
indentation fallback still anchors extraction on the match's first line.
A multiline causal match can extend beyond that fallback block; its
metadata nevertheless reports the complete original match span.

## Overlap and ordering

`--overlap=all|outermost|innermost` defaults to `all`. Policies operate on
inclusive block line ranges within one file visit, after exact-range
merging and before AWK filtering:

- `all` retains every distinct range.
- `outermost` drops ranges fully contained by another distinct range.
- `innermost` drops ranges that fully contain another distinct range.

Partial overlaps survive both policies. Equal ranges were already merged.
Suppressed blocks do not donate matches to retained blocks. A multiline
match crossing constructs can resolve to a common parent; overlap policy
neither splits that span nor invents smaller blocks. AWK cannot restore a
range suppressed by overlap selection.

Files emit in walk order despite parallel searching; retained blocks emit
in first-causal-match order, not size order. Input arguments retain argv
order, glob matches and directory entries are sorted, and repeated paths
in separate arguments are not deduplicated. Containment never crosses
separate visits to a file.

## Resource limits

All limits accept nonnegative decimal integers; zero means unlimited.
Crossing a positive bound fails explicitly, never silently truncates a
block, match array, or record. Limits do not impose a wall-clock timeout.

| Flag | Scope |
| --- | --- |
| `--max-file-bytes` | Raw input bytes per file, before BOM/CR normalization or binary detection |
| `--max-matches` | Regexp matches per file, before exact-range merging |
| `--max-blocks` | Distinct block ranges per file, before overlap/AWK filtering |
| `--max-block-lines` | Lines in each candidate block, before filtering |
| `--max-block-bytes` | UTF-8/text bytes of each normalized block, with internal LF separators, without a final separator |
| `--max-output-bytes` | Total encoded/formatted bytes emitted or fed to consumers across the search |

A file read and regexp match collection probe only one item beyond their
configured bound. Candidate line/byte limits are checked before building
block text. Tree-sitter additionally cannot accept inputs larger than its
32-bit byte-position API. Set all relevant bounds when processing
untrusted inputs: a count bound alone does not bound input or record size.

Output counting includes filenames, JSON escaping, hash arrays, metadata,
and record separators. It excludes stderr and a stream command's own
stdout/stderr. Each output record is checked before writing; earlier
records may already have been emitted. Consumers must enforce their own
output limits if needed. Encoding still allocates one record at a time;
file contents and candidate matches/blocks are retained during extraction.

The ordered pipeline admits at most `GOMAXPROCS` files until each file's
output is consumed. A slow earlier file or consumer therefore cannot grow
an unbounded reorder queue. Limits, output failures, and consumer failures
cancel the search; ordinary input errors permit later inputs to continue.
Cancellation is checked between regexp/filesystem operations and during
extraction, and propagated into tree-sitter and AWK computation. A single
regexp or filesystem operation is not a preemptible CPU-time budget. AWK
external commands and blocking I/O are subject to the interpreter's
cancellation limitations; AWK programs should be trusted. On Unix, waiting
for a FIFO writer, pollable input reads, and blocked stdout pipe/socket
writes are cancellable. The non-Unix I/O fallback cannot interrupt every
blocked open/write; use an external process supervisor there. Regular-file
kernel I/O and blocked stderr writes are not made preemptible.

## Stream consumers

`--stream <cmd>` (`-S`) retains the per-block interface: run `sh -c <cmd>`
once per block, with exactly one JSON document on stdin and no trailing
LF. Commands may contain arguments and shell syntax; they are not treated
as executable paths. Only trusted command strings should be supplied.
Unlike v1.5.0, failure to feed the complete document is now an error even
if the per-block command exits successfully. Commands that intentionally
ignore stdin (such as `true`) can therefore fail when the pipe closes
before writing finishes; use a command that drains stdin for this mode.

`--stream-persistent <cmd>` launches one `sh -c <cmd>` for the whole search
and feeds LF-terminated JSON records through its stdin. It also starts
once on a search with no matching blocks, then receives EOF. Writes are
synchronous and pipe backpressure is bounded. The command's stdout and
stderr are inherited. A nonzero exit or failure to feed stdin fails the
search and cancels outstanding work. On completion stdin closes and the
command is waited for; on cancellation it is terminated and reaped.

Consumers are expected to read until EOF and exit. A consumer that hangs
while keeping stdin open can stall the search; send SIGINT/SIGTERM or use
an external timeout. The per-block mode remains available for consumers
that expect EOF after each block.

## Diagnostics and partial results

Default diagnostics are human-readable stderr. `--diagnostics=json`
instead emits NDJSON diagnostics and a final completion object on stderr,
leaving the selected stdout format unchanged. `-v` is suppressed in this
mode; `--message` emits a JSON message record rather than plain text.
Stream commands inherit stderr and may write arbitrary non-JSON data;
redirect their stderr inside the command if a pure diagnostic channel is
required.

Examples:

```json
{"type":"diagnostic","kind":"input","path":"missing.txt","message":"missing.txt: ..."}
{"type":"completion","success":false,"results_partial":true,"exit_code":1}
```

Diagnostic `kind` is stable: `argument`, `query`, `input`, `limit`,
`output`, `stream`, or `canceled`. Each diagnostic has a human-readable
`message`; `path` or `query` identifies the affected input when available.
Limits add `resource` and `maximum`; stream failures add `command`.
Messages are not a parsing API. Limit resources are `file_bytes`,
`matches`, `blocks`, `block_lines`, `block_bytes`, and `output_bytes`.
Multiple failures can produce multiple diagnostics; their relative
ordering is not an API.

The final `completion` has `success`, `results_partial`, and `exit_code`.
A successful search sets `results_partial` to false, even if the explicit
`--exit-code` policy requests a nonzero exit when matches are found. Any
failure sets it to true: stdout is not certified exhaustive and may be
empty. There is no rollback of already emitted records and no strict
buffer-everything mode. Missing inputs and unmatched globs permit results
from other inputs. Limits/output/consumer failures stop production.

Argument validation and query compilation failures exit 2 before search;
search/input/limit/output/consumer failures exit 1. A successful run exits
0 unless matching blocks trigger `--exit-code`. This changes invalid-query
exit status from the probed v1.5.0 status 1 to 2. `--help` and `--version`
exit normally without completion records.

A killed process or unwritable stderr cannot guarantee completion delivery.
Wrappers MUST check process termination as well as completion; missing
completion means the result is not certified complete. A failed stdout
write can leave a torn final record. Persistent-consumer completion says
the command exited successfully, not that its downstream effects are
transactional or durable.
