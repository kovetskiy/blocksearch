package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/benhoyt/goawk/interp"
	"github.com/benhoyt/goawk/parser"
)

type BlockConditionMatcher struct {
	Condition  string
	awkProgram string
}

func NewBlockConditionMatcher(condition string) *BlockConditionMatcher {
	if condition == "" {
		condition = "1"
	}

	matcher := &BlockConditionMatcher{
		Condition: condition,
	}

	matcher.awkProgram = `
	{
		_line = $0
		if (_count++) {
			_block = _block "\n" _line
		} else {
			_block = _line
		}
	}
	END {
		_matched = 0

		$0 = _block
		if (` + condition + `) {
			_matched = 1
		}

		if (_matched) {
			print "TRUE"
		} else {
			print "FALSE"
		}
	}`

	return matcher
}

func (matcher *BlockConditionMatcher) Match(block string) (bool, error) {
	return matcher.MatchContext(context.Background(), block)
}

func (matcher *BlockConditionMatcher) MatchContext(ctx context.Context, block string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	program, err := parser.ParseProgram([]byte(matcher.awkProgram), nil)
	if err != nil {
		return false, err
	}
	interpreter, err := interp.New(program)
	if err != nil {
		return false, err
	}
	output := bytes.NewBuffer(nil)
	_, err = interpreter.ExecuteContext(ctx, &interp.Config{
		Stdin: strings.NewReader(block), Output: output, Error: io.Discard,
		Vars: []string{"FS", " "},
	})
	if err != nil {
		return false, err
	}

	result := strings.TrimSpace(output.String())

	switch result {
	case "TRUE":
		return true, nil
	case "FALSE":
		return false, nil
	default:
		return false, fmt.Errorf("unexpected result: %s", result)
	}
}

// Validate parses the embedded AWK program so malformed filters fail at
// search construction (before any block is matched), not lazily when a
// matched block happens to be evaluated.
func (matcher *BlockConditionMatcher) Validate() error {
	_, err := parser.ParseProgram([]byte(matcher.awkProgram), nil)
	return err
}
