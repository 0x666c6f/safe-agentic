.PHONY: build dev test clean

build:
	wails3 build

dev:
	wails3 dev

test:
	go test ./...
	cd frontend && npx vitest run --passWithNoTests

clean:
	rm -rf bin frontend/dist frontend/node_modules
