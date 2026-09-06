package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"syscall"

	"github.com/docopt/docopt-go"
	"github.com/kovetskiy/lorg"
	"github.com/mattn/go-isatty"
	"github.com/monochromegane/go-gitignore"
	"github.com/reconquest/pkg/log"
)

var (
	version = "v7.0"
	usage   = "blocksearch " + version + `

Usage:
  blocksearch [options] [--] <query> [<file>...] [-a <if>]... [-x <include>]... [-X <exclude>]...
  blocksearch -h | --help
  blocksearch --version

Options:
  -t --file              Show filename before the line.
  -l --no-line           Do not show number of line before the line.
  --color <when>          When to colorize output: never, always, or auto. [default: auto]
  -j --json              Output blocks as incremental NDJSON; with --files, filename objects.
  --matches              Include causal match byte offsets and coordinates; implies --json.
  --overlap <policy>     Keep all, outermost, or innermost line ranges. [default: all]
  -0 --null              NUL-delimit --files output; incompatible with --json.
  --stream-persistent <cmd>  Run one shell command (sh -c), feeding NDJSON for the search.
  --max-file-bytes <n>   Limit raw input bytes per file (0 is unlimited). [default: 0]
  --max-block-lines <n>  Limit lines per block before filtering. [default: 0]
  --max-block-bytes <n>  Limit normalized text bytes per block. [default: 0]
  --max-matches <n>      Limit regex matches per file before deduplication. [default: 0]
  --max-blocks <n>       Limit deduplicated blocks per file before filtering. [default: 0]
  --max-output-bytes <n>  Limit bytes emitted or fed to consumers in total. [default: 0]
  --diagnostics <format>  Stderr diagnostics: text, or JSON with a completion record. [default: text]
  -H --hashline          Prefix lines with hashline edit anchors (LINE#HASH│). In JSON, fills line_hashes. Anchors are off by default.
  -S --stream <cmd>      Run <cmd> via the shell (sh -c) per block, piping its JSON to stdin. Enforces JSON.
  -a --awk <if>          Filter blocks by specified AWK condition.
  -e --exit-code <code>  Exit with the specified code if blocks were found. [default: 0]
  --message <warn>       Show the specified message if blocks were found.
  -L --files             Only print filenames that contain matching blocks.
  -x --include <pat>     Only search files whose path matches the glob. A pattern without a slash matches the basename; with a slash, the full path. Multiple allowed, OR'd.
  -X --exclude <pat>     Skip files whose path matches the glob. Same matching rule as --include. Multiple allowed, OR'd. Wins over include.
  -f --literal           Treat <query> as a literal string, not a regex.
  -v                     Be verbose.
  --version              Show version.
  -h --help              Show this screen.

Each <file> may be an existing file, an existing directory (searched
recursively), or a glob (e.g. *.go, src/**/*.go, {a,b}.py). With no
<file>, blocksearch searches . when stdin is a terminal and /dev/stdin
otherwise. A glob that matches nothing is an error.

--files cannot be combined with streaming or --matches; --null requires
--files. The stream modes are mutually exclusive and take precedence over
--json. --no-line or --file disables --hashline. Overlap selection precedes
AWK filtering; retained blocks stay in first-match order within walk order.
Failures may leave partial stdout. --diagnostics=json ends stderr with a
completion record (success and results_partial). See docs/machine-interface.md
for match normalization, resource scope, and diagnostic schemas.
`
)

