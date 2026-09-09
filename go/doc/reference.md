# Reference (Go)

The complete public surface of the Go `semver` module: every exported
symbol with its signature, the value a parse returns, the exact syntax
accepted and rejected, the tokens and lexer configuration the plugin
installs, and the error contract. For a guided introduction see the
[tutorial](tutorial.md); for task recipes see the [how-to guide](guide.md);
for how the plugin works (and how it differs from TypeScript) see
[concepts](concepts.md).

## Module

```bash
go get github.com/tabnas/semver/go@latest
```

```go
import (
    tabnas "github.com/tabnas/parser/go"
    tabnassemver "github.com/tabnas/semver/go"
)
```

| | |
|---|---|
| Module | `github.com/tabnas/semver/go` |
| Package | `tabnassemver` |
| Engine | `github.com/tabnas/parser/go` (imported as `tabnas`) |
| Compiler | `github.com/tabnas/abnf/go` — compiles the grammar at install time; pulls in `github.com/tabnas/bnf/go` |
| Test-only | `github.com/tabnas/support/go` — the shared-fixture runner |
| Grammar | [`semver-grammar.abnf`](../../semver-grammar.abnf), embedded verbatim in [`semver.go`](../semver.go) |
| Options | none — `Defaults` is an empty map |

The dependencies are required at the versions pinned in
[`go.mod`](../go.mod); there is no `replace` directive.

## Public API

Nine exported names: `Parse`, `Compare` and `Format` over values; `Make`
and `Semver` to build an engine; `Defaults`, `Grammar`, `VERSION` and
`MaxSafeInteger`.

### `func Parse(src string) (any, error)`

