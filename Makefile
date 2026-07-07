.PHONY: build build-all build-tui build-app build-detonate install test validate-skills clean

VERSION ?= dev

build:
	go build -ldflags "-X main.Version=$(VERSION)" -o bin/berth ./cmd/berth

build-tui:
	go build -o bin/berth-tui ./tui

build-app:
	$(MAKE) -C app build

build-detonate:
	go build -o bin/detonate ./cmd/detonate

build-all: build build-tui build-detonate

install: build build-tui
	cp bin/berth /usr/local/bin/berth
	cp bin/berth-tui /usr/local/bin/berth-tui
	cp bin/berth-claude /usr/local/bin/berth-claude
	cp bin/berth-codex /usr/local/bin/berth-codex

test:
	go test ./...

validate-skills:
	go run ./tools/validate-skills

clean:
	rm -f bin/berth bin/berth-tui bin/detonate
