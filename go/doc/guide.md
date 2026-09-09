# How-to guide (Go)

Short, task-focused recipes. Each is self-contained and assumes the
module is installed (`go get github.com/tabnas/semver/go@latest`; the
[tutorial](tutorial.md) walks through a first parse). For every export,
every field of the value and the complete syntax, follow the links into
the [reference](reference.md); for why the plugin is built the way it
is, see [concepts](concepts.md).

```go
import tabnassemver "github.com/tabnas/semver/go"
```

## Parse a single string

`tabnassemver.Parse` is the one-call entry point — pass a version
string, get a value and an error:

```go
v, err := tabnassemver.Parse("1.2.3-alpha.1+build.5")
// v: map[string]any{
//   "major": float64(1), "minor": float64(2), "patch": float64(3),
//   "prerelease": []any{"alpha", float64(1)},
//   "build":      []any{"build", "5"},
// }
```

The value is typed `any`; on success it is always a `map[string]any`
with exactly those five keys. `Parse` reuses one cached engine behind a
mutex, so a loop does not recompile the grammar, and it is safe to call
from several goroutines at once.

## Validate without using the value

Only the error matters — the grammar is the only acceptor, so a
successful parse *is* the validation:

```go
func isSemver(s string) bool {
    _, err := tabnassemver.Parse(s)
    return err == nil
}

isSemver("1.0.0-rc.1") // true
isSemver("v1.0.0")     // false — no prefix of any kind
isSemver("1.0")        // false — three components, always
isSemver("")           // false — the empty string is not a version
```

A `v` prefix, a blank or newline anywhere, a leading zero in
`MAJOR`/`MINOR`/`PATCH` or in a numeric pre-release identifier, an empty
identifier, a second `+`: all errors (the [reference](reference.md)
lists them in full).

## Read the parts out of the value

Assert down to the map, then to each part:

```go
v, err := tabnassemver.Parse("2.5.1-rc.3+20260909.git.abcdef0")
if err != nil {
    return err
}
m := v.(map[string]any)

major := m["major"].(float64)  // float64(2) — but see the *big.Int recipe
pre := m["prerelease"].([]any) // []any{"rc", float64(3)}
for _, id := range pre {
    switch id.(type) {
    case string:   // an alphanumeric identifier: "rc"
    case float64:  // a numeric identifier: 3
    case *big.Int: // a numeric identifier above 2^53 - 1
    }
}
build := m["build"].([]any)    // []any{"20260909", "git", "abcdef0"}
name := build[0].(string)      // build identifiers are always strings
```

| Part | Go type |
|---|---|
| `major`, `minor`, `patch` | `float64`, or `*big.Int` above `MaxSafeInteger` (2^53 − 1) |
| a pre-release identifier that is all digits | `float64` / `*big.Int`, same rule |
| any other pre-release identifier | `string` — `"alpha"`, `"1a"`, `"01a"`, `"-"` |
| a build identifier | `string`, always — `"001"` keeps its zeros |

`prerelease` and `build` are present on every value; without a `-` or
`+` part they are an empty `[]any{}`, never `nil`. A bare `.(float64)`
assertion panics on a `*big.Int`, so use the type switch on any
component whose size you do not control.

## Sort versions by precedence

`Compare` returns `-1`, `0` or `1` by the specification's precedence
rules (§11), so it slots straight into `sort.Slice` over values from
`Parse`:

```go
// vs: []any, each element from Parse
sort.Slice(vs, func(i, k int) bool {
    c, _ := tabnassemver.Compare(vs[i], vs[k]) // never errs on parsed values
    return c < 0
})
// "1.0.0", "1.0.0-rc.1", "1.0.0-beta.11", "1.0.0-beta.2", "1.0.0-alpha"
// sort to: 1.0.0-alpha, 1.0.0-beta.2, 1.0.0-beta.11, 1.0.0-rc.1, 1.0.0
```

The order is the specification's own: `major`, `minor`, `patch`
numerically; a pre-release below its normal version; then identifier by
identifier — numeric ones numerically (`beta.2` before `beta.11`),
alphanumeric ones in ASCII order, numeric below alphanumeric, and a
longer list of otherwise-equal identifiers above the shorter one.
`Compare` errs only when an argument is not a parsed value (a string, or
a map with a wrong-typed field), which values from `Parse` never are —
hence the discarded error. The highest version is the last element.