Parses one version string. On success the value is the `map[string]any`
described under [Value types](#value-types); on failure the value is
`nil` and the error a `*tabnas.TabnasError` with `Code == "unexpected"`
(see [Errors](#errors)). `Parse` uses one package-level engine, built on
first use with `sync.Once` and guarded by a mutex, so it never recompiles
the grammar and is safe for concurrent use.

```go
v, err := tabnassemver.Parse("1.2.3-alpha.1+build.5")
// v == map[string]any{
//   "major": float64(1), "minor": float64(2), "patch": float64(3),
//   "prerelease": []any{"alpha", float64(1)},
//   "build":      []any{"build", "5"},
// }
```

### `func Make() *tabnas.Tabnas`

Returns a new engine with the plugin installed: `tabnas.Make()` followed
by `j.UseDefaults(Semver, Defaults)`. Build one and reuse it — installing
compiles the grammar, which dominates a parse by orders of magnitude. The
instance is **not** safe for concurrent `Parse` calls; use
`tabnassemver.Parse` for a shared one, or one instance per goroutine.
`Make` panics if the plugin fails to install, which cannot happen with
the embedded grammar: a panic here is a broken build, not bad input.

```go
j := tabnassemver.Make()
v, err := j.Parse("1.2.3") // func (j *tabnas.Tabnas) Parse(src string) (any, error)
```

`j.Parse` returns exactly what `tabnassemver.Parse` returns, and a failed
parse does not affect the next one on the same instance.

### `func Semver(j *tabnas.Tabnas, _ map[string]any) error`

The plugin function, of the engine's `tabnas.Plugin` type
(`func(j *tabnas.Tabnas, opts map[string]any) error`). Install it with
`j.Use(Semver)` or `j.UseDefaults(Semver, Defaults)`; the options map is
ignored, as there are no options. Installing compiles `Grammar` with
`abnf.Abnf(grammarText, &abnf.AbnfConvertOptions{Start: "semver", Tag: "semver"})`,
attaches the plugin's one action — `@semver:ac`, an after-close hook on
the `semver` rule that replaces the parse tree with the value — and
applies the [lexer configuration](#tokens-and-lexer-configuration) and
the `unexpected` [hint](#errors), all through one `j.Grammar(spec)` call.

It is idempotent: the first call sets the decoration `semver-init` on the
instance, and a later call returns `nil` without compiling again. It
returns an error only when the grammar fails to compile
(`semver: grammar failed to compile: ...`) or the action cannot be
attached, neither of which is reachable with the embedded grammar.

```go
j := tabnas.Make()
if err := j.Use(tabnassemver.Semver); err != nil {
    panic(err)
}
```

### `func Compare(a, b any) (int, error)`

Orders two parsed values by precedence as §11 of the specification
defines it: `-1` when `a` ranks below `b`, `1` when above, `0` when they
are the same version.

1. `major`, `minor`, `patch`, numerically, in that order; a `float64` and
   a `*big.Int` compare exactly.
2. A version with a pre-release ranks below the same version without one.
3. Pre-release identifiers left to right: numeric ones numerically,
   alphanumeric ones in ASCII order (a Go string comparison), and a
   numeric identifier below any alphanumeric one.
4. When every shared identifier is equal, the longer list ranks higher.
5. Build metadata is ignored: `1.0.0+a` and `1.0.0+b` compare `0`.

So `1.0.0-alpha` < `1.0.0-alpha.1` < `1.0.0-alpha.beta` < `1.0.0-beta` <
`1.0.0-beta.2` < `1.0.0-beta.11` < `1.0.0-rc.1` < `1.0.0` < `2.0.0` <
`2.1.0` < `2.1.1`, pair by pair.

```go
a, _ := tabnassemver.Parse("1.0.0-beta.2")
b, _ := tabnassemver.Parse("1.0.0-beta.11")
c, err := tabnassemver.Compare(a, b) // -1, nil
```

`Compare` returns `0` and an error when an argument is not a parsed
value: `semver: not a parsed version (want map[string]any)` for a
non-map, `semver: prerelease is not a list`, or
`semver: major: not a number: string` (likewise `minor`, `patch`,
`prerelease`) when a component is neither `float64` nor `*big.Int`, or
`semver: major: not an integer: 1.5` when a fractional `float64` meets a
`*big.Int` (two `float64` values compare without that check).

### `func Format(v any) (string, error)`

Renders a parsed value back to its version string. For a value that came
out of `Parse` the result is the input, byte for byte; numbers render as
plain digits (a `*big.Int` through its `String` method).

```go
v, _ := tabnassemver.Parse("2.0.0-rc.1+sha.abc")
s, err := tabnassemver.Format(v) // "2.0.0-rc.1+sha.abc", nil
```

A hand-built map of the same shape is accepted. Anything else returns
`""` and an error: `semver: not a parsed version (want map[string]any)`;
`semver: major: not a non-negative integer: 1.5` for a fractional or
negative `float64`, or `semver: major: not a number: <type>`;
`semver: prerelease is not a list` / `semver: build is not a list`;
`semver: prerelease: ...` for an element that is neither string nor
number; and `semver: build identifier is not a string`.

### `var Defaults = map[string]any{}`

The default option map, paired with `Semver` for `UseDefaults`. Empty:
the plugin has no options. It mirrors `Semver.defaults` in TypeScript.

### `const Grammar`

The grammar as ABNF text — the same string the plugin compiles, which is
[`semver-grammar.abnf`](../../semver-grammar.abnf) verbatim, comments
included. Exported for tooling; it is the constant the plugin itself
compiles (`Grammar = grammarText`), not a second copy.

### `const VERSION = "0.1.0"`

The module's version. It equals `ts/package.json` `"version"` and the
TypeScript `VERSION`; `version_test.go` reads `ts/package.json` and fails
(never skips) when this constant drifts from it.

### `const MaxSafeInteger = 1<<53 - 1`

`9007199254740991`, JavaScript's `Number.MAX_SAFE_INTEGER`: the largest
integer a numeric component is returned as a `float64` for. Anything
larger is a `*big.Int`. Both runtimes switch representation here.

## Value types

`Parse` returns `any`; the concrete value is always a `map[string]any`
with these five keys, all present on every successful parse:

| Key | Go type | Content |
|---|---|---|
| `major`, `minor`, `patch` | `float64` or `*big.Int` | the version core, exact |
| `prerelease` | `[]any` | one element per pre-release identifier: `string`, `float64` or `*big.Int`; `[]any{}` (non-nil, empty) when there is none |
| `build` | `[]any` | one `string` per build identifier; `[]any{}` when there is none |

**Integers.** A digit string whose value is at most `MaxSafeInteger` is a
`float64`; beyond that it is a `*big.Int` from `math/big`. Both are
exact: `9007199254740991.0.0` gives `float64(9007199254740991)`,
`9007199254740992.0.0` a `*big.Int`, never a rounded neighbour. Code that
reads a component handles both:

```go
switch n := m["major"].(type) {
case float64: // exact, 0 <= n <= MaxSafeInteger
case *big.Int: // above MaxSafeInteger
}
```

**Pre-release identifiers.** An identifier that is all digits is numeric
and becomes a `float64` (or `*big.Int`, at the same boundary); the
grammar has already excluded a leading zero there. Any other identifier —
one with a letter or hyphen anywhere in it, leading zeros included — is a
`string`: `1.0.0-0.10.a1.1a.01a.-1` gives
`[]any{float64(0), float64(10), "a1", "1a", "01a", "-1"}`. The
distinction carries the specification's kind-dependent comparison rules
(§11.4.1–11.4.3): numeric identifiers compare numerically, alphanumeric
ones in ASCII order, and a numeric one ranks below any alphanumeric one.

**Build identifiers** are always strings, digits included: `1.0.0+001`
gives `[]any{"001"}`. Build metadata takes no part in precedence and may
carry leading zeros, so a number would lose information.

## Syntax accepted

The plugin accepts exactly the language of the specification's grammar
("Backus–Naur Form Grammar for Valid SemVer Versions" in the
[specification](https://semver.org/spec/v2.0.0.html)). These are the
productions of [`semver-grammar.abnf`](../../semver-grammar.abnf) with
their comments stripped; every name is the specification's with spaces
written as hyphens, and `semver` is an alias of the root added as the
plugin's entry point.

```abnf
semver                 = valid-semver
valid-semver           = version-core [ "-" pre-release ] [ "+" build ]
version-core           = major "." minor "." patch
major                  = numeric-identifier
minor                  = numeric-identifier
patch                  = numeric-identifier
pre-release            = pre-release-identifier *( "." pre-release-identifier )
build                  = build-identifier *( "." build-identifier )
pre-release-identifier = "0" [ *digit alphanumeric-tail ]
                       / positive-digit *digit [ alphanumeric-tail ]
                       / alphanumeric-tail
alphanumeric-tail      = non-digit *identifier-character
build-identifier       = 1*identifier-character
numeric-identifier     = "0" / positive-digit *digit
identifier-character   = digit / non-digit
non-digit              = letter / "-"
digit                  = "0" / positive-digit
positive-digit         = %x31-39
letter                 = %x41-5A / %x61-7A
```

`pre-release-identifier` and `build-identifier` differ from the
specification's shape without changing its language — the first is
factored on its initial character, the second is the union of
`<alphanumeric identifier>` and `<digits>`; the file explains both
inline, and [concepts](concepts.md) covers why the compiler needs them.

### Accepted

| Input | Value |
|---|---|
| `1.0.0` | `major` `float64(1)`, `minor` `float64(0)`, `patch` `float64(0)`, `prerelease` `[]any{}`, `build` `[]any{}` |
| `1.0.0-alpha.1` | `prerelease` `[]any{"alpha", float64(1)}` |
| `1.0.0-0.3.7` | `prerelease` `[]any{float64(0), float64(3), float64(7)}` |
| `1.0.0-x.7.z.92` | `prerelease` `[]any{"x", float64(7), "z", float64(92)}` |
| `1.0.0-x-y-z.--` | `prerelease` `[]any{"x-y-z", "--"}` |
| `1.0.0-01a` | `prerelease` `[]any{"01a"}` — alphanumeric, so leading zeros are allowed |
| `1.0.0-alpha+001` | `prerelease` `[]any{"alpha"}`, `build` `[]any{"001"}` |
| `1.0.0+20130313144700` | `build` `[]any{"20130313144700"}` |
| `1.0.0-beta+exp.sha.5114f85` | `prerelease` `[]any{"beta"}`, `build` `[]any{"exp", "sha", "5114f85"}` |
| `1.0.0+21AF26D3----117B344092BD` | `build` `[]any{"21AF26D3----117B344092BD"}` |
| `1.2.3-1.2.3-1.2.3` | `prerelease` `[]any{float64(1), float64(2), "3-1", float64(2), float64(3)}` |

### Rejected

Every one of these is an `unexpected` error; the full list the fixtures
pin is [`test/spec/strict.tsv`](../../test/spec/strict.tsv).

| Input | Why |
|---|---|
| `` (empty) | not a version; `lex.empty` is off |
| `1`, `1.2` | incomplete version core |
| `1.2.3.4` | too many components |
| `01.2.3`, `1.02.3` | leading zero in the version core |
| `v1.2.3` | prefix; `v` is not in the grammar |
| ` 1.2.3`, `1.2.3 `, `1.2.3\n` | whitespace, anywhere |
| `1.2.3-`, `1.2.3+` | empty pre-release / build |
| `1.2.3-01` | leading zero in a numeric pre-release identifier |
| `1.2.3-a..b` | empty identifier |
| `1.2.3-a_b` | `_` is not an identifier character |
| `1.2.3-α` | non-ASCII |
| `1.2.3+a+b` | a second `+` section |

## Tokens and lexer configuration

The compiled grammar brings its own tokens, and the plugin turns
everything else the engine lexes by default off, so one character is one
token and a character the grammar does not name has no matcher at all.

**Four fixed tokens**, one per literal, named by the compiler in
allocation order, and **three character-class tokens**, one per `%x`
range, as anchored regular expressions on `spec.Options.Match.Token`,
each of the three marked eager in `spec.Options.Match.TokenEager` so it
can be lexed at any lookahead slot (fixed tokens carry no such flag):

| Token | Source | Role |
|---|---|---|
| `#0` | `0` | the digit zero, singled out by `digit`, `numeric-identifier` and `pre-release-identifier` |
| `#T` | `.` | separator |
| `#T1` | `-` | opens the pre-release; also an identifier character |
| `#T2` | `+` | opens the build metadata |
| `#RX___X_0031___X_0039` | `^[\x{0031}-\x{0039}]` | `positive-digit` (`1`–`9`) |
| `#RX___X_0041___X_005A` | `^[\x{0041}-\x{005a}]` | `letter` (`A`–`Z`) |
| `#RX___X_0061___X_007A` | `^[\x{0061}-\x{007a}]` | `letter` (`a`–`z`) |

The names appear in diagnostics (`token.name` and `expected` below); the
TypeScript engine spells the class names differently
(`#RX___U0031__U0039`, ...). The seven are pairwise disjoint, so no
character can be lexed two ways.

**Everything else is off.** On the compiled spec's options the plugin
sets:

| Option | Value | Effect |
|---|---|---|
| `Space.Lex`, `Line.Lex`, `Comment.Lex`, `String.Lex`, `Number.Lex`, `Text.Lex`, `Value.Lex` | `false` | a blank, tab, newline, `#`, `//`, quote, bare word or keyword is rejected, not skipped or swallowed; digits are matched by the grammar's tokens, never the number lexer |
| `Fixed.Token["#OB"]`, `"#CB"`, `"#OS"`, `"#CS"`, `"#CL"`, `"#CA"` | `nil` | the engine's JSON punctuation `{ } [ ] : ,` is unbound |
| `Lex.Empty` | `false` | the empty string is an error rather than the engine's default `nil` result |
| `Hint["unexpected"]` | text | the hint under [Errors](#errors) |

Turning any of these back on makes the plugin accept strings the
specification rejects. The Go toolchain needed neither of the two
TypeScript fixes recorded in [`AGENTS.md`](../../AGENTS.md): the Go
emitter has always marked classes eager and the Go lexer has always tried
a rule's expected tokens first.

## Grammar group tag

Every alternate the plugin installs carries the group tag `semver` (the
compiler's `Tag` option; the `__start__` wrapper's close alternate
carries `semver,end`). The tag identifies the plugin's alternates in
introspection. Excluding it removes the whole grammar — the plugin
installs nothing else — so every input is then rejected with
`unexpected` at column 1:

```go
j := tabnassemver.Make()
j.SetOptions(tabnas.Options{Rule: &tabnas.RuleOptions{Exclude: "semver"}})
_, err := j.Parse("1.2.3") // unexpected character(s): 1
```

## Errors

`Parse`, `j.Parse`, `Compare` and `Format` return an `error`; nothing
panics on input. A parse error is always the engine's
`*tabnas.TabnasError`, and its `Code` is always `"unexpected"`:

```go
_, err := tabnassemver.Parse("v1.2.3")
if te, ok := err.(*tabnas.TabnasError); ok {
    te.Code, te.Row, te.Col, te.Src // "unexpected", 1, 1, "v"
}
```

| Field | Type | Content |
|---|---|---|
| `Code` | `string` | always `"unexpected"` |
| `Detail` | `string` | `unexpected character(s): <src>` |
| `Row`, `Col` | `int` | 1-based line and column of the offending character |
| `Pos` | `int` | 0-based offset of the offending character (bytes; the JSON `pos` below is in characters) |
| `Src` | `string` | the offending text — one character, or `""` when the failure is at the end of the input (`1.0.0-01`, the empty string) |
| `Hint` | `string` | the plugin's explanation of what a version must look like |

`err.Error()` renders the engine's multi-line message — the
`[tabnas/unexpected]: unexpected character(s): v` header, the location
`--> <no-file>:1:1`, the source line with a caret under the character,
the hint, and an `--internal:` suffix naming the rule and token. The text
carries ANSI colour escapes unless the engine's `Options.Color.Active` is
set to `false`. The hint, as the plugin registers it under
`Hint["unexpected"]`, reads (`{src}` is replaced by the offending text in
both `te.Hint` and the JSON `hint` below):

```
The character(s) {src} do not match any rule alternative active at
this position.

A Semantic Version is MAJOR.MINOR.PATCH — three integers with no
leading zeros — optionally followed by -PRERELEASE and +BUILD, each a
dot-separated list of non-empty identifiers made of [0-9A-Za-z-],
where a numeric pre-release identifier has no leading zero. Nothing
else is allowed: no whitespace, no "v" prefix, no empty identifier.
See https://semver.org/spec/v2.0.0.html
```

`json.Marshal(err)` gives the engine's structured diagnostic, the same
shape `JSON.stringify` gives in TypeScript:

```json
{"status":"failure","code":"unexpected","message":"unexpected character(s): v",
 "hint":"...","row":1,"col":1,"pos":0,"len":1,"rule":"valid-semver",
 "ruleStack":["__start__","semver","valid-semver"],
 "token":{"name":"#RX___X_0061___X_007A","src":"v"},
 "expected":["#0","#RX___X_0031___X_0039"],"src":"v1.2.3",
 "plugins":["Semver"],"version":"0.9.0"}
```

Here `src` is the whole source line, `len` the length of the offending
text in characters, `expected` the tokens some alternate of the failing
rule would have accepted, and `version` the engine's version, not the
plugin's.

**Only one code.** The plugin declares no error codes of its own; every
rejection is `unexpected`, raised where the grammar has no alternative
for the next character. That is a decision, not a gap: the grammar is the
sole acceptor, and because the compiler inlines leading rule references,
a trap production for a better-labelled error would either never be
pushed or would change the accepted language
([`AGENTS.md`](../../AGENTS.md#error-codes)). The hint is where a reader
learns what a version has to look like; the fixtures pin
`ERROR:unexpected` exactly.

**Positions are not part of the contract.** Where a rejection needs
lookahead the reported column can differ between runtimes: `01.2.3` is
rejected by both, with Go pointing at the `0` (column 1) and TypeScript
at the `1`. Compare on `Code`, not on `Col`.

## Concurrency

- `tabnassemver.Parse` is safe for concurrent use: it owns one cached
  engine and serialises callers through a mutex.
- A `*tabnas.Tabnas` from `Make`, or any engine with `Semver` installed,
  is not safe for concurrent `Parse` calls. Use it from one goroutine,
  guard it yourself, or build one per goroutine.
- `Compare` and `Format` are pure functions of their arguments, and
  `Parse` returns a fresh map on every call.

## Performance

Installing the plugin compiles the ABNF into the engine's rule set —
about 10 ms in Go — and a parse of a typical version takes about 100 µs.
Build one engine and reuse it, or call `tabnassemver.Parse`, which does
that for you; `perf_test.go` pins `Parse` to within a small factor of
instance reuse, so a regression that rebuilt the grammar per call fails
the suite. Compiling happens once per instance, never per parse.

## C library

[`clib/`](../clib/) builds `libtabnassemver`, this parser as a C shared
library with the fleet's uniform five-symbol ABI (`tabnas_version`,
`tabnas_grammar`, `tabnas_parse`, `tabnas_grammar_free`, `tabnas_free`):
every call returns JSON, a rejection is an answer
(`ok:true, accept:false` with the diagnostic above) rather than a
failure, a value with a `*big.Int` component is reported as `valueError`
rather than as a rounded JSON number, and handles are safe to use from
several threads. See
[`clib/README.md`](../clib/README.md) for the contract, the build script
and the header.
