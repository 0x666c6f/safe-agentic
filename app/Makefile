.PHONY: build bundle dev test clean

# Bare binary (fast; no dock icon — macOS shows icons only for .app bundles).
build:
	wails3 build

# Signed .app bundle with the shield dock icon. Use this for real runs.
bundle:
	wails3 package

dev:
	wails3 dev

test:
	go test ./...
	cd frontend && npx vitest run --passWithNoTests

clean:
	rm -rf bin frontend/dist frontend/node_modules
