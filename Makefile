VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)

.PHONY: hooks
hooks:
	git config core.hooksPath .githooks

.PHONY: test
test:
	env -u GOROOT go vet ./...
	env -u GOROOT golangci-lint run ./...
	env -u GOROOT go test -race -shuffle=on ./...

.PHONY: build
build:
	go build -ldflags "$(LDFLAGS)" -o bin/nem ./cmd/nem

.PHONY: install
install:
	go install -ldflags "$(LDFLAGS)" ./cmd/nem
