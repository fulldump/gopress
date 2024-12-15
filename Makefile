
VERSION = $(shell git describe --tags --always)
FLAGS = -ldflags "\
  -X main.VERSION=$(VERSION) \
"

.PHONY: run
run:
	STATICS=statics/www/ go run $(FLAGS) main.go

.PHONY: build
build:
	CGO_ENABLED=0 go build $(FLAGS) -o bin/gopress main.go

.PHONY: release
release: clean
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build $(FLAGS) -o bin/gopress.linux.arm64 ./
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build $(FLAGS) -o bin/gopress.linux.amd64 ./
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build $(FLAGS) -o bin/gopress.win.arm64.exe ./
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(FLAGS) -o bin/gopress.win.amd64.exe ./
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build $(FLAGS) -o bin/gopress.mac.arm64 ./
	CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build $(FLAGS) -o bin/gopress.mac.amd64 ./
	md5sum bin/gopress.* > bin/checksum-md5
	sha256sum bin/gopress.* > bin/checksum-sha256

.PHONY: clean
clean:
	rm -f bin/*

.PHONY: deps
deps:
	go get -t -u ./...
	go mod tidy
	go mod vendor

.PHONY: test
test:
	go test -cover ./...

.PHONY: version
version:
	@echo $(VERSION)
