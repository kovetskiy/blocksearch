NAME = kovetskiy/blocksearch

build:
	CGO_ENABLED=1 go build -o blocksearch ./cmd/blocksearch
