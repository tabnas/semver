# Tutorial — your first semver parse (Go)

This walks you from nothing to a working parse, then through precedence,
rendering and one error. Follow it in order; each step builds on the
last. When you finish you will have installed the module, parsed a
version with pre-release and build parts, compared two versions,
rendered one back to text, handled a rejection, and set up an instance
for a hot loop.

For a recipe-style index of individual tasks, see the
[how-to guide](guide.md). For exhaustive signatures, the value shape and
the full syntax accepted, see the [reference](reference.md). For how the
specification's grammar becomes the parser — and how the Go version
differs from TypeScript — see [concepts](concepts.md).

## 1. Install

`tabnassemver` is a plugin for the tabnas engine. The engine and the
ABNF compiler it needs are dependencies of the module, so a single
`go get` is enough:

```bash
go get github.com/tabnas/semver/go@latest
```

```go
import tabnassemver "github.com/tabnas/semver/go"
```

## 2. Parse a version

`tabnassemver.Parse` is the one-call entry point. Give it a version
string and it returns the parsed value as `any` plus an `error`:

```go
v, err := tabnassemver.Parse("1.2.3")
// v:   map[string]any{
//        "major": float64(1), "minor": float64(2), "patch": float64(3),
//        "prerelease": []any{}, "build": []any{},
//      }
// err: nil
```

The value is a `map[string]any` with the five keys the specification
names, always all five: `major`, `minor` and `patch` are `float64`, and
`prerelease` and `build` are `[]any` lists — empty here, because `1.2.3`
has neither. Assert the types to read them:

```go
m := v.(map[string]any)
major := m["major"].(float64)         // 1
prerelease := m["prerelease"].([]any) // empty: 1.2.3 has no pre-release
```

A `float64` holds every integer up to `tabnassemver.MaxSafeInteger`
(2^53 − 1) exactly. The specification puts no upper bound on a
component, so anything larger comes back as a `*big.Int` instead of
being rounded; the [reference](reference.md) covers that case.

## 3. Parse a pre-release and build

Now the full shape. Everything after `-` is the pre-release, everything
after `+` is the build metadata, and each is a dot-separated list of
identifiers:

```go
v, err := tabnassemver.Parse("1.2.3-alpha.1+build.5")
// v: map[string]any{
//   "major": float64(1), "minor": float64(2), "patch": float64(3),
//   "prerelease": []any{"alpha", float64(1)},
//   "build":      []any{"build", "5"},
// }
```

Look at the types inside the two lists. In `prerelease`, `alpha` is a
`string` but `1` is a `float64`: a pre-release identifier made only of
digits is *numeric* and comes back as a number; any other identifier is
*alphanumeric* and stays a string. In `build`, `5` is the string `"5"` —
build identifiers are always strings. The distinction is the
specification's own: it compares the two kinds of pre-release
identifier differently (step 4), and build metadata takes no part in
precedence at all. A type switch tells them apart (a numeric identifier
larger than `MaxSafeInteger` is a `*big.Int` from `math/big`, like a
large `major`):

```go
m := v.(map[string]any)
for _, id := range m["prerelease"].([]any) {
	switch id.(type) {
	case string:   // alphanumeric identifier, e.g. "alpha"
	case float64:  // numeric identifier, e.g. 1
	case *big.Int: // numeric identifier above MaxSafeInteger
	}
}
```

Because build identifiers are strings, they keep their leading zeros:
`1.0.0-alpha+001` gives `"build": []any{"001"}`. A numeric pre-release
identifier may not have one at all — `1.2.3-01` is rejected, as you will
see in step 6.

## 4. Compare two versions

`Compare` takes two parsed values and orders them by precedence, as
§11 of the specification defines it: `-1` when the first ranks below the
second, `1` when above, `0` when they are the same version.

```go
a, _ := tabnassemver.Parse("1.0.0-beta.2")
b, _ := tabnassemver.Parse("1.0.0-beta.11")

c, err := tabnassemver.Compare(a, b)
// c:   -1 — beta.2 ranks below beta.11
// err: nil
```

Numeric identifiers compare numerically, which is why `2` ranks below
`11` here rather than after it as text would. The rest of the rules are
the specification's: major, minor and patch numerically; a pre-release
ranks below its normal version (`1.0.0-rc.1` is below `1.0.0`);
identifiers compare left to right, alphanumeric ones in ASCII order and
numeric ones always below alphanumeric ones; and a longer list ranks
above a shorter one that matches its prefix. Build metadata is ignored:

