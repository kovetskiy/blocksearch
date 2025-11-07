package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kovetskiy/lorg"
	"github.com/monochromegane/go-gitignore"
	"github.com/reconquest/pkg/log"

	"github.com/docopt/docopt-go"

	"github.com/mattn/go-isatty"
)

var (
	version = "v1.4.0"
	usage   = "blocksearch " + version + `

Usage:
  blocksearch [options] <query> [<file>...] [-a <if>]... [-x <ext>]...
  blocksearch -h | --help
  blocksearch --version

Options:
  -i <n>                 Show lines higher than current indentation level plus <n> (can be negative).
  -t --file              Show filename before the line.
  -l --no-line           Do not show number of line before the line.
  -c --no-colors         Do not use colors for syntax highlighting.
  -j --json              Output blocks in JSON.
  -S --stream <path>     Stream and execute the given program. Enforces JSON.
  -a --awk <if>          Filter blocks by specified AWK condition.
  -e --exit-code <code>  Exit with the specified code if blocks were found. [default: 0]
  --message <warn>       Show the specified message if blocks were found.
  -x --extension <ext>   Search files only with the specified extensions.
  -v                     Be verbose.
  --version              Show version.
  -h --help              Show this screen.
`
)

type Arguments struct {
	ValueHigherThan int      `docopt:"-i"`
	ValuePipeStream string   `docopt:"--stream"`
	ValueFilters    []string `docopt:"--filter"`
	ValueExtensions []string `docopt:"--extension"`
	ValueExitCode   int      `docopt:"--exit-code"`
	ValueAwkIfs     []string `docopt:"--awk"`
	ValueMessage    string   `docopt:"--message"`

	FlagShowFilenamePerLine bool `docopt:"--file"`
	FlagNoShowLineNumber    bool `docopt:"--no-line"`
	FlagNoColors            bool `docopt:"--no-colors"`
	FlagJSON                bool `docopt:"--json"`
	FlagVerbose             bool `docopt:"-v"`

	ValueQuery string   `docopt:"<query>"`
	ValueFiles []string `docopt:"<file>"`
}

func main() {
	opts, err := docopt.ParseDoc(usage)
	if err != nil {
		panic(err)
	}

	var args Arguments
	err = opts.Bind(&args)
	if err != nil {
		panic(err)
	}

	var (
		extensions = expandExtensions(args.ValueExtensions)
	)

	if args.FlagVerbose {
		log.SetLevel(lorg.LevelDebug)
	}

	query, err := regexp.Compile(args.ValueQuery)
	if err != nil {
		log.Fatalf(err, "invalid regexp")
	}

	ignoreMatcher, err := gitignore.NewGitIgnore(".gitignore")
	if err != nil && !os.IsNotExist(err) {
		log.Fatalf(
			err,
			"unable to read .gitignore",
		)
	}

	files := args.ValueFiles
	if len(args.ValueFiles) == 0 {
		if isatty.IsTerminal(os.Stdin.Fd()) {
			files = []string{"."}
		} else {
			files = []string{"/dev/stdin"}
		}
	}

	var filters []*AwkwardMatcher
	for _, filter := range args.ValueAwkIfs {
		filters = append(
			filters,
			NewAwkwardMatcher(filter),
		)
	}

	found := 0
	shouldAddLine := false
	for _, file := range files {
		log.Debug("stat: " + file)

		stat, err := os.Stat(file)
		if err != nil {
			log.Errorf(err, "%s", file)
			continue
		}

		process := func(path string) {
			log.Debug("process: " + path)

			blocks, err := findBlocks(path, query, args.ValueHigherThan)
			if err != nil {
				log.Errorf(err, "%s", path)
				return
			}

			blocks, err = filterBlocks(blocks, filters)
			if err != nil {
				log.Errorf(err, "%s", path)
				return
			}

			if len(blocks) == 0 {
				return
			}

			found += len(blocks)

			switch {
			case args.ValuePipeStream != "":
				err := blocks.Stream(args.ValuePipeStream, path)
				if err != nil {
					log.Errorf(err, "stream failed")
				}
			case args.FlagJSON:
				buffer, err := blocks.EncodeJSON(path)
				if err != nil {
					log.Errorf(err, "json encode blocks")
				} else {
					os.Stdout.Write(buffer)
				}
			default:
				if shouldAddLine {
					fmt.Println()
				}

				fmt.Println(
					strings.Join(
						blocks.Format(
							args.FlagShowFilenamePerLine,
							path,
							!args.FlagNoShowLineNumber,
							!args.FlagNoColors,
						),
						"\n\n",
					),
				)

				shouldAddLine = true
			}
		}

		if stat.IsDir() {
			err := filepath.Walk(
				file,
				func(path string, info os.FileInfo, _ error) error {
					if err != nil {
						return err
					}

					if info.IsDir() {
						if filepath.Base(path) == ".git" {
							return filepath.SkipDir
						}

						if ignoreMatcher != nil && ignoreMatcher.Match(path, true) {
							return filepath.SkipDir
						}

						return nil
					}

					if len(extensions) != 0 && !hasExtension(path, extensions) {
						return nil
					}

					if ignoreMatcher != nil &&
						ignoreMatcher.Match(path, info.IsDir()) {
						return nil
					}

					process(path)

					return nil
				},
			)
			if err != nil {
				log.Errorf(err, "walk through %s", file)
			}
		} else {
			process(file)
		}
	}

	if found != 0 {
		if args.ValueMessage != "" {
			fmt.Println(args.ValueMessage)
		}

		os.Exit(args.ValueExitCode)
	}
}

func expandExtensions(args []string) []string {
	result := []string{}
	for _, ext := range args {
		if strings.Contains(ext, ",") {
			result = append(result, strings.Split(ext, ",")...)
		} else {
			result = append(result, ext)
		}
	}
	return result
}

func hasExtension(path string, extensions []string) bool {
	for _, ext := range extensions {
		if strings.HasSuffix(path, "."+ext) {
			return true
		}
	}
	return false
}
