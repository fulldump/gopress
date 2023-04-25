
run:
	go run main.go

build:
	CGO_ENABLED=0 go build -o bin/gopress main.go

deps:
	go mod tidy
	go mod vendor

test:
	go test -cover ./...
