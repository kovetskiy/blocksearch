package main

type Ledger struct {
	total int
}

func (l Ledger) Tally() int {
	return l.total
}