## Detect a pre-release

A version is a pre-release exactly when its `prerelease` list is
non-empty:

```go
func isPrerelease(v any) bool {
    return len(v.(map[string]any)["prerelease"].([]any)) > 0
}

a, _ := tabnassemver.Parse("1.0.0-alpha")
b, _ := tabnassemver.Parse("1.0.0")
isPrerelease(a) // true
isPrerelease(b) // false
```

A pre-release ranks below the release it precedes but above every
earlier release — `1.0.0-alpha` < `1.0.0` < `2.0.0-0` — so the highest
element of a sorted list can be a pre-release. Filter with
`isPrerelease` before sorting when you want the highest stable release.

## Ignore build metadata

`Compare` already ignores it, as the specification says (§10, §11.1):
`1.0.0+a` and `1.0.0+b` compare `0`, the same version. To drop the
metadata from the string, copy the value with an empty `build` and
format it — `Format` writes the `+` part only when `build` holds at
least one identifier:

```go
func stripBuild(v any) (string, error) {
    m := v.(map[string]any)
    out := make(map[string]any, len(m))
    for k, val := range m {
        out[k] = val
    }
    out["build"] = []any{}
    return tabnassemver.Format(out)
}

v, _ := tabnassemver.Parse("1.0.0-beta+exp.sha.5114f85")
stripBuild(v) // "1.0.0-beta", nil
```

## Round-trip with Format

`Format` renders a value back to its string. For a value that came out
of `Parse` the result is the input, character for character — nothing
in a valid version is normalised away, so a value can always be traced
back to the text it came from:

```go
for _, s := range []string{"1.0.0-x-y-z.--", "1.0.0-alpha+001", "1.0.0+21AF26D3----117B344092BD"} {
    v, _ := tabnassemver.Parse(s)
    out, _ := tabnassemver.Format(v)
    fmt.Println(out == s) // true, every time
}

// A hand-built value works too, if it has the parsed shape.
s, err := tabnassemver.Format(map[string]any{
    "major": float64(1), "minor": float64(2), "patch": float64(3),
    "prerelease": []any{"rc", float64(1)},
    "build":      []any{"sha", "abc"},
}) // "1.2.3-rc.1+sha.abc", nil
```

`Format` errs when the value is not a `map[string]any`, when a
component is not a non-negative integer (`1.5`), when `prerelease` or
`build` is not a `[]any`, or when a build identifier is not a string. It
checks shape, not grammar: `"a_b"` under `prerelease` renders as
`1.0.0-a_b`, which `Parse` rejects — to be sure a hand-built value is a
valid version, parse the formatted string back.

## Handle integers beyond 2^53 − 1

The specification puts no upper bound on `MAJOR`, `MINOR`, `PATCH` or a
numeric pre-release identifier. A component up to `MaxSafeInteger`
(2^53 − 1 = 9007199254740991, JavaScript's `Number.MAX_SAFE_INTEGER`, so
both runtimes switch at the same value) is a `float64`; one above it is
a `*big.Int` from `math/big`, exact to the digit, never rounded:

```go
v, _ := tabnassemver.Parse("9007199254740991.9007199254740992.0")
m := v.(map[string]any)
m["major"] // float64(9007199254740991) — the last safe integer
m["minor"] // *big.Int 9007199254740992 — one more, no longer safe
m["patch"] // float64(0)
```

To work with one type, lift the `float64` case with
`big.NewInt(int64(x))` — exact, since every `float64` component is an
integer at most 2^53 − 1. `Compare` handles the mixed case itself
(`9007199254740991.0.0` ranks below `9007199254740992.0.0`) and `Format`
renders a `*big.Int` as plain digits, so neither needs help. A build
identifier is never converted: `1.0.0+9007199254740993` keeps
`"9007199254740993"` as a string.

## Reuse a parser for many inputs

Installing the plugin compiles the ABNF grammar (on the order of 10 ms
in Go); a parse takes on the order of 100 µs. `Parse` hides the compile
by keeping one instance, but it also serialises every caller through a
mutex. For a hot loop on one goroutine, build your own instance with
`Make` and reuse it:

```go
j := tabnassemver.Make() // *tabnas.Tabnas with the plugin installed

for _, s := range inputs {
    v, err := j.Parse(s)
    if err != nil {
        continue // a rejected string; the instance is fine for the next one
    }
    use(v)
}
```

