package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestSearchFiltersPreserveORPolicy(t *testing.T) {
	blocks := emissionBlocks("keep", "drop", "also keep")
	filters := []*BlockConditionMatcher{NewBlockConditionMatcher("$0 == \"keep\""), NewBlockConditionMatcher("$0 == \"also keep\"")}
	expected, err := filterBlocks(blocks, filters)
	if err != nil {
		t.Fatalf("legacy filter: %v", err)
	}
	actual, err := filterSearchBlocks(context.Background(), blocks, filters)
	if err != nil || !reflect.DeepEqual(actual, expected) {
		t.Fatalf("filtered = %v, %v; want %v", actual, err, expected)
	}
}

func TestSearchFiltersCancelRunningInterpreter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	filter := NewBlockConditionMatcher("1")
	filter.awkProgram = "BEGIN { while (1) {} }"
	started := time.Now()
	_, err := filterSearchBlocks(ctx, emissionBlocks("hit"), []*BlockConditionMatcher{filter})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("filter: %v, want deadline", err)
	}
	if time.Since(started) > time.Second {
		t.Fatalf("interpreter cancellation took %s", time.Since(started))
	}
}
