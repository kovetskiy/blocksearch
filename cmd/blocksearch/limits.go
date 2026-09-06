package main

import "fmt"

type SearchLimits struct {
	FileBytes   int64
	BlockLines  int
	BlockBytes  int64
	Matches     int
	Blocks      int
	OutputBytes int64
}

type LimitError struct {
	Resource string
	Maximum  int64
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("limit exceeded: %s (maximum %d)", e.Resource, e.Maximum)
}
