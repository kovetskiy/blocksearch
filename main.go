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
	version = "v1.5.0"
	usage   = "blocksearch " + version + `

Usage:
  blocksearch [options] <query> [<file>...] [-a <if>]... [-x <ext>]...
  blocksearch -M [--workdir <dir>]
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
  -M --mcp               Start MCP (Model Context Protocol) server on stdio.
  --workdir <dir>        Working directory for MCP server (default: current directory).
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
	ValueWorkdir    string   `docopt:"--workdir"`

	FlagShowFilenamePerLine bool `docopt:"--file"`
	FlagNoShowLineNumber    bool `docopt:"--no-line"`
	FlagNoColors            bool `docopt:"--no-colors"`
	FlagJSON                bool `docopt:"--json"`
	FlagVerbose             bool `docopt:"-v"`
	FlagMCP                 bool `docopt:"--mcp"`

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

	// Handle MCP subcommand
	if args.FlagMCP {
		mcpServer, err := NewMCPServer(args.ValueWorkdir)
		if err != nil {
			log.Fatalf(err, "failed to create MCP server")
		}

		if err := mcpServer.Run(); err != nil {
			log.Fatalf(err, "MCP server error")
		}
		return
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

	// Create file walker with current directory as base
	walker := NewFileWalker(".", extensions)

	found := 0
	shouldAddLine := false
	for _, file := range files {
		log.Debug("stat: " + file)

		process := func(path string) error {
			log.Debug("process: " + path)

			blocks, err := findBlocks(path, query, args.ValueHigherThan)
			if err != nil {
				log.Errorf(err, "%s", path)
				return nil
			}

			blocks, err = filterBlocks(blocks, filters)
			if err != nil {
				log.Errorf(err, "%s", path)
				return nil
			}

			if len(blocks) == 0 {
				return nil
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
			return nil
		}

		err := walker.Walk(file, process)
		if err != nil {
			log.Errorf(err, "%s", file)
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

// FileWalker handles walking through files respecting gitignore patterns
type FileWalker struct {
	ignoreMatcher       gitignore.IgnoreMatcher
	globalIgnoreMatcher gitignore.IgnoreMatcher
	extensions          []string
}

// NewFileWalker creates a new FileWalker with gitignore patterns loaded from the given base directory
func NewFileWalker(baseDir string, extensions []string) *FileWalker {
	fw := &FileWalker{
		extensions: extensions,
	}

	gitignorePath := filepath.Join(baseDir, ".gitignore")
	fw.ignoreMatcher, _ = gitignore.NewGitIgnore(gitignorePath)

	globalGitignore := filepath.Join(os.Getenv("HOME"), ".gitignore_global")
	fw.globalIgnoreMatcher, _ = gitignore.NewGitIgnore(globalGitignore)

	return fw
}

// Walk iterates through files in the given path, calling processFile for each matching file
func (fw *FileWalker) Walk(path string, processFile func(path string) error) error {
	stat, err := os.Stat(path)
	if err != nil {
		return err
	}

	if stat.IsDir() {
		return filepath.Walk(path, func(filePath string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return nil // Skip errors
			}

			if info.IsDir() {
				if filepath.Base(filePath) == ".git" {
					return filepath.SkipDir
				}

				if fw.ignoreMatcher != nil && fw.ignoreMatcher.Match(filePath, true) {
					return filepath.SkipDir
				}

				if fw.globalIgnoreMatcher != nil && fw.globalIgnoreMatcher.Match(filePath, true) {
					return filepath.SkipDir
				}

				return nil
			}

			if len(fw.extensions) != 0 && !hasExtension(filePath, fw.extensions) {
				return nil
			}

			if fw.ignoreMatcher != nil && fw.ignoreMatcher.Match(filePath, false) {
				return nil
			}

			if fw.globalIgnoreMatcher != nil && fw.globalIgnoreMatcher.Match(filePath, false) {
				return nil
			}

			return processFile(filePath)
		})
	}

	return processFile(path)
}

// ListFiles returns a list of all files matching the walker's criteria
func (fw *FileWalker) ListFiles(path string) ([]string, error) {
	var files []string
	err := fw.Walk(path, func(filePath string) error {
		files = append(files, filePath)
		return nil
	})
	return files, err
}
