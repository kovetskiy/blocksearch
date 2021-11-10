package main

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/lexers"
	"github.com/alecthomas/chroma/quick"
	"github.com/reconquest/karma-go"
	"github.com/reconquest/pkg/log"
)

type BlockLine struct {
	Line int
	Text string
}

type Block []BlockLine

func (block Block) GetLineStart() int {
	return block[0].Line
}

func (block Block) GetLineEnd() int {
	return block[len(block)-1].Line
}

func (block Block) JoinLines() string {
	lines := make([]string, len(block))
	for i := 0; i < len(block); i++ {
		lines[i] = block[i].Text
	}
	return strings.Join(lines, "\n")
}

func (block Block) Format(
	showFilenameInline bool,
	filename string,
	showLine bool,
	useColors bool,
) string {
	if !useColors {
		lines := make([]string, len(block))
		for i := 0; i < len(block); i++ {
			lines[i] = formatLine(
				showFilenameInline,
				filename,
				showLine,
				block[i].Line,
				block[i].Text,
			)
		}
	}

	lines := make([]string, len(block))
	numbers := make([]int, len(block))
	for i := 0; i < len(block); i++ {
		lines[i] = block[i].Text
		numbers[i] = block[i].Line
	}

	buffer := bytes.NewBuffer(nil)

	lexer := lexers.Match(filename)
	if lexer == nil {
		lexer = lexers.Fallback
	}

	err := quick.Highlight(
		buffer,
		strings.Join(lines, "\n"),
		lexer.Config().Name,
		"terminal",
		"vim",
	)
	if err != nil {
		log.Errorf(err, "syntax highlight: %q %v", filename, numbers)
	}

	if !showLine && !showFilenameInline {
		return buffer.String()
	}

	highlighted := strings.Split(buffer.String(), "\n")

	min := len(highlighted)
	if len(block) < min {
		min = len(block)
	}
	for i := 0; i < min; i++ {
		highlighted[i] = formatLine(
			showFilenameInline,
			filename,
			showLine,
			block[i].Line,
			highlighted[i],
		)
	}

	return strings.Join(highlighted, "\n")
}

type Blocks []Block

func (blocks Blocks) Format(
	showFilenameInline bool,
	filename string,
	showLine bool,
	useColors bool,
) string {
	result := make([]string, len(blocks))
	for i := 0; i < len(blocks); i++ {
		result[i] = blocks[i].Format(
			showFilenameInline,
			filename,
			showLine,
			useColors,
		)
	}

	return strings.Join(result, "\n\n")
}

type BlockExport struct {
	Filename  string `json:"filename"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
	Text      string `json:"text"`
}

func (blocks *Blocks) EncodeJSON(
	filename string,
) ([]byte, error) {
	buffer := []byte{}
	for _, block := range *blocks {
		js, err := block.EncodeJSON(filename)
		if err != nil {
			return nil, err
		}

		buffer = append(buffer, js...)
		buffer = append(buffer, []byte("\n")...)
	}

	return buffer, nil
}

func (blocks *Blocks) Stream(streamCmd string, filename string) error {
	for _, block := range *blocks {
		encoded, err := block.EncodeJSON(filename)
		if err != nil {
			return err
		}

		cmd := exec.Command(streamCmd)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = bytes.NewBuffer(encoded)

		err = cmd.Run()
		if err != nil {
			if _, ok := err.(*exec.ExitError); !ok {
				return err
			}
		}
	}

	return nil
}

func (block Block) EncodeJSON(filename string) ([]byte, error) {
	export := BlockExport{
		Filename:  filename,
		LineStart: block.GetLineStart(),
		LineEnd:   block.GetLineEnd(),
		Text:      block.JoinLines(),
	}

	return json.Marshal(export)
}

func findBlocks(
	filename string,
	query *regexp.Regexp,
	higherThan int,
) (Blocks, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, karma.Format(err, "open file")
	}

	defer file.Close()

	header := make([]byte, 256)
	_, err = file.Read(header)
	if err != nil && err != io.EOF {
		return nil, karma.Format(err, "read header")
	}

	kind := http.DetectContentType(header)

	log.Debug("content type: " + kind)

	if !strings.HasPrefix(kind, "text/plain") {
		return nil, nil
	}

	_, err = file.Seek(0, 0)
	if err != nil {
		return nil, karma.Format(err, "file seek")
	}

	contents, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, karma.Format(err, "read file")
	}

	lines := strings.Split(string(contents), "\n")

	indent, err := getIndentation(lines)
	if err != nil {
		return nil, err
	}

	result := []Block{}
	for lineIndex := 0; lineIndex < len(lines); lineIndex++ {
		text := lines[lineIndex]

		if query.Match([]byte(text)) {
			lineLevel := getIndentationLevel(text, indent)

			block := []BlockLine{
				{
					Line: lineIndex + 1,
					Text: text,
				},
			}

			nextLine := lineIndex + 1
			for ; nextLine < len(lines); nextLine++ {
				if lines[nextLine] == "" ||
					getIndentationLevel(
						lines[nextLine],
						indent,
					) > lineLevel+higherThan {
					block = append(block, BlockLine{
						Line: nextLine + 1,
						Text: lines[nextLine],
					})
				} else {
					if len(block) > 1 {
						block = append(block, BlockLine{
							Line: nextLine + 1,
							Text: lines[nextLine],
						})
					}
					break
				}
			}

			result = append(result, Block(block))

			lineIndex = nextLine
			continue
		}
	}

	return result, nil
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

func formatLine(
	showFilenameInline bool,
	filename string,
	showLine bool,
	line int,
	text string,
) string {
	if showLine {
		text = strconv.Itoa(line) + ":" + text
	}
	if showFilenameInline {
		text = filename + ":" + text
	}
	return text
}

func filterBlocks(blocks Blocks, filters []*regexp.Regexp) Blocks {
	if len(filters) == 0 {
		return blocks
	}

	result := Blocks{}

	for _, block := range blocks {
		found := false
	nextline:
		for _, line := range block {
			for _, filter := range filters {
				if filter.MatchString(line.Text) {
					found = true
					break nextline
				}
			}
		}

		if found {
			result = append(result, block)
		}
	}
	return result
}
