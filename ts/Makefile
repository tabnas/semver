.PHONY: all build test clean build-ts build-go build-rs test-ts test-go test-rs clean-ts clean-go clean-rs publish-go tags-go tidy-go reset

all: build test

build: build-ts build-go build-rs

test: test-ts test-go test-rs

clean: clean-ts clean-go clean-rs

# TypeScript
build-ts:
	npm run build

test-ts:
	npm test

clean-ts:
	rm -rf dist dist-test

# Go
build-go:
	cd ../go && go build ./...

test-go:
	cd ../go && go test ./...

clean-go:
	cd ../go && go clean -cache

# Rust. The engine, the ABNF compiler and the fixture runner are path
# dependencies on sibling checkouts, so there is nothing to fetch.
# `--all-targets` does NOT include doctests, and ../rs/README.md is
# doctested, so the doc run is a second command rather than a flag.
build-rs:
	cd ../rs && cargo build --all-targets

test-rs:
	cd ../rs && cargo test --all-targets
	cd ../rs && cargo test --doc
	cd ../rs && cargo clippy --all-targets --all-features -- -D warnings

clean-rs:
	cd ../rs && cargo clean

# Publish Go module: make publish-go V=0.1.7
publish-go: test-go
	@test -n "$(V)" || (echo "Usage: make publish-go V=x.y.z" && exit 1)
	sed -i '' 's/^const VERSION = ".*"/const VERSION = "$(V)"/' ../go/semver.go
	git add ../go/semver.go
	git commit -m "go: v$(V)"
	git tag go/v$(V)
	git push origin main go/v$(V)
	if command -v gh >/dev/null 2>&1; then gh release create go/v$(V) --title "go/v$(V)" --notes "Go module release v$(V)"; fi

tidy-go:
	cd ../go && go mod tidy

tags-go:
	git tag -l 'go/v*' --sort=-version:refname

reset:
	npm run reset
	cd ../go && go clean -cache
	cd ../go && go build ./...
	cd ../go && go test -v ./...
