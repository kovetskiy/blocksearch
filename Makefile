NAME = kovetskiy/blocksearch

build:
	CGO_ENABLED=0 go build -o blocksearch
	docker build -t $(NAME):latest -f Dockerfile .

push:
	$(eval VERSION = latest)
	$(eval TAG = $(NAME):$(VERSION))
	docker tag $(NAME):latest $(TAG)
	docker push $(TAG)
