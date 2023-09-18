package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/benhoyt/goawk/interp"
)

type AwkwardMatcher struct {
	Condition string
	contents  string
}

func NewAwkwardMatcher(condition string) *AwkwardMatcher {
	if condition == "" {
		condition = "1"
	}

	matcher := &AwkwardMatcher{
		Condition: condition,
	}

	matcher.contents = `
	{
		_line = $0
		if (_block) {
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

func (matcher *AwkwardMatcher) Match(block string) (bool, error) {
	input := strings.NewReader(block)
	output := bytes.NewBuffer(nil)

	err := interp.Exec(matcher.contents, " ", input, output)
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
