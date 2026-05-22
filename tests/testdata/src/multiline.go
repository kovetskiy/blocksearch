package main

import (
	"context"
	"net/http"
)

func foobar(
	ctx context.Context,
	r *http.Request,
	w http.ResponseWriter,
) {
	panic("foobar")
}
