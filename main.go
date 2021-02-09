package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
		query            = args["<query>"].(string)
		files, withFiles = args["<file>"].([]string)
		dontShowLine, _  = args["--no-line"].(bool)
		showFilename, _  = args["--file"].(bool)
	)

	if !withFiles {
		files = []string{"/dev/stdin"}
	}

	prevFound := false
	for _, file := range files {
		stat, err := os.Stat(file)
		if err != nil {
			log.Errorf(err, "%s", file)
			continue
		}

		process := func(path string) {
			if prevFound {
				fmt.Println()
			}

			found, err := queryFile(path, query, showFilename, !dontShowLine)
			if err != nil {
				log.Errorf(err, "%s", path)
				return
			}

			prevFound = found
		}

		if stat.IsDir() {
			err := filepath.Walk(file, func(path string, info os.FileInfo, _ error) error {
				if err != nil {
					return err
				}

				if info.IsDir() {
					return nil
				}

				process(path)

				return nil
			})
			if err != nil {
				log.Errorf(err, "walk through %s", file)
			}
		} else {
			process(file)
		}
	}
}

func queryFile(filename string, query string, showFilename bool, showLine bool) (bool, error) {
	contents, err := ioutil.ReadFile(filename)
	if err != nil {
		return false, err
	}

	lines := strings.Split(string(contents), "\n")

	indent, err := getIndentation(lines)
	if err != nil {
		return false, err
	}

	formatLine := func(text string, line int) string {
		if showLine {
			text = strconv.Itoa(line) + ":" + text
		}
		if showFilename {
			text = filename + ":" + text
		}

		return text
	}

	multiple := false
	found := false
	for lineIndex := 0; lineIndex < len(lines); lineIndex++ {
		line := lines[lineIndex]

		if strings.Contains(line, query) {
			found = true
			lineLevel := getIndentationLevel(line, indent)

			result := []string{formatLine(line, lineIndex+1)}
			nextLine := lineIndex + 1
			for ; nextLine < len(lines); nextLine++ {
				if lines[nextLine] == "" ||
					getIndentationLevel(lines[nextLine], indent) > lineLevel {
					result = append(
						result,
						formatLine(lines[nextLine], nextLine+1),
					)
				} else {
					if len(result) > 1 {
						result = append(
							result,
							formatLine(lines[nextLine], nextLine+1),
						)
					}
					break
				}
			}

			if multiple {
				fmt.Println()
			}

			multiple = true

			if !showFilename {
				fmt.Println(filename)
			}

			fmt.Println(strings.Join(result, "\n"))

			lineIndex = nextLine
			continue
		}
	}

	return found, nil
}

func getIndentation(lines []string) (byte, error) {
	for _, line := range lines {
		if line == "" {
			continue
		}
		if line[0] == '\t' {
			return '\t', nil
		}
		if line[0] == ' ' {
			return ' ', nil
		}
	}

	return ' ', nil
}

func getIndentationLevel(line string, indent byte) int {
	for i := 0; i < len(line); i++ {
		if line[i] != indent {
			return i
		}
	}

	// the entire line is just spacing, so no indentation
	return 0
}
