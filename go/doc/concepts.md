# Concepts (Go)

Background on how the Go semver plugin is put together, and why — and,
at the end, how it differs from the canonical TypeScript version. This
is understanding-oriented reading; for steps see the
[tutorial](tutorial.md) and [how-to guide](guide.md), and for exact
signatures, the value shape and the complete accepted syntax see the
[reference](reference.md).

## A grammar plugin on a shared engine

The module has no parser of its own. It sits at the top of a stack of
four pieces:

- the **tabnas engine** (`github.com/tabnas/parser/go`) — a
  configurable lexer under a rule-and-alternative parser, driven by
  whatever grammar it is handed;
- the **notation-neutral compiler** (`github.com/tabnas/bnf/go`) — turns
  a grammar into the engine's rule set without knowing which notation it
  was written in;
- the **ABNF front end** (`github.com/tabnas/abnf/go`) — reads RFC 5234
  ABNF and drives that compiler; the plugin calls its `Abnf` and
  `AttachActions`;
- **this module** (`github.com/tabnas/semver/go`, package
  `tabnassemver`) — the grammar text, one semantic action, a set of
  engine options, and the `Compare` and `Format` helpers.

Install is where the work happens. `Semver(j, opts)` compiles the ABNF
into a rule set (about 10 ms), attaches the action, sets the options on
the compiled spec and hands the whole thing to the engine in one
`j.Grammar(spec)` call, so grammar and options arrive together. A parse
afterwards costs about 100 µs, which is why every document here says to
build one instance and reuse it: `Make()` returns a bare engine with the
plugin installed, and the package-level `Parse` keeps one such instance
for the life of the process. Installing the plugin a second time on the
same instance is a no-op — a `semver-init` decoration on the engine
records that the work is done — and `perf_test.go` pins the
reuse-versus-rebuild ratio.

## The grammar is the parser

The specification publishes its grammar in BNF. `semver-grammar.abnf`
at the repository root is that grammar transcribed into RFC 5234 ABNF,
one production per production, with the specification's names kept and
its spaces written as hyphens, so `<version core>` is `version-core`:

```abnf
semver       = valid-semver
valid-semver = version-core [ "-" pre-release ] [ "+" build ]
version-core = major "." minor "." patch
pre-release  = pre-release-identifier *( "." pre-release-identifier )
build        = build-identifier *( "." build-identifier )
```

Nothing in the plugin's code decides what a valid version is: the
grammar accepts or rejects, and code runs only after it has accepted.
The file is single-sourced — the TypeScript build's `embed-grammar.js`
copies it verbatim into `semver.go` (and into `ts/src/semver.ts`)
between `BEGIN/END EMBEDDED` markers — and the same text is exported as
the `Grammar` constant for tooling.

### Two rewrites, one language

The engine picks an alternative from a bounded lookahead of a few
tokens — a character each — never by reading to the end of an
identifier. Two of the specification's productions cannot be dispatched
that way, so the file rewrites their *shape*; its comments show that
the *language* is unchanged.

**`pre-release-identifier`.** The specification says
`<alphanumeric identifier> | <numeric identifier>`. Both can begin with
a digit — `1` is numeric, `1a` alphanumeric, `01a` alphanumeric, `01`
nothing at all — and telling them apart may need every character of the
identifier. The grammar factors on the first character instead:

```abnf
pre-release-identifier = "0" [ *digit alphanumeric-tail ]
                       / positive-digit *digit [ alphanumeric-tail ]
                       / alphanumeric-tail
alphanumeric-tail      = non-digit *identifier-character
```

A lone `0` is the numeric identifier zero; `0` followed by more digits
is valid only if a non-digit eventually arrives (`007a`); a positive
digit starts a numeric identifier that becomes alphanumeric if a
non-digit follows; anything else must be a non-digit. The union of the
three is exactly alphanumeric ∪ numeric, and what it excludes is exactly
a digit string with a leading zero — which the specification excludes
too.

**`build-identifier`.** The specification says
`<alphanumeric identifier> | <digits>`: a non-empty identifier string
with a non-digit in it, or one without. Their union is every non-empty
identifier string, so the grammar writes `1*identifier-character`, and
`001` is a valid build identifier, zeros and all.

Neither rewrite is an approximation, and the conformance corpus (below)
checks that against an independent judge on every run.

## What the compiler makes of it

`abnf.Abnf(Grammar, &abnf.AbnfConvertOptions{Start: "semver", Tag:
"semver"})` returns a `*tabnas.GrammarSpec` holding the engine's rule
set and the options it needs — for this grammar, 150 rules over seven
tokens:

