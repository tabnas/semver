# semver (Go)

A tabnas grammar plugin that parses
[Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html) version
strings into Go values, exactly as the specification defines them. The
parser is the specification's own grammar, compiled from ABNF by
[`@tabnas/abnf`](https://github.com/tabnas/abnf) at build time. The
module embeds the compiled rule set and imports no compiler.

## Install

```bash
go get github.com/tabnas/semver/go@latest
```

```go
import tabnassemver "github.com/tabnas/semver/go"
```

## One example

`tabnassemver.Parse` is the one-call entry point: pass a string, get a
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

- [Tutorial](doc/tutorial.md). A guided first parse, start to finish.
- [How-to guide](doc/guide.md). Short recipes for individual tasks.
- [Reference](doc/reference.md). The public API, the value shape, and
  the complete syntax accepted.
- [Concepts](doc/concepts.md). How the plugin turns the specification's
  grammar into a parser, and how the Go version differs from TypeScript.

For the canonical TypeScript implementation, see
[`../ts/README.md`](../ts/README.md).

## Grammar

The grammar is defined once in the top-level
[`semver-grammar.abnf`](../semver-grammar.abnf). The TypeScript build
compiles it into [`semver-grammar.json`](../semver-grammar.json), the
engine's rule set, and copies that file here
([`semver-grammar.json`](semver-grammar.json), embedded with
`//go:embed`); the plugin installs it. [`semver.go`](semver.go) embeds
the ABNF text as well, and exports it, as `Grammar`, for
tooling that wants it. Edit the grammar in the top-level file, never in
a generated copy, and run `npm run gen-grammar` from `../ts`.

## C library

[`clib/`](clib/) builds `libtabnassemver`, the parser as a C shared library
with the fleet's uniform five-symbol ABI, for languages with no tabnas
port.

## License

Copyright (c) 2026 Richard Rodger and other contributors, MIT License.