```go
x, _ := tabnassemver.Parse("1.0.0+a")
y, _ := tabnassemver.Parse("1.0.0+b")

c, err = tabnassemver.Compare(x, y)
// c: 0 — build metadata takes no part in precedence
```

`Compare` returns an error if either argument is not a parsed value —
pass it the string `"1.0.0"` instead of the value and you get one.

## 5. Render a version with Format

`Format` turns a value back into its version string. For a value that
came out of `Parse` that is exactly the text you parsed:

```go
s, err := tabnassemver.Format(v)
// s:   "1.2.3-alpha.1+build.5"
// err: nil
```

It works just as well on a map you built yourself in the same shape:

```go
s, err = tabnassemver.Format(map[string]any{
	"major": float64(2), "minor": float64(0), "patch": float64(0),
	"prerelease": []any{"rc", float64(1)},
	"build":      []any{"sha", "abc"},
})
// s: "2.0.0-rc.1+sha.abc"
```

Like `Compare`, it returns an error rather than a string when the
argument is not a version value.

## 6. Handle an error

Anything the specification does not allow is a parse error: a `v`
prefix, a blank, a leading zero on a numeric part, an empty identifier,
the empty string. Nothing is skipped or forgiven, so `Parse` returns
`nil` and a non-nil error — never a panic:

```go
_, err := tabnassemver.Parse("v1.2.3")
// err: non-nil
```

The error is the engine's `*tabnas.TabnasError`. Import the engine
package to name it (its module is already a dependency of the plugin;
`go mod tidy` records the import), then use `errors.As`:

```go
import (
	"errors"

	tabnas "github.com/tabnas/parser/go"
	tabnassemver "github.com/tabnas/semver/go"
)

_, err := tabnassemver.Parse("v1.2.3")

var te *tabnas.TabnasError
if errors.As(err, &te) {
	te.Code // "unexpected"
	te.Row  // 1 — line, 1-based
	te.Col  // 1 — column, 1-based
	te.Src  // "v" — the text the parser stopped at
	te.Hint // what a version has to look like, with a link to semver.org
}
```

`Code` is always `"unexpected"`: the plugin declares no error codes of
its own, because the grammar is the sole judge of what a version is and
every rejection is the same event — a character with no rule to match
it. `Hint` is where a reader learns the rules; `err.Error()` puts it all
together as a multi-line message whose first line reads
`[tabnas/unexpected]: unexpected character(s): v`, followed by the
position, the source line and the hint (coloured with ANSI escapes
unless the engine's colour option is turned off). For logs,
`json.Marshal(err)` gives the structured diagnostic — `status`
(`"failure"`), `code`, `message`, `hint`, `row`, `col` and more.

## 7. Reuse one instance in a hot loop

Installing the plugin compiles the grammar, and that compile dominates a
parse by orders of magnitude (about 10 ms against about 100 µs). The
package-level `Parse` you have used so far builds one engine on the
first call and keeps it, serialising callers through a mutex, so it is
safe to call from any goroutine. When one goroutine parses many strings,
skip the mutex: build an instance with `Make` and reuse it.

```go
j := tabnassemver.Make() // compiles the grammar once

for _, s := range tags {
	v, err := j.Parse(s)
	if err != nil {
		// not a version — see step 6
		continue
	}
	// use v: the same map[string]any as tabnassemver.Parse returns
}
```

An instance from `Make` is not safe for concurrent `Parse` calls; make
one per goroutine. `Make` is exactly a bare engine with the plugin
installed, which you can also spell out yourself:

```go
j := tabnas.Make()
if err := j.Use(tabnassemver.Semver); err != nil {
	// the embedded grammar failed to compile — a broken build, not bad input
}
```

Installing the plugin twice on one instance is a no-op, and there are no
options to pass — `tabnassemver.Defaults` is an empty map.

## Where to go next

- [How-to guide](guide.md) — focused recipes for individual tasks.
- [Reference](reference.md) — the public API, the value shape including
  `*big.Int`, the error fields, and the full syntax accepted.
- [Concepts](concepts.md) — how the specification's grammar becomes the
  parser, and how the Go version differs from TypeScript.
- The root [README](../../README.md) and [AGENTS.md](../../AGENTS.md) —
  the conformance claim and how the plugin is checked against it.
