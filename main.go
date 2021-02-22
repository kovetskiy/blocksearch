package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/reconquest/pkg/log"

	"github.com/docopt/docopt-go"
)

var (
	version = "[manual build]"
	usage   = "blocksearch " + version + `

Usage:
  blocksearch [options] <query> [<file>...]
  blocksearch -h | --help
  blocksearch --version

Options:
  -i <n>          Show lines higher than current indentation level plus <n> (can be negative).
  -f --file       Show filename before the line.
  -l --no-line       Show number of line before the line.
  -h --help       Show this screen.
  --version       Show version.
`
	//-e --exactly    Do not use regexp, search for exactly specified string instead.
)

func main() {
	args, err := docopt.Parse(usage, nil, true, version, false)
	if err != nil {
		panic(err)
	}

	var (
		query                 = args["<query>"].(string)
		files, withFiles      = args["<file>"].([]string)
		dontShowLine, _       = args["--no-line"].(bool)
		showFilenameInline, _ = args["--file"].(bool)
		higherThanArg, _      = args["-i"].(string)
	)

	var higherThan int
	if higherThanArg != "" {
		higherThan, err = strconv.Atoi(higherThanArg)
		if err != nil {
			log.Fatal(err)
		}
	}

	if !withFiles {
		files = []string{"/dev/stdin"}
	}

	shouldAddLine := false
	for _, file := range files {
		stat, err := os.Stat(file)
		if err != nil {
			log.Errorf(err, "%s", file)
			continue
		}

		process := func(path string) {
			blocks, err := findBlocks(path, query, higherThan)
			if err != nil {
				log.Errorf(err, "%s", path)
				return
			}

			if len(blocks) > 0 {
				if shouldAddLine {
					fmt.Println()
				}

				if !showFilenameInline {
					fmt.Println(path)
				}

				fmt.Println(
					blocks.Format(showFilenameInline, path, !dontShowLine),
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