type Arguments struct {
	FlagMatches                  bool     `docopt:"--matches"`
	FlagNull                     bool     `docopt:"--null"`
	ValueOverlap                 string   `docopt:"--overlap"`
	ValuePersistentStreamCommand string   `docopt:"--stream-persistent"`
	ValueMaxFileBytes            int64    `docopt:"--max-file-bytes"`
	ValueMaxBlockLines           int      `docopt:"--max-block-lines"`
	ValueMaxBlockBytes           int64    `docopt:"--max-block-bytes"`
	ValueMaxMatches              int      `docopt:"--max-matches"`
	ValueMaxBlocks               int      `docopt:"--max-blocks"`
	ValueMaxOutputBytes          int64    `docopt:"--max-output-bytes"`
	ValueDiagnostics             string   `docopt:"--diagnostics"`
	ValueStreamCommand           string   `docopt:"--stream"`
	ValueIncludes                []string `docopt:"--include"`
	ValueExcludes                []string `docopt:"--exclude"`
	ValueExitCode                int      `docopt:"--exit-code"`
	ValueAwkIfs                  []string `docopt:"--awk"`
	ValueMessage                 string   `docopt:"--message"`

	FlagShowFilenamePerLine bool   `docopt:"--file"`
	FlagNoLine              bool   `docopt:"--no-line"`
	ValueColor              string `docopt:"--color"`
	FlagJSON                bool   `docopt:"--json"`
	FlagHashline            bool   `docopt:"--hashline"`
	FlagVerbose             bool   `docopt:"-v"`
	FlagFilesOnly           bool   `docopt:"--files"`
	FlagLiteral             bool   `docopt:"--literal"`

	// EndOfOptions exists only so docopt.Bind accepts the literal `--`
	// terminator in argv; it is never read.
	EndOfOptions bool `docopt:"--"`

	ValueQuery string   `docopt:"<query>"`
	ValueFiles []string `docopt:"<file>"`
}

func main() {
	signal.Ignore(syscall.SIGPIPE)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	exitCode := runCLI(ctx, os.Args[1:])
	stop()
	os.Exit(exitCode)
}

func runCLI(ctx context.Context, argv []string) int {
	args, err := parseArgumentsArgs(argv)
	jsonDiagnostics := requestsJSONDiagnostics(argv)
	if err != nil {
		reportCompletion(os.Stderr, jsonDiagnostics, diagnosticError("argument", "", "", err), 2)
		return 2
	}
	jsonDiagnostics = args.ValueDiagnostics == "json"
	if args.FlagVerbose && !jsonDiagnostics {
		log.SetLevel(lorg.LevelDebug)
	}

	search, err := buildSearch(args)
	if err != nil {
		reportCompletion(os.Stderr, jsonDiagnostics, err, 2)
		return 2
	}

	emittedBlockCount, err := search.RunContext(ctx)
	if err != nil {
		reportCompletion(os.Stderr, jsonDiagnostics, err, 1)
		return 1
	}

	exitCode := 0
	if emittedBlockCount > 0 {
		exitCode = args.ValueExitCode
		if args.ValueMessage != "" {
			reportMatchMessage(os.Stderr, jsonDiagnostics, args.ValueMessage)
		}
	}
	reportCompletion(os.Stderr, jsonDiagnostics, nil, exitCode)
	return exitCode
}

func parseArgumentsArgs(argv []string) (Arguments, error) {
	parser := &docopt.Parser{HelpHandler: func(err error, text string) {
		if err == nil {
			fmt.Fprintln(os.Stdout, text)
			os.Exit(0)
		}
	}}
	opts, err := parser.ParseArgs(usage, argv, version)
	if err != nil {
		return Arguments{}, err
	}

	for _, option := range []string{"--overlap", "--diagnostics", "--stream", "--stream-persistent"} {
		if value, present := opts[option].(string); present && value == "" {
			return Arguments{}, fmt.Errorf("%s must not be empty", option)
		}
	}
	var args Arguments
	if err := opts.Bind(&args); err != nil {
		return Arguments{}, err
	}

	return args, nil
}

