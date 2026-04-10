.PHONY: build build-tui test test-go test-bash clean

VERSION ?= dev

build:
	go build -ldflags "-X main.Version=$(VERSION)" -o bin/safe-ag ./cmd/safe-ag

build-tui:
	go build -o bin/agent-tui ./tui

test: test-go

test-go:
	go test ./... -v

test-bash:
	bash tests/run-all.sh

clean:
	rm -f bin/safe-ag bin/agent-tui