`j.Parse` has the same signature and results as `tabnassemver.Parse`,
and a failed parse does not affect the next one. The instance is **not**
safe for concurrent use: give each goroutine its own `Make()`, or stay
with the package-level `Parse`, which is. `Make` panics if the plugin
fails to install; with the embedded grammar that cannot happen, so a
panic there means a broken build, never a bad input.

## Handle a parse error and read the diagnostic

Every rejection is one error type and one code: a `*tabnas.TabnasError`
whose `Code` is `"unexpected"`, raised where the grammar has no
alternative for what comes next — usually at the offending character,
though a lookahead failure can be reported earlier (`1.02.3` fails at
column 1). The plugin declares no codes of its own (see
[AGENTS.md](../../AGENTS.md), "Error codes"), so `Code` never says *why*
a string was rejected — the position, the offending text and the hint
do:

```go
import tabnas "github.com/tabnas/parser/go"

_, err := tabnassemver.Parse("1.2.3-a_b")

var te *tabnas.TabnasError
if errors.As(err, &te) {
    te.Code // "unexpected"
    te.Row  // 1
    te.Col  // 8 — 1-based column of the offending character
    te.Src  // "_" — the offending text
    te.Hint // what a version has to look like, with a link to semver.org
}

raw, _ := json.Marshal(err) // the engine's structured diagnostic
// {"status":"failure","code":"unexpected",
//  "message":"unexpected character(s): _","hint":"...",
//  "row":1,"col":8,"pos":7,"len":1,"rule":"...", ...}
```

`err.Error()` is a multi-line message that reads
`[tabnas/unexpected]: unexpected character(s): _`, points at the column
in the source line, and repeats the hint; the JSON form is the one for a
log, an API response or a tool. There `status` is always `"failure"`,
`code` is the contract, `hint` is the same text as `te.Hint`; `row` and
`col` are 1-based, `pos` 0-based, `len` the length of the offending
text. The empty string fails at row 1, column 1 with an empty `Src`;
`v1.2.3` at column 1 with `Src` `"v"`. Only the code is guaranteed to
match the TypeScript plugin's: `code` is the one cross-runtime field of
the diagnostic, and at a lookahead failure the reported column is not
guaranteed to agree between the two engines, so compare positions only
within one runtime.

## Install the plugin on your own engine

`Make` is only `tabnas.Make()` plus one install call. Do it yourself
when you already have an engine in hand:

```go
import tabnas "github.com/tabnas/parser/go"

j := tabnas.Make()
if err := j.Use(tabnassemver.Semver); err != nil {
    return err
}
v, err := j.Parse("1.2.3")
```

`j.UseDefaults(tabnassemver.Semver, tabnassemver.Defaults)` is the same
thing spelled the way every tabnas plugin is installed, and is what
`Make` itself calls; `Defaults` is an empty map because the plugin has
no options — the grammar is the specification, and there is nothing to
configure. Installing the plugin a second time on the same instance is
a no-op, not a second grammar.

The plugin installs the specification's grammar and, on the same spec,
switches every default lexer off (whitespace, line ends, comments,
strings, numbers, bare words, keyword values), unbinds the engine's
JSON punctuation tokens (`{ } [ ] : ,`) and turns `lex.empty` off, so
the instance parses versions and nothing else — not even the empty
string, which is an error rather than `nil`. Do not turn any of those
back on — it would then accept strings the specification rejects.

## Get the grammar text

The ABNF the plugin compiles is exported as `Grammar`, a string constant
holding the text of
[`semver-grammar.abnf`](../../semver-grammar.abnf) at the repository
root, embedded at build time (the literal opens with one newline, so the
first line printed is blank):

```go
fmt.Print(tabnassemver.Grammar)
// ; Semantic Versioning 2.0.0 — the grammar of a valid version string.
// ...
// semver = valid-semver
// valid-semver = version-core [ "-" pre-release ] [ "+" build ]
// ...
```

Use it to feed grammar tooling or to show a user exactly what is
accepted. `VERSION` (`"0.1.0"`) is the module's own version — itself a
valid version string, so `tabnassemver.Parse(tabnassemver.VERSION)`
succeeds.

The TypeScript recipes for the same tasks are in
[`../../ts/doc/guide.md`](../../ts/doc/guide.md).
