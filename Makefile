# Build, test and publish the TypeScript (ts/), Go (go/) and Rust (rs/)
# implementations. ts/ is canonical; go/ and rs/ track it.
#
# Local build/test resolve the @tabnas siblings from the registry, or from
# sibling checkouts linked into node_modules / a go.work (admin/scripts/link.sh).
# The Rust crate resolves them from SIBLING CHECKOUTS only, by path: see
# rs/AGENTS.md.

.PHONY: all build test clean build-ts build-go build-rs test-ts test-go test-rs \
        clean-ts clean-go clean-rs publish-ts publish-go set-version tags-go reset \
        prose prose-counts

all: build test

build: build-ts build-go build-rs

test: test-ts test-go test-rs

clean: clean-ts clean-go clean-rs

# --- Version ---

# Set the release version everywhere: make set-version V=x.y.z
#
# ts/package.json "version" is the source of truth (tabnas.plugin.json
# "versionSource" names it), and five copies must equal it: the two
# package-lock entries npm rewrites, the TS `const VERSION`, the Go
# `const VERSION`, and the Rust pair `version` in rs/Cargo.toml with
# `pub const VERSION` in rs/src/lib.rs. ts/test/version.test.ts,
# go/version_test.go and rs/tests/version_test.rs fail the build when any
# of them drift, so run `make test` after this.
#
# The Rust edits are guarded on rs/ existing, the way ts/embed-grammar.js
# guards its Rust block, so this target still works in a checkout that
# predates the port. The sed for Cargo.toml is anchored to the FIRST
# `version = "..."` under [package]: the file's dependency entries carry
# the same key, and an unanchored substitution would rewrite them too.
set-version:
	@test -n "$(V)" || (echo "Usage: make set-version V=x.y.z" && exit 1)
	@echo "$(V)" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$$' \
	  || (echo "Not a semver version: $(V)" && exit 1)
	cd ts && npm version "$(V)" --no-git-tag-version --allow-same-version >/dev/null
	sed -i.bak "s/^const VERSION = '.*'/const VERSION = '$(V)'/" ts/src/semver.ts
	sed -i.bak 's/^const VERSION = ".*"/const VERSION = "$(V)"/' go/semver.go
	rm -f ts/src/semver.ts.bak go/semver.go.bak
	@if [ -f rs/Cargo.toml ]; then \
	  sed -i.bak '0,/^version = ".*"/s//version = "$(V)"/' rs/Cargo.toml; \
	  sed -i.bak 's/^pub const VERSION: &str = ".*";/pub const VERSION: \&str = "$(V)";/' rs/src/lib.rs; \
	  rm -f rs/Cargo.toml.bak rs/src/lib.rs.bak; \
	fi
	@echo "--- version set to $(V) ---"
	@grep -m1 '"version"' ts/package.json
	@grep -h '^const VERSION' ts/src/semver.ts go/semver.go
	@test -f rs/Cargo.toml && grep -m1 '^version = ' rs/Cargo.toml || true
	@test -f rs/src/lib.rs && grep -h '^pub const VERSION' rs/src/lib.rs || true

# --- TypeScript (package in ts/) ---
build-ts:
	cd ts && npm run build

test-ts:
	cd ts && npm test

clean-ts:
	rm -rf ts/dist ts/dist-test

# Publish the TypeScript package at its current package.json version.
publish-ts: test-ts
	cd ts && npm publish --access public

# --- Go (module in go/) ---
build-go:
	cd go && go build ./...

test-go:
	cd go && go test -v ./...

clean-go:
	cd go && go clean

# --- Rust (crate in rs/) ---
#
# The engine, the ABNF compiler, the fixture runner and the debug plugin
# are path dependencies on sibling checkouts (parser, abnf, bnf, support,
# debug), so there is nothing to fetch and nothing to link.
build-rs:
	cd rs && cargo build --all-targets

# `--all-targets` does NOT include doctests, and rs/README.md is
# doctested, so the doc run is a second command rather than a flag.
test-rs:
	cd rs && cargo test --all-targets
	cd rs && cargo test --doc
	cd rs && cargo clippy --all-targets --all-features -- -D warnings

clean-rs:
	cd rs && cargo clean

# Publish the Go module: make publish-go V=x.y.z
# Injects V into the Go `VERSION` const, commits, tags go/vX.Y.Z, and
# (when gh is available) creates a GitHub release.
publish-go: test-go
	@test -n "$(V)" || (echo "Usage: make publish-go V=x.y.z" && exit 1)
	sed -i.bak 's/^const VERSION = ".*"/const VERSION = "$(V)"/' go/semver.go
	rm -f go/semver.go.bak
	git add go/semver.go
	git commit -m "go: v$(V)"
	git tag go/v$(V)
	git push origin main go/v$(V)
	@command -v gh >/dev/null 2>&1 && gh release create go/v$(V) --title "go/v$(V)" --notes "Go module release v$(V)" || true

# List published Go module tags, newest first.
tags-go:
	git tag -l 'go/v*' --sort=-version:refname

reset:
	cd ts && npm run reset
	cd go && go clean -cache && go build ./... && go test -v ./...

# The prose gate (see docs/STYLE-GUIDE.md). Vale over the reader-facing
# pages, at the levels set in .vale.ini, on the same file list
# ts/test/docs.test.js reads. Requires `vale` on PATH and one
# `vale sync`. Warnings are advisory, errors fail.
prose:
	vale --minAlertLevel=error $$(node ts/scripts/gated-docs.cjs)
	node ts/scripts/vale-counts.cjs

# Re-measure what .vale.ini and the style guide record, after
# a change to the pages or to the rules moves the numbers.
prose-counts:
	node ts/scripts/vale-counts.cjs --write
