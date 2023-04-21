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
	version = "[manual build]"
	usage   = "blocksearch " + version + `

Usage:
  blocksearch [options] <query> [<file>...] [-f <regexp>]... [-x <ext>]...
  blocksearch -h | --help
  blocksearch --version

Options:
  -i <n>                Show lines higher than current indentation level plus <n> (can be negative).
  -t --file             Show filename before the line.
  -l --no-line          Do not show number of line before the line.
  -c --no-colors        Do not use colors for syntax highlighting.
  -j --json             Output blocks in JSON.
  -S --stream <path>    Stream and execute the given program. Enforces JSON.
  -f --filter <regexp>  Filter blocks by specified regexp.  
  -x --extension <ext>  Search files only with the specified extensions.
  -v                    Be verbose.
  --version             Show version.
  -h --help             Show this screen.
`
)

type Arguments struct {
	ValueHigherThan int      `docopt:"-i"`
	ValuePipeStream string   `docopt:"--stream"`
	ValueFilters    []string `docopt:"--filter"`
	ValueExtensions []string `docopt:"--extension"`

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
		filters    = compileRegexps(args.ValueFilters)
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

			blocks = filterBlocks(blocks, filters)

			if len(blocks) > 0 {
				if args.ValuePipeStream != "" {
					err := blocks.Stream(args.ValuePipeStream, path)
					if err != nil {
						log.Errorf(err, "stream failed")
						return
					}
				} else if args.FlagJSON {
					buffer, err := blocks.EncodeJSON(path)
					if err != nil {
						log.Errorf(err, "json encode blocks")
						return
					}

					os.Stdout.Write(buffer)
				} else {
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
}

func compileRegexps(raw []string) []*regexp.Regexp {
	result := []*regexp.Regexp{}
	for _, query := range raw {
		re, err := regexp.Compile(query)
		if err != nil {
			log.Fatalf(err, "compile: %q", query)
		}

		result = append(result, re)
	}
	return result
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
