.PHONY: build test lint clean install-tools preflight cross-compile

BINARY=grid-worker
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

build:
	CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=$(VERSION)" -o bin/$(BINARY) ./cmd/grid-worker

test:
	go test -race -cover ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/

install-tools:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

cross-compile:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/$(BINARY)-linux-amd64 ./cmd/grid-worker
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bin/$(BINARY)-linux-arm64 ./cmd/grid-worker
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -o bin/$(BINARY)-darwin-amd64 ./cmd/grid-worker
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o bin/$(BINARY)-darwin-arm64 ./cmd/grid-worker
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o bin/$(BINARY)-windows-amd64.exe ./cmd/grid-worker

preflight:
	./bin/$(BINARY) preflight --json
