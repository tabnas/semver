# Build, test and publish both the TypeScript (ts/) and Go (go/)
# implementations. ts/ is canonical; go/ tracks it.
#
# Local build/test resolve the @tabnas siblings from the registry, or from
# sibling checkouts linked into node_modules / a go.work (admin/scripts/link.sh).

.PHONY: all build test clean build-ts build-go test-ts test-go \
        clean-ts clean-go publish-ts publish-go set-version tags-go reset

all: build test

build: build-ts build-go

test: test-ts test-go

clean: clean-ts clean-go

# --- Version ---

# Set the release version everywhere: make set-version V=x.y.z
#
# ts/package.json "version" is the source of truth (tabnas.plugin.json
# "versionSource" names it), and three copies must equal it: the two
# package-lock entries npm rewrites, the TS `const VERSION` and the Go
# `const VERSION`. ts/test/version.test.ts and go/version_test.go fail
# the build when any of them drift, so run `make test` after this.
set-version:
	@test -n "$(V)" || (echo "Usage: make set-version V=x.y.z" && exit 1)
	@echo "$(V)" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$$' \
	  || (echo "Not a semver version: $(V)" && exit 1)
	cd ts && npm version "$(V)" --no-git-tag-version --allow-same-version >/dev/null
	sed -i.bak "s/^const VERSION = '.*'/const VERSION = '$(V)'/" ts/src/semver.ts
	sed -i.bak 's/^const VERSION = ".*"/const VERSION = "$(V)"/' go/semver.go
	rm -f ts/src/semver.ts.bak go/semver.go.bak
	@echo "--- version set to $(V) ---"
	@grep -m1 '"version"' ts/package.json
	@grep -h '^const VERSION' ts/src/semver.ts go/semver.go

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
