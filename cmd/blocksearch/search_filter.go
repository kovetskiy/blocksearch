package main

import (
	"context"
	"fmt"
)

func filterSearchBlocks(ctx context.Context, blocks Blocks, filters []*BlockConditionMatcher) (Blocks, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(filters) == 0 {
		return blocks, nil
	}
	var selected Blocks
	for _, block := range blocks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		lines := block.JoinLines()
		for _, filter := range filters {
			matched, err := filter.MatchContext(ctx, lines)
			if err != nil {
				return nil, fmt.Errorf("match block against condition %q: %w", filter.Condition, err)
			}
			if matched {
				selected = append(selected, block)
				break
			}
		}
	}
	return selected, nil
}
