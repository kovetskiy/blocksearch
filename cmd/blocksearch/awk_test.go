package main

import "testing"

func TestAwkConditionMatcherMatchesBlocks(t *testing.T) {
	matcher := NewBlockConditionMatcher("/a/")

	testcases := []struct {
		block    string
		expected bool
	}{
		{
			block:    "a",
			expected: true,
		},
		{
			block:    "b",
			expected: false,
		},
	}

	for i, testcase := range testcases {
		actual, err := matcher.Match(testcase.block)
		if err != nil {
			t.Fatalf("testcase %d: %v", i, err)
		}

		if actual != testcase.expected {
			t.Fatalf("testcase %d: Match = %v, want %v", i, actual, testcase.expected)
		}
	}
}

// Bug: an AWK filter that should match a block whose first line is "0" did
// not, because the accumulator used `if (_block)` and the string "0" is
// falsey in AWK, so the second line replaced the first instead of appending.
func TestAwkMatcherKeepsFirstLineZero(t *testing.T) {
	matcher := NewBlockConditionMatcher("/0/")

	ok, err := matcher.Match("0\nneedle")
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if !ok {
		t.Fatalf("Match = false, want true for a block whose text contains 0")
	}
}