| Grammar element | Compiled form |
|---|---|
| `"0"`, `"."`, `"-"`, `"+"` | fixed tokens `#0`, `#T`, `#T1`, `#T2` |
| `%x31-39`, `%x41-5A`, `%x61-7A` | one regular-expression class token each, marked eager |
| `*digit`, `[ … ]`, `1*identifier-character`, `( … )` | helper rules (`_gen5_star_digit`, `_gen13_opt__gen12_group`, …) |
| an alternative with a reference in its middle | a head rule plus `$stepN` continuation rules (`valid-semver$alt0$step1`, …) |
| the start rule | wrapped in `__start__`, the compiler's end-of-source rule |

One character is one token, because the grammar names only single
characters. Helper and chain rules flatten: their text rolls up into the
enclosing named rule's `src` and they add no node. Every named
production survives by name, but not every one of them takes part in a
parse.

### Leading references are inlined

`github.com/tabnas/bnf/go` runs Paull's substitution over every
production: an alternative that *begins* with a reference to another
rule has that rule's alternatives inlined, recursively, which is what
fills in the lookahead columns. Here that dissolves `version-core`,
`major` and the `numeric-identifier` beneath them into `valid-semver` —
the compiled `valid-semver` has two opening alternatives, one for a `0`
and one for a positive digit, and pushes `minor` — and likewise the
first `pre-release-identifier` into `pre-release` and the first
`build-identifier` into `build`. The parse never pushes those rules, so
they never get a node and never fire a lifecycle hook, while `minor`,
`patch` and every identifier after the first do. So the parse tree is
not the grammar tree, and which rules the compiler keeps is a property
of the compiler, not of the specification; a value built by walking the
tree would be coupled to that detail.

## Why the value is built from the accepted text

The plugin has one semantic action, registered as `@semver:ac` — the
after-close phase of the start rule. When `semver` closes, its node's
`src` is the text every terminal under it matched, and because every
default lexer is off (below) that is the whole input, byte for byte.
The grammar has just proven the text well-formed, so the action splits
it at the separators without checking anything: the first `+` opens the
build metadata (no identifier contains `+`); before it, the first `-`
opens the pre-release (the version core contains no `-`); `.` separates
identifiers, which never contain it. The action replaces the compiler's
`{rule, src, kids}` node with the value map, and the `__start__` wrapper
bubbles it up as the parse result:

```go
err = abnf.AttachActions(spec, abnf.ActionsMap{
	"@semver:ac": {func(r *tabnas.Rule, _ *tabnas.Context) {
		if node, ok := r.Node.(map[string]any); ok {
			if src, ok := node["src"].(string); ok {
				r.Node = fromText(src)
			}
		}
	}},
})
```

Because the value is a function of the accepted text alone, `Format`
gives the input back exactly, and the plugin is immune to which rules
the compiler inlines:

```go
v, _ := tabnassemver.Parse("1.0.0-x.7.z.92+exp.sha.5114f85")
// map[string]any{
//   "major": float64(1), "minor": float64(0), "patch": float64(0),
//   "prerelease": []any{"x", float64(7), "z", float64(92)},
//   "build":      []any{"exp", "sha", "5114f85"},
// }
s, _ := tabnassemver.Format(v) // "1.0.0-x.7.z.92+exp.sha.5114f85"
```

**Why the hook hangs on `semver`, not `valid-semver`.** The `semver`
production is a pure alias of the specification's root, and exists for
one reason. `AttachActions` turns `@semver:ac` into the engine's
`@semver-ac` function reference, and the published TypeScript engine
(0.9.0) derived the phase of such a reference by splitting at the
*first* hyphen: `@valid-semver-ac` was read as the phase `semver-ac`
and threw at install. The Go engine never had that bug and would bind
the hook on `valid-semver` directly; the alias is kept here so both
runtimes compile the same grammar, and costs nothing.

## The value decisions

The value is a `map[string]any` with the five parts the specification
names, always all five, with an empty `[]any{}` where a part is absent.
Three representation choices deserve an explanation, each about a value
the specification leaves open.

**Integers switch to `*big.Int` above 2^53 − 1.** `major`, `minor`,
`patch` and a numeric pre-release identifier are a `float64` while they
fit `MaxSafeInteger` (`1<<53 - 1`, JavaScript's
`Number.MAX_SAFE_INTEGER`) and a `*big.Int` from `math/big` beyond it.
The specification places no upper bound on an integer, and a parser
that silently rounded `9007199254740993.0.0` would report the wrong
version. The threshold is JavaScript's so that the two runtimes switch
representation at the same value; `Format` renders either as plain
digits.

