.PHONY: build build-all build-tui install test clean

VERSION ?= dev

build:
	go build -ldflags "-X main.Version=$(VERSION)" -o bin/safe-ag ./cmd/safe-ag

build-tui:
	go build -o bin/safe-ag-tui ./tui

build-all: build build-tui

install: build build-tui
	cp bin/safe-ag /usr/local/bin/safe-ag
	cp bin/safe-ag-tui /usr/local/bin/safe-ag-tui
	cp bin/safe-ag-claude /usr/local/bin/safe-ag-claude
	cp bin/safe-ag-codex /usr/local/bin/safe-ag-codex

test:
	go test ./...

clean:
	rm -f bin/safe-ag bin/safe-ag-tui
