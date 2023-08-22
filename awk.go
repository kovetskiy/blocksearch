package main

import (
	"os/exec"
	"strings"

	"github.com/reconquest/karma-go"
)

type AwkwardMatcher struct {
	Condition string
	contents  string
}

func NewAwkwardMatcher(condition string) *AwkwardMatcher {
	matcher := &AwkwardMatcher{
		Condition: condition,
	}

	matcher.contents = `{if (` + condition + `) { exit 0 } else { exit 3 }}`

	return matcher
}

func (matcher *AwkwardMatcher) Match(block string) (bool, error) {
	cmd := exec.Command("awk", matcher.contents)

	cmd.Stdin = strings.NewReader(block)

	output, err := cmd.CombinedOutput()
	switch {
	case err == nil:
		return true, nil
	case err.(*exec.ExitError).ExitCode() == 3:
		return false, nil
	default:
		return false, karma.
			Describe("output", string(output)).
			Format(err, "awk exited with error")
	}
}
