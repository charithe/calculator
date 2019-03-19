USER:=$(shell id -u):$(shell id -g)
VERSION:=$(shell git describe --tag --always --dirty)
DOCKER_IMAGE:=charithe/calculator:$(VERSION)

generate:
	@docker run --rm --user $(USER) -i -t -v $(shell pwd):/work uber/prototool:latest prototool all 

build: generate
	@skaffold build

deploy:
	@skaffold deploy

destroy:
	@skaffold delete

launch:
	@docker build --rm -t $(DOCKER_IMAGE) .
	@docker run --rm -i -t -p 8080:8080 -p 5000:5000 $(DOCKER_IMAGE)

cli:
	@GO111MODULE=on go build -o cli cmd/cli/main.go
