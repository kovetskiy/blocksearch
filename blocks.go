package main

import (
	"bytes"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/lexers"
	"github.com/alecthomas/chroma/quick"
	"github.com/reconquest/pkg/log"
)

type BlockLine struct {
	Line int
	Text string
}

type Block []BlockLine

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

func findBlocks(
	filename string,
	query string,
	higherThan int,
) (Blocks, error) {
	contents, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(contents), "\n")

	indent, err := getIndentation(lines)
	if err != nil {
		return nil, err
	}

	result := []Block{}
	for lineIndex := 0; lineIndex < len(lines); lineIndex++ {
		text := lines[lineIndex]

		if strings.Contains(text, query) {
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