**Pre-release identifiers keep their kind.** An identifier that is all
digits is a number; the grammar has already excluded a leading zero
there. Any other identifier is a `string`, leading zeros included
(`01a`, `007a`), as are the specification's odder examples such as
`--`. The specification compares the two kinds differently (§11.4), so
the value has to carry the distinction; a consumer can type-switch
instead of re-parsing.

**Build identifiers are always strings.** `001` keeps its zeros, and
build metadata takes no part in precedence, so a number would lose
information for nothing.

```go
v, _ := tabnassemver.Parse("9007199254740991.0.0")
v.(map[string]any)["major"] // float64(9007199254740991)

v, _ = tabnassemver.Parse("9007199254740992.0.0")
v.(map[string]any)["major"] // *big.Int 9007199254740992
s, _ := tabnassemver.Format(v)      // "9007199254740992.0.0"

v, _ = tabnassemver.Parse("1.0.0-0.3.7")
v.(map[string]any)["prerelease"] // []any{float64(0), float64(3), float64(7)}
v, _ = tabnassemver.Parse("1.0.0-1a")
v.(map[string]any)["prerelease"] // []any{"1a"}
v, _ = tabnassemver.Parse("1.0.0-alpha+001")
v.(map[string]any)["build"]      // []any{"001"}
```

## Precedence

`Compare(a, b)` is the specification's §11 over two parsed values,
returning `-1`, `0` or `1`. It does not re-parse, and `0` means the two
have the same precedence, not that they were the same string:

1. `major`, `minor`, `patch` numerically — two `float64` values compare
   directly, and once either side is a `*big.Int` both are compared as
   big integers, so the representation switch is invisible here.
2. A pre-release version ranks below its normal version (§11.3).
3. Otherwise pre-release identifiers are compared left to right: numeric
   ones numerically, alphanumeric ones in ASCII order (a plain Go string
   comparison, since identifiers are ASCII), and a numeric identifier
   always below an alphanumeric one (§11.4.1–3).
4. When every preceding identifier is equal, the larger set of
   identifiers ranks higher (§11.4.4).
5. Build metadata is ignored (§10, §11.1).

```go
lt := func(a, b string) int {
	x, _ := tabnassemver.Parse(a)
	y, _ := tabnassemver.Parse(b)
	c, _ := tabnassemver.Compare(x, y)
	return c
}
lt("1.0.0-alpha", "1.0.0-alpha.1")      // -1
lt("1.0.0-alpha.1", "1.0.0-alpha.beta") // -1
lt("1.0.0-beta.2", "1.0.0-beta.11")     // -1
lt("1.0.0-rc.1", "1.0.0")               // -1
lt("1.0.0-Z", "1.0.0-a")                // -1
lt("1.0.0+a", "1.0.0+b")                // 0
lt("2.0.0", "10.0.0")                   // -1
```

The specification's own chain — `1.0.0-alpha < 1.0.0-alpha.1 <
1.0.0-alpha.beta < 1.0.0-beta < 1.0.0-beta.2 < 1.0.0-beta.11 <
1.0.0-rc.1 < 1.0.0`, and `1.0.0 < 2.0.0 < 2.1.0 < 2.1.1` — sits inside
the shared fixture `test/precedence/order.tsv`, which both runtimes
check pairwise in both directions, so transitivity is pinned too;
`equal.tsv` holds the pairs that differ only in build metadata.

## Why every default lexer is off

The engine's defaults are JSON's: it skips whitespace, line ends and
comments, lexes quoted strings, numbers, bare words and keyword values
such as `true` and `null`, and binds `{ } [ ] : ,` as punctuation. Each
of those would let the plugin accept something the specification
rejects — `" 1.2.3"`, `"1.2.3\n"`, `"\"1.2.3\""`, `"1.2.3#comment"` —
or mis-lex something it accepts, such as the pre-release identifier
`true`.

So the plugin sets, on the compiled spec's `Options`, `Lex: &off` for
the space, line, comment, string, number, text and value lexers;
unbinds the six punctuation tokens (`#OB`, `#CB`, `#OS`, `#CS`, `#CL`,
`#CA`) by setting each to `nil` in `Fixed.Token`; and sets
`Lex.Empty: &off`, so an empty source is a parse error rather than the
engine's default answer of `nil`. What remains is exactly the grammar's
seven tokens. A character the grammar does not name — a blank, a tab, a
newline, a quote, a `v` prefix — has no matcher at all and is rejected
as `unexpected` at its position, never skipped, never swallowed. The
punctuation is unbound rather than merely unused so that JSON's tokens
do not show up in diagnostics and introspection as tokens of this
grammar. Turn any of these defaults back on and the plugin accepts
strings the specification rejects; the options ride on the spec
precisely so that they can only arrive together with the grammar.

