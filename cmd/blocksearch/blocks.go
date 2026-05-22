package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/lexers"
	"github.com/alecthomas/chroma/quick"
	"github.com/reconquest/karma-go"
	"github.com/reconquest/pkg/log"
)

type BlockLine struct {
	Line int
	Hash string
	Text string
}

type Block []BlockLine

func (block Block) LineStart() int {
	return block[0].Line
}

func (block Block) LineEnd() int {
	return block[len(block)-1].Line
}

func (block Block) JoinLines() string {
	lines := make([]string, len(block))
	for i := 0; i < len(block); i++ {
		lines[i] = block[i].Text
	}
	return strings.Join(lines, "\n")
}

type FormatOptions struct {
	Filename        string
	ShowFilename    bool
	ShowLineNumbers bool
	UseColors       bool
	Hashline        bool
	LineNumberWidth int
}

func (block Block) Format(options FormatOptions) string {
	options.LineNumberWidth = block.lineNumberWidth()
	if options.UseColors {
		return block.formatHighlighted(options)
	}
	return block.formatPlain(options)
}

func (block Block) formatPlain(options FormatOptions) string {
	lines := make([]string, len(block))
	for i := 0; i < len(block); i++ {
		lines[i] = formatLine(options, block[i], block[i].Text)
	}
	return strings.Join(lines, "\n")
}

func (block Block) formatHighlighted(options FormatOptions) string {
	texts := make([]string, len(block))
	numbers := make([]int, len(block))
	for i := 0; i < len(block); i++ {
		texts[i] = block[i].Text
		numbers[i] = block[i].Line
	}

	buffer := bytes.NewBuffer(nil)
	lexer := lexers.Match(options.Filename)
	if lexer == nil {
		lexer = lexers.Fallback
	}

	err := quick.Highlight(
		buffer,
		strings.Join(texts, "\n"),
		lexer.Config().Name,
		"terminal",
		"vim",
	)
	if err != nil {
		log.Errorf(err, "syntax highlight: %q %v", options.Filename, numbers)
	}

	if !options.Hashline && !options.ShowLineNumbers && !options.ShowFilename {
		return buffer.String()
	}

	highlighted := strings.Split(buffer.String(), "\n")
	formattedLineCount := len(highlighted)
	if len(block) < formattedLineCount {
		formattedLineCount = len(block)
	}
	for i := 0; i < formattedLineCount; i++ {
		highlighted[i] = formatLine(options, block[i], highlighted[i])
	}

	return strings.Join(highlighted, "\n")
}

type Blocks []Block

func (blocks Blocks) Format(options FormatOptions) []string {
	result := make([]string, len(blocks))
	for i := 0; i < len(blocks); i++ {
		block := blocks[i].Format(options)

		if options.Hashline || !options.ShowFilename {
			result[i] = options.Filename + "\n" + block
		} else {
			result[i] = block
		}
	}

	return result
}

type BlockExport struct {
	Filename   string   `json:"filename"`
	LineStart  int      `json:"line_start"`
	LineEnd    int      `json:"line_end"`
	Text       string   `json:"text"`
	LineHashes []string `json:"line_hashes,omitempty"`
}

func (blocks Blocks) EncodeJSON(filename string, hashline bool) ([]byte, error) {
	var buffer bytes.Buffer
	for _, block := range blocks {
		js, err := block.EncodeJSON(filename, hashline)
		if err != nil {
			return nil, err
		}

		buffer.Write(js)
		buffer.WriteByte('\n')
	}

	return buffer.Bytes(), nil
}

func (blocks Blocks) Stream(streamCommand string, filename string, hashline bool) error {
	for _, block := range blocks {
		encoded, err := block.EncodeJSON(filename, hashline)
		if err != nil {
			return err
		}

		cmd := exec.Command("sh", "-c", streamCommand)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = bytes.NewBuffer(encoded)

		if err := cmd.Run(); err != nil {
			return karma.
				Describe("command", streamCommand).
				Describe("block", filename).
				Format(err, "stream block to consumer")
		}
	}

	return nil
}

func (block Block) EncodeJSON(filename string, hashline bool) ([]byte, error) {
	var lineHashes []string
	if hashline {
		lineHashes = block.LineHashes()
	}

	export := BlockExport{
		Filename:   filename,
		LineStart:  block.LineStart(),
		LineEnd:    block.LineEnd(),
		Text:       block.JoinLines(),
		LineHashes: lineHashes,
	}

	return json.Marshal(export)
}

func (block Block) LineHashes() []string {
	hashes := make([]string, len(block))
	for i := 0; i < len(block); i++ {
		hashes[i] = block[i].Hash
	}
	return hashes
}

func (block Block) lineNumberWidth() int {
	width := 1
	for i := 0; i < len(block); i++ {
		lineWidth := len(strconv.Itoa(block[i].Line))
		if lineWidth > width {
			width = lineWidth
		}
	}
	return width
}

func formatLine(options FormatOptions, line BlockLine, text string) string {
	if options.Hashline {
		lineNumber := paddedLineNumber(line.Line, options.LineNumberWidth)
		text = lineNumber + "#" + line.Hash + "│" + text
	} else if options.ShowLineNumbers {
		text = strconv.Itoa(line.Line) + ":" + text
	}
	if !options.Hashline && options.ShowFilename {
		text = options.Filename + ":" + text
	}
	return text
}

func paddedLineNumber(line int, width int) string {
	lineNumber := strconv.Itoa(line)
	if len(lineNumber) < width {
		lineNumber = strings.Repeat(" ", width-len(lineNumber)) + lineNumber
	}
	return lineNumber
}

func filterBlocks(blocks Blocks, filters []*BlockConditionMatcher) (Blocks, error) {
	if len(filters) == 0 {
		return blocks, nil
	}

	result := Blocks{}

	for _, block := range blocks {
		lines := block.JoinLines()

		found := false
		for _, filter := range filters {
			ok, err := filter.Match(lines)
			if err != nil {
				return nil, karma.
					Describe("condition", filter.Condition).
					Describe("block", lines).
					Format(
						err,
						"match block against condition",
					)

			}

			if ok {
				found = true
				break
			}
		}

		if found {
			result = append(result, block)
		}
	}
	return result, nil
}
