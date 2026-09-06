package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAWKComputationCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	matcher := &BlockConditionMatcher{awkProgram: "BEGIN { while (1) {} }"}
	matched, err := matcher.MatchContext(ctx, "hit")
	if matched || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("MatchContext = %v, %v; want false, DeadlineExceeded", matched, err)
	}
}
