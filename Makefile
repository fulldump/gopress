
VERSION = $(shell git describe --tags --always)
FLAGS = -ldflags "\
  -X main.VERSION=$(VERSION) \
"

run:
	go run $(FLAGS) main.go

build:
	CGO_ENABLED=0 go build $(FLAGS) -o bin/gopress main.go

deps:
	go mod tidy
	go mod vendor

test:
	go test -cover ./...

.PHONY: version
version:
	@echo $(VERSION)
