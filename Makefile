.PHONY: build build-all build-tui install test test-go test-bash clean

VERSION ?= dev

build:
	go build -ldflags "-X main.Version=$(VERSION)" -o bin/safe-ag ./cmd/safe-ag

build-tui:
	go build -o bin/agent-tui ./tui

build-all: build build-tui

install: build
	cp bin/safe-ag /usr/local/bin/safe-ag

test: test-go

test-go:
	go test ./...

test-bash:
	bash tests/run-all.sh

test-all: test-go test-bash

clean:
	rm -f bin/safe-ag bin/agent-tui
