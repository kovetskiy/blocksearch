NAME = kovetskiy/blocksearch

build:
	CGO_ENABLED=0 go build -o $(NAME)
	docker build -t $(NAME):latest -f Dockerfile .

push:
	$(eval VERSION = latest)
	$(eval TAG = $(NAME):$(VERSION))
	docker tag $(NAME):$(VERSION) $(TAG)
	docker push $(TAG)
