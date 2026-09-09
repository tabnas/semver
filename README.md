# @tabnas/semver

<!-- tabnas-badges -->
[![npm](https://tabnas.github.io/status/badges/semver-npm.svg)](https://www.npmjs.com/package/@tabnas/semver)
[![CI](https://github.com/tabnas/semver/actions/workflows/ci.yml/badge.svg)](https://github.com/tabnas/semver/actions/workflows/ci.yml)
[![go](https://tabnas.github.io/status/badges/semver-go.svg)](https://pkg.go.dev/github.com/tabnas/semver/go)
[![tabnas standard](https://tabnas.github.io/status/badges/semver-standard.svg)](https://tabnas.github.io/status/)
<!-- /tabnas-badges -->

A grammar plugin that teaches the [Tabnas](https://github.com/tabnas/parser)
parser to read [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html)
version strings — `1.2.3-alpha.1+build.5` — exactly as the specification
defines them. Available for both TypeScript and Go, built from one ABNF
grammar.

Docs, guides, the error reference and the playground: **[tabnas.dev](https://tabnas.dev)**.

## Install

```bash
# TypeScript / JavaScript
npm install @tabnas/parser @tabnas/abnf @tabnas/semver

# Go
go get github.com/tabnas/semver/go@latest
```

## One tiny example

**TypeScript** — the plugin installs on a bare Tabnas engine:

```js
import { Tabnas } from '@tabnas/parser'
import { Semver, compare, format } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

tn.parse('1.2.3-alpha.1+build.5')
// => { major: 1, minor: 2, patch: 3, prerelease: ['alpha', 1], build: ['build', '5'] }

compare(tn.parse('1.0.0-beta.2'), tn.parse('1.0.0-beta.11')) // => -1
format(tn.parse('2.0.0-rc.1+sha.abc'))                        // => '2.0.0-rc.1+sha.abc'
```

Anything the specification does not allow is a parse error — a `v`
prefix, a blank, a leading zero, an empty identifier:

```js
import { Tabnas } from '@tabnas/parser'
import { Semver } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

let code
try { tn.parse('v1.2.3') } catch (e) { code = e.code }
code // => 'unexpected'
```

**Go** — `tabnassemver.Parse` is the one-call entry point:

```go
import tabnassemver "github.com/tabnas/semver/go"

v, err := tabnassemver.Parse("1.2.3-alpha.1+build.5")
// map[string]any{
//   "major": float64(1), "minor": float64(2), "patch": float64(3),
//   "prerelease": []any{"alpha", float64(1)},
//   "build":      []any{"build", "5"},
// }
```

## The grammar is the parser

The specification publishes its grammar in BNF. This plugin transcribes
it into RFC 5234 ABNF — [`semver-grammar.abnf`](semver-grammar.abnf), one
production per production of the specification, with the same names —
and [`@tabnas/abnf`](https://github.com/tabnas/abnf) compiles that text
into the engine's rule set when the plugin is installed. Nothing in the
plugin's code decides what a valid version is. The two places the grammar
departs from the specification's *shape* (never its language) are
explained inline in the file.

```abnf
valid-semver = version-core [ "-" pre-release ] [ "+" build ]
version-core = major "." minor "." patch
pre-release  = pre-release-identifier *( "." pre-release-identifier )
build        = build-identifier *( "." build-identifier )
```

## Conformance

`@tabnas/semver` accepts exactly the strings the semver.org grammar
accepts, and produces the parts the specification names for each. The
judge is the regular expression semver.org publishes for the purpose: both
runtimes generate the same corpus of **58,449 strings** — every string of
length up to five over an eight-character alphabet, every short
pre-release and build tail on five version-core shapes, thousands of
mutated valid versions, and random noise — and check verdict, value and
round-trip against it on every test run. The census is pinned in both
runtimes:

| Corpus | Strings | Accepted | Rejected |
|---|---|---|---|
| exhaustive (length ≤ 5 over `019aZ-.+`) | 37,449 | 27 | 37,422 |
| structured (cores × pre-release tails × build tails) | 17,000 | 1,634 | 15,366 |
| mutation (1–3 random edits of valid versions) | 3,000 | 838 | 2,162 |
| random (length ≤ 12 over a wide alphabet) | 1,000 | 0 | 1,000 |

Identical in both runtimes. Two representation decisions, both about
values the specification leaves unbounded:

- an integer component above `Number.MAX_SAFE_INTEGER` (2^53 − 1) is a
  `bigint` (TypeScript) / `*big.Int` (Go) rather than silently rounded;
- a numeric pre-release identifier is a number and an alphanumeric one a
  string, because the specification compares the two kinds differently.

See [`AGENTS.md`](AGENTS.md#conformance-claim) for the full details.

## Documentation

Full documentation follows the [Diátaxis](https://diataxis.fr)
framework — one file per quadrant, per language:

| | TypeScript | Go |
|---|---|---|
| **Tutorial** (learning) | [ts/doc/tutorial.md](ts/doc/tutorial.md) | [go/doc/tutorial.md](go/doc/tutorial.md) |
| **How-to guide** (tasks) | [ts/doc/guide.md](ts/doc/guide.md) | [go/doc/guide.md](go/doc/guide.md) |
| **Reference** (API + syntax) | [ts/doc/reference.md](ts/doc/reference.md) | [go/doc/reference.md](go/doc/reference.md) |
| **Concepts** (explanation) | [ts/doc/concepts.md](ts/doc/concepts.md) | [go/doc/concepts.md](go/doc/concepts.md) |

Per-language hubs: [`ts/README.md`](ts/README.md),
[`go/README.md`](go/README.md).

## License

MIT. Copyright (c) Richard Rodger.