func buildSearch(args Arguments) (*Search, error) {
	if err := validateSearchArguments(args); err != nil {
		return nil, diagnosticError("argument", "", "", err)
	}
	if err := validateGlobPatterns(args.ValueIncludes); err != nil {
		return nil, diagnosticError("argument", "", "", err)
	}
	if err := validateGlobPatterns(args.ValueExcludes); err != nil {
		return nil, diagnosticError("argument", "", "", err)
	}

	query, err := compileQuery(args.ValueQuery, args.FlagLiteral)
	if err != nil {
		return nil, diagnosticError("query", "", args.ValueQuery, err)
	}

	filters, err := buildBlockFilters(args.ValueAwkIfs)
	if err != nil {
		return nil, diagnosticError("argument", "", "", err)
	}

	output, err := outputPolicyFromArgs(args)
	if err != nil {
		return nil, diagnosticError("argument", "", "", err)
	}

	return &Search{
		parseOptions: ParseOptions{Matches: args.FlagMatches, Overlap: args.ValueOverlap, Limits: limitsFromArgs(args)},
		query:        query,
		filters:      filters,
		files:        resolveInputFiles(args.ValueFiles),
		output:       output,
		walker:       newFileWalkerForCLI(args.ValueIncludes, args.ValueExcludes),
	}, nil
}

func compileQuery(pattern string, literal bool) (*regexp.Regexp, error) {
	if literal {
		pattern = regexp.QuoteMeta(pattern)
	}

	// Queries match the whole file so multiline patterns (with \n) resolve
	// to the surrounding block, but ^ and $ match at each line boundary.
	return regexp.Compile("(?m)" + pattern)
}

func buildBlockFilters(conditions []string) ([]*BlockConditionMatcher, error) {
	filters := make([]*BlockConditionMatcher, len(conditions))
	for i, condition := range conditions {
		filters[i] = NewBlockConditionMatcher(condition)
		if err := filters[i].Validate(); err != nil {
			return nil, fmt.Errorf("compile AWK filter %q: %w", condition, err)
		}
	}
	return filters, nil
}

func resolveInputFiles(files []string) []string {
	if len(files) != 0 {
		return files
	}
	if isatty.IsTerminal(os.Stdin.Fd()) {
		return []string{"."}
	}
	return []string{"/dev/stdin"}
}

func outputPolicyFromArgs(args Arguments) (OutputPolicy, error) {
	useColors, err := resolveUseColors(args.ValueColor)
	if err != nil {
		return OutputPolicy{}, err
	}

	showLineNumbers := !args.FlagNoLine
	hashline := args.FlagHashline && showLineNumbers && !args.FlagShowFilenamePerLine

	return OutputPolicy{
		ShowFilename:            args.FlagShowFilenamePerLine,
		ShowLine:                showLineNumbers,
		UseColors:               useColors,
		FilesOnly:               args.FlagFilesOnly,
		JSON:                    args.FlagJSON || args.FlagMatches,
		Null:                    args.FlagNull,
		PersistentStreamCommand: args.ValuePersistentStreamCommand,
		OutputBytes:             args.ValueMaxOutputBytes,
		Hashline:                hashline,
		StreamCommand:           args.ValueStreamCommand,
	}, nil
}

// resolveUseColors maps the --color policy to a concrete on/off choice.
// "auto" follows stdout: colorize only when it is a terminal, so piped or
// redirected output (into another command or a file) stays plain, matching
// grep's default.
func resolveUseColors(when string) (bool, error) {
	switch when {
	case "", "auto":
		return isatty.IsTerminal(os.Stdout.Fd()), nil
	case "always":
		return true, nil
	case "never":
		return false, nil
	default:
		return false, fmt.Errorf("invalid --color %q: want never, always, or auto", when)
	}
}

// newFileWalkerForCLI builds the production walker and wires the HOME-based
// global ignore matcher, the only place that should read the environment.
func newFileWalkerForCLI(includes, excludes []string) *FileWalker {
	walker := NewFileWalker(".", includes, excludes)

	// Only a set HOME names a real global ignore file; an empty HOME would
	// collapse filepath.Join to a relative path and read a file from the
	// current directory instead.
	if home := os.Getenv("HOME"); home != "" {
		globalGitignore := filepath.Join(home, ".gitignore_global")
		if matcher, err := gitignore.NewGitIgnore(globalGitignore); err == nil {
			walker.SetGlobalIgnore(matcher)
		}
	}

	return walker
}