```go
_, err := tabnassemver.Parse(" 1.2.3") // *tabnas.TabnasError, Code "unexpected", Col 1
_, err = tabnassemver.Parse("")        // *tabnas.TabnasError, Code "unexpected"
v, _ := tabnassemver.Parse("1.0.0-true.null")
v.(map[string]any)["prerelease"]       // []any{"true", "null"}
```

## Why there are no error codes

Every rejection is the engine's base `unexpected` code, raised where the
grammar has no alternative for the next character. The plugin declares
no code of its own — `tabnas.plugin.json` at the repository root lists
an empty `errorCodes` — and adds only a `Hint` for `unexpected` that
says what a version has to look like and links to the specification.

That is a decision, not a gap. The grammar is the sole acceptor, and the
compiler offers no safe place for an error production: a trap
alternative at a leading position is inlined by Paull's substitution
(above), and a nullable trap inlined there would change the accepted
language rather than merely label a rejection. A code that can only be
raised from some positions is worse than none. The fixtures pin the
contract instead: all 141 rows of `test/spec/strict.tsv` expect
`ERROR:unexpected`, compared exactly in both runtimes.

The position is where the grammar ran out of alternatives, which is not
always where a human would point. `v1.2.3` fails at column 1 on the
`v`; `1.2.3-01` fails at column 9, the end of the input, because `01`
could still have become the alphanumeric identifier `01a` and only the
end of the string settled it. At a lookahead failure the two runtimes
may even differ: `01.2.3` is reported at the `0` here and at the `1` in
TypeScript. The code is the contract; the position is not.

## Conformance

The claim is that the plugin accepts exactly the strings the semver.org
grammar accepts, and produces the parts the specification names for
each. The judge is not this repository: semver.org publishes, in its
FAQ, a regular expression that recognises the language of its grammar,
and `oracle_test.go` (with `ts/test/oracle.test.ts` as its twin) grades
every string of a generated corpus against it. The plugin's verdict must
equal the expression's; on every accepted string the value must match
the expression's captures and `Format` must return the input.

| Section | Strings | Accepted | Rejected |
|---|---|---|---|
| `exhaustive` — the empty string and every string of length 1–5 over `019aZ-.+` | 37,449 | 27 | 37,422 |
| `structured` — 5 version-core shapes × pre-release tails of length 0–3 over `01a.` × build tails of length 0–3 over `0a.` | 17,000 | 1,634 | 15,366 |
| `mutation` — valid versions with 1–3 random edits | 3,000 | 838 | 2,162 |
| `random` — random strings of length 1–12 over a wider alphabet (blanks, tab, `v`, `_`, `/`, `:`) | 1,000 | 0 | 1,000 |

The corpus is generated, not committed: both runtimes derive the same
58,449 strings from the same alphabets, enumeration order and
xorshift32 stream, and a pinned FNV-1a hash over the whole corpus
(`0x97bd27cb`) proves they graded the same strings. The per-section
census is pinned as well, so a section that starts accepting more or
fewer strings goes red instead of inflating a pass rate; changing the
generator means re-pinning both constants in both runtimes in one
commit. The suites never skip.

Everything the corpus pins that is worth reading is also committed as a
shared fixture: `test/spec/*.tsv` holds the specification's own
examples, the version core, pre-release and build identifiers, and the
141 rejections of `strict.tsv`; `test/precedence/*.tsv` holds the
`Compare` chain and the equal pairs. Both runtimes auto-discover and run
every file (`parity_test.go`, `precedence_test.go`). A new parse case
belongs there; the in-language suites keep only what a `.tsv` cannot
express — `*big.Int` values, function results, error details.

## Differences from the TS version

The TypeScript implementation in `ts/src/semver.ts` is canonical; this
module is a port built from the same `semver-grammar.abnf`, compiling
it with the same options and running the same fixtures and the same
corpus with the same pinned census and hash. When the two disagree on
parse behaviour, Go changes to match — unless Go has exposed a
TypeScript defect, in which case TypeScript is fixed first, as happened
with both toolchain fixes below. The differences do not change *which*
strings parse or *what* parts they produce; they are about the host
language.

### API shape

