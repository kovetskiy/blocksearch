package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/kovetskiy/lorg"
	"github.com/reconquest/pkg/log"

	"github.com/docopt/docopt-go"

	"github.com/mattn/go-isatty"
)

var (
	version = "[manual build]"
	usage   = "blocksearch " + version + `

Usage:
  blocksearch [options] <query> [<file>...]
  blocksearch -h | --help
  blocksearch --version

Options:
  -i <n>              Show lines higher than current indentation level plus <n> (can be negative).
  -f --file           Show filename before the line.
  -l --no-line        Do not show number of line before the line.
  -c --no-color       Do not use colors for syntax highlighting.
  -j --json           Output blocks in JSON.
  -S --stream <path>  Stream and execute the given program. Enforces JSON.
  -h --help           Show this screen.
  -v                  Be verbose.
  --version           Show version.
`
)

func main() {
	args, err := docopt.Parse(usage, nil, true, version, false)
	if err != nil {
		panic(err)
	}

	var (
		files, _                = args["<file>"].([]string)
		dontShowLine, _         = args["--no-line"].(bool)
		dontUseColors, _        = args["--no-colors"].(bool)
		showFilenameInline, _   = args["--file"].(bool)
		higherThanArg, _        = args["-i"].(string)
		useJSON                 = args["--json"].(bool)
		streamCmd, useStreaming = args["--stream"].(string)
	)

	verbose, _ := args["-v"].(bool)
	if verbose {
		log.SetLevel(lorg.LevelDebug)
	}

	query, err := regexp.Compile(args["<query>"].(string))
	if err != nil {
		log.Fatalf(err, "invalid regexp")
	}

	var higherThan int
	if higherThanArg != "" {
		higherThan, err = strconv.Atoi(higherThanArg)
		if err != nil {
			log.Fatal(err)
		}
	}

	if len(files) == 0 {
		if isatty.IsTerminal(os.Stdout.Fd()) {
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

			blocks, err := findBlocks(path, query, higherThan)
			if err != nil {
				log.Errorf(err, "%s", path)
				return
			}

			if len(blocks) > 0 {
				if useStreaming {
					err := blocks.Stream(streamCmd, path)
					if err != nil {
						log.Errorf(err, "stream failed")
						return
					}
				} else if useJSON {
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

					if !showFilenameInline {
						fmt.Println(path)
					}

					fmt.Print(
						blocks.Format(
							showFilenameInline,
							path,
							!dontShowLine,
							!dontUseColors,
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
