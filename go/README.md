# semver (Go)

A tabnas grammar plugin that parses
[Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html) version
strings into Go values, exactly as the specification defines them. The
parser is the specification's own grammar, compiled from ABNF by
[`github.com/tabnas/abnf/go`](https://github.com/tabnas/abnf) when the
plugin is installed.

## Install

```bash
go get github.com/tabnas/semver/go@latest
```

```go
import tabnassemver "github.com/tabnas/semver/go"
```

## One example

`tabnassemver.Parse` is the one-call entry point — pass a string, get a
value and an `error`:

```go
v, err := tabnassemver.Parse("1.2.3-alpha.1+build.5")
// map[string]any{
//   "major": float64(1), "minor": float64(2), "patch": float64(3),
//   "prerelease": []any{"alpha", float64(1)},
//   "build":      []any{"build", "5"},
// }

c, err := tabnassemver.Compare(a, b) // -1, 0 or 1 by precedence (§11)
s, err := tabnassemver.Format(v)     // "1.2.3-alpha.1+build.5"
```

Integer components come back as `float64`, or as `*big.Int` above
2^53 − 1. `Parse` reuses a cached engine and is safe for concurrent use;
for a hot loop, build one instance with `tabnassemver.Make` and reuse it
on one goroutine.

## Documentation

Full documentation follows the [Diátaxis](https://diataxis.fr)
framework:

- [Tutorial](doc/tutorial.md) — a guided first parse, start to finish.
- [How-to guide](doc/guide.md) — short recipes for individual tasks.
- [Reference](doc/reference.md) — the public API, the value shape, and
  the complete syntax accepted.
- [Concepts](doc/concepts.md) — how the plugin turns the specification's
  grammar into a parser, and how the Go version differs from TypeScript.

For the canonical TypeScript implementation, see
[`../ts/README.md`](../ts/README.md).

## Grammar

The grammar is defined once in the top-level
[`semver-grammar.abnf`](../semver-grammar.abnf) and embedded into this Go
source ([`semver.go`](semver.go)) and the TypeScript source during the
build. Edit the grammar there, not in the generated source. It is also
exported, as `Grammar`, for tooling that wants the text.

## C library

[`clib/`](clib/) builds `libtabnassemver`, the parser as a C shared library
with the fleet's uniform five-symbol ABI, for languages with no tabnas
port.

## License

Copyright (c) 2026 Richard Rodger and other contributors, MIT License.