| Area | TypeScript | Go |
|---|---|---|
| Convenience entry | none, by design — install the plugin yourself | `tabnassemver.Parse(src)` over one cached instance |
| Build a parser | `new Tabnas().use(Semver)` | `tabnassemver.Make()`, or `j.Use(tabnassemver.Semver)` / `j.UseDefaults(tabnassemver.Semver, tabnassemver.Defaults)` |
| Options | `Semver.defaults` is `{}` | `Defaults` is an empty `map[string]any` |
| Parse failure | `tn.parse` **throws** | `Parse` returns `(nil, error)`; never panics on bad input |
| Precedence | `compare(a, b)` returns `-1 \| 0 \| 1` | `Compare(a, b)` returns `(int, error)` |
| Rendering | `format(v)` returns `string` | `Format(v)` returns `(string, error)` |
| Grammar text | `grammar` | `Grammar` |
| Package version | `VERSION` | `VERSION` |

`Compare` and `Format` take `any`, because that is what `Parse` returns,
and so they can be handed something that is not a parsed version; they
return an error (`semver: not a parsed version (want map[string]any)`)
where the TypeScript functions rely on the `Version` type at compile
time. `Make` panics only if the embedded grammar fails to install,
which cannot happen on a correct build; bad input never panics.

### Value types

TypeScript returns a typed `Version` object with its keys in
specification order; Go returns `any` holding a `map[string]any` with
predictable concrete types and, being a map, no key order:

| Value | TypeScript | Go |
|---|---|---|
| The version | `Version` object | `map[string]any` with keys `major`, `minor`, `patch`, `prerelease`, `build` |
| `major`, `minor`, `patch` up to 2^53 − 1 | `number` | `float64` |
| … beyond 2^53 − 1 | `bigint` | `*big.Int` |
| Numeric pre-release identifier | `number` or `bigint` | `float64` or `*big.Int` |
| Alphanumeric pre-release identifier | `string` | `string` |
| `prerelease` | `(string \| number \| bigint)[]` | `[]any` |
| `build` | `string[]` | `[]any` of `string` |

The most visible consequence is that `1.2.3` comes back as
`float64(1)`, `float64(2)`, `float64(3)`: Go has no separate integer
type in the result, and the switch to `*big.Int` happens at exactly the
value where TypeScript switches to `bigint`, so a reader of either
runtime's value can rely on the same boundary.

### Concurrency

A Go engine instance is not safe for concurrent use, so the
package-level `Parse` builds its instance once (`sync.Once`) and
serialises callers through a mutex — the cost is far below rebuilding
the grammar per call. An instance from `Make` has no such guard: reuse
it on one goroutine, or make one per goroutine. The TypeScript side has
no cached instance to guard: it has no `Parse` convenience, by design.

### Errors

Where TypeScript throws a `TabnasError` (a `SyntaxError`) with `code`,
`lineNumber` and `columnNumber`, Go returns a `*tabnas.TabnasError` as
the second value, with `Code`, `Row`, `Col`, `Pos`, `Src` (the offending
text) and `Hint`. In both, `Code` is `"unexpected"` for every rejection,
the empty string included, and the hint text is the same. Marshalling
the Go error with `encoding/json` gives the same structured diagnostic
as `JSON.stringify` on the TypeScript side — `status: "failure"`,
`code`, `message`, `hint` and the position fields — so a log line or an
API response looks alike whichever runtime produced it. Only the code
is guaranteed to match at a lookahead failure; see above.

### Hooks

The Go engine has always accepted a `@<rule>-<phase>` function
reference on a hyphenated rule name, so this port could have attached
its action to `valid-semver` directly. It uses the `semver` alias
anyway, for parity: the two runtimes compile one grammar, and the alias
is what keeps the TypeScript plugin working on the published engine.

### The two toolchain fixes

The oracle corpus found two defects in the TypeScript toolchain, and
both were already the Go behaviour. The Go emitter in
`github.com/tabnas/bnf/go` has always marked character-class tokens
eager, so a class can be lexed at any lookahead slot; and the Go engine
has always tried the match tokens a rule expects at a slot before the
eager ones it does not. Without the first, the TypeScript plugin
rejected strings such as `1.0.0-01a` and `1.0.0-12a` — the `*digit`
helper peeks two digits, and the letter that ends the run lexed as a
fatal bad token at the second slot. Both are fixed upstream
([tabnas/bnf#33](https://github.com/tabnas/bnf/pull/33),
[tabnas/parser#161](https://github.com/tabnas/parser/pull/161)), and
the TypeScript plugin carries the first itself until a compiler that
sets the flag is published. This module never needed either: there is
no port of the fix in `semver.go`, and nothing to remove when the
TypeScript side catches up.

The TypeScript side of all of this is in
[`../../ts/doc/concepts.md`](../../ts/doc/concepts.md); the conformance
claim and the alignment rules the two runtimes follow are in the root
[AGENTS.md](../../AGENTS.md).
