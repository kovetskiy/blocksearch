package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAwkTest(t *testing.T) {
	test := assert.New(t)

	matcher := NewAwkwardMatcher("/a/")

	testcases := []struct {
		block    string
		expected bool
		err      bool
	}{
		{
			block:    "a",
			expected: true,
			err:      false,
		},
		{
			block:    "b",
			expected: false,
			err:      false,
		},
	}

	for i, testcase := range testcases {
		actual, err := matcher.Match(testcase.block)
		if testcase.err {
			test.Error(err, "testcase %d", i)
		} else {
			test.NoError(err, "testcase %d", i)
		}

		test.Equal(testcase.expected, actual, "testcase %d", i)
	}
}
