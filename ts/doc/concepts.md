# Concepts

Background on how the semver plugin is put together, and why. This is
understanding-oriented reading — for steps see the
[tutorial](tutorial.md) and [how-to guide](guide.md), and for exact
signatures, the value shape and the complete accepted syntax see the
[reference](reference.md).

## A grammar plugin on a shared engine

The plugin has no parser of its own. It sits at the top of a stack of
four pieces:

- the **Tabnas engine** (`@tabnas/parser`) — a configurable lexer under
  a rule-and-alternative parser, driven by the grammar it is handed;
- the **notation-neutral compiler** (`@tabnas/bnf`) — turns a grammar
  into the engine's rule set without knowing which notation it was
  written in;
- the **ABNF front end** (`@tabnas/abnf`) — reads RFC 5234 ABNF and
  drives that compiler; the plugin calls its `abnfConvert` and
  `attachActions`;
- **this plugin** (`@tabnas/semver`) — the grammar text, one semantic
  action, and a set of engine options.

Install is where the work happens. `new Tabnas().use(Semver)` compiles
the ABNF into a rule set (about 75 ms), attaches the action, sets the
options on the compiled spec and hands the whole thing to the engine in
one `tn.grammar(spec)` call, so grammar and options arrive together. A
parse afterwards costs about 100 µs — which is why every document here
says to build one instance and reuse it. The instance keeps no state
between parses, and `perf.test.ts` pins the reuse-versus-rebuild ratio.

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
The file is single-sourced — `embed-grammar.js` copies it verbatim into
`src/semver.ts` and into the Go port at build time — and the same text
is exported as `grammar` for tooling.

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

`abnfConvert(grammar, { start: 'semver', tag: 'semver' })` returns a
spec holding the engine's rule set and the options it needs — for this
grammar, 150 rules over seven tokens:

| Grammar element | Compiled form |
|---|---|
| `"0"`, `"."`, `"-"`, `"+"` | fixed tokens `#0`, `#T`, `#T1`, `#T2` |
| `%x31-39`, `%x41-5A`, `%x61-7A` | one regex class token each |
| `*digit`, `[ … ]`, `1*identifier-character`, `( … )` | helper rules (`_gen5_star_digit`, `_gen13_opt__gen12_group`, …) |
| an alternative in which a reference is followed by more — another reference or a terminal | a head rule (`<rule>$altN`, one per alternative, when the production has several) plus `$stepN` continuation rules (`valid-semver$alt0$step1`, …) |
| the start rule | wrapped in `__start__`, the compiler's end-of-source rule |

One character is one token, because the grammar names only single
characters. Helper and chain rules flatten: in the `{rule, src, kids}`
tree the compiler's own actions would build, their text rolls up into
the enclosing named rule's `src` and they add no node — though the
plugin installs those rules without those actions, and builds no tree
at all (below). Every named production survives by name — the
`debug-model` test asserts as much — but not every one of them takes
part in a parse.

### Leading references are inlined

`@tabnas/bnf` runs Paull's substitution over every production: an
alternative that *begins* with a reference to another rule has that
rule's alternatives inlined, recursively, which is what fills in the
lookahead columns. Here that dissolves `version-core`, `major` and the
`numeric-identifier` beneath them into `valid-semver` — the compiled
`valid-semver` dispatches on `#0` or a positive digit at its first slot,
and the alternative it picks consumes the major and its `.` itself, then
pushes `minor` — and likewise the first `pre-release-identifier` into
`pre-release` and the first `build-identifier` into `build`. The parse
never pushes those rules, so they never get a node and never fire a
lifecycle hook, while `minor`, `patch` and every identifier after the
first do. The one shape the substitution leaves alone is a pure alias —
a production that is nothing but a single reference, outside any cycle,
such as `semver = valid-semver` or `minor = numeric-identifier` — which
is why `semver` survives to push `valid-semver`. So the parse tree is
not the grammar tree, and which rules the compiler keeps is a property
of the compiler, not of the specification; a value built by walking the
tree would be coupled to that detail.

## Why the value is built from the accepted text

The plugin asks for no parse tree. `toRecognitionSpec` — `@tabnas/bnf`'s,
re-exported by `@tabnas/abnf` — takes the converted spec and gives back
the same 150 rules over the same seven tokens with every AST-building
action dropped: 1,567 of the 1,608 alternatives carry one on the way out
of the compiler, and none do on the way into the engine. The rules, the
tokens and the accepted language are untouched; what goes is the
`{rule, src, kids}` node each rule would otherwise leave behind.

That is not a matter of taste. Every `*` and `1*` repetition compiles to
a chain of per-character helper rules, and each level of the chain
re-appends its child's `src` and re-copies its `kids`, so building the
tree costs time and memory quadratic in an identifier's length:
`1.0.0-` and 16,000 letters took about 8 s and 2.7 GB, and 32,000
letters filled the V8 heap and killed the process outright, which no
`try` can catch. Those are valid versions — the specification bounds
neither the length of an identifier nor how many a version has — so the
tree was a denial of service on strings the grammar accepts. With it
gone the same 32,000 characters parse in about 0.2 s and some 60 MB, and
cost grows with the length of the input rather than with its square:
four times the identifier costs about four times the time, not
sixteen. `perf.test.ts` pins both.

What remains is one semantic action, on the compiler's end-of-source
wrapper: the rule named by `spec.options.rule.start`, which for this
grammar is `__start__`. It opens by pushing `semver` and closes on the
end token `#ZZ` and nothing else, so when it closes the whole source has
been accepted and `ctx.src()` — the text being parsed — IS the accepted
version, character for character; every default lexer is off (below), so
nothing was skipped on the way in. The grammar has just proven the text
well-formed, so the action splits it at the separators without checking
anything: the first `+` opens the build metadata (no identifier contains
`+`); before it, the first `-` opens the pre-release (the version core
contains no `-`); `.` separates identifiers, which never contain it. The
`Version` it builds becomes that rule's node, which is what `parse`
returns.

```js
import { Tabnas } from '@tabnas/parser'
import { Semver, format } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

tn.parse('1.0.0-x.7.z.92+exp.sha.5114f85') // => { major: 1, minor: 0, patch: 0, prerelease: ['x', 7, 'z', 92], build: ['exp', 'sha', '5114f85'] }
format(tn.parse('1.0.0-x-y-z.--')) // => '1.0.0-x-y-z.--'
```

Because the value is a function of the accepted text alone, `format`
gives the input back exactly, and the plugin is immune to which rules
the compiler inlines.

**Why the hook hangs on the wrapper, not on `semver`.** `semver` closes
as soon as a version has been read, which is not the moment the input
ends. For `1.2.3f` it closes on `1.2.3`, before the engine reaches the
`f` it is going to reject, and `ctx.src()` there is the whole input, `f`
and all: a value built at that point would describe a string the parse
is about to throw out — and this one would not even get that far, since
`BigInt('3f')` raises a bare `SyntaxError` through the engine. The
wrapper cannot close early, having no alternative but the end of the
source, and it is also the only place left that can carry the value: with
no tree, nothing bubbles a child rule's node up to the result. The
`semver` alias is still the entry production — the name the grammar, the
fixtures and the diagnostics all use, kept as a rule of its own by the
pure-alias exemption above — but it is a name now, not a mechanism.

## The value decisions

`Version` is a plain object with the five parts the specification
names, keys in that order, and an empty array where a part is absent.
Three representation choices deserve an explanation, each about a value
the specification leaves open.

**Integers switch to `bigint` above 2^53 − 1.** `major`, `minor`,
`patch` and a numeric pre-release identifier are a `number` while they
fit `Number.MAX_SAFE_INTEGER` and a `bigint` beyond it. The
specification places no upper bound on an integer, and a parser that
silently rounded `9007199254740993.0.0` would report the wrong version.
`format` renders either as plain digits.

**Pre-release identifiers keep their kind.** An identifier that is all
digits is a number; the grammar has already excluded a leading zero
there. Any other identifier is a string, leading zeros included (`01a`,
`007a`), as are the specification's odder examples such as `--`. The
specification compares the two kinds differently (§11.4), so the value
has to carry the distinction; a consumer can test `typeof` instead of
re-parsing.

**Build identifiers are always strings.** `001` keeps its zeros, and
build metadata takes no part in precedence, so a number would lose
information for nothing.

```js
import { Tabnas } from '@tabnas/parser'
import { Semver, format } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

tn.parse('9007199254740991.0.0').major // => 9007199254740991
tn.parse('9007199254740992.0.0').major // => 9007199254740992n
format(tn.parse('9007199254740992.0.0')) // => '9007199254740992.0.0'

tn.parse('1.0.0-alpha.1').prerelease // => ['alpha', 1]
tn.parse('1.0.0-0.3.7').prerelease // => [0, 3, 7]
tn.parse('1.0.0-1a').prerelease // => ['1a']
tn.parse('1.0.0-x-y-z.--').prerelease // => ['x-y-z', '--']

tn.parse('1.0.0-alpha+001').build // => ['001']
tn.parse('1.0.0+21AF26D3----117B344092BD').build // => ['21AF26D3----117B344092BD']
```

## Precedence

`compare(a, b)` is the specification's §11 over two parsed values,
returning `-1`, `0` or `1`. It does not re-parse, and `0` means the two
have the same precedence, not that they were the same string:

1. `major`, `minor`, `patch` numerically — `<` and `>` compare a
   `number` with a `bigint` correctly, so the representation switch is
   invisible here.
2. A pre-release version ranks below its normal version (§11.3).
3. Otherwise pre-release identifiers are compared left to right: numeric
   ones numerically, alphanumeric ones in ASCII order, and a numeric
   identifier always below an alphanumeric one (§11.4.1–3).
4. When every preceding identifier is equal, the larger set of
   identifiers ranks higher (§11.4.4).
5. Build metadata is ignored (§10, §11.1).

```js
import { Tabnas } from '@tabnas/parser'
import { Semver, compare } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

compare(tn.parse('1.0.0-alpha'), tn.parse('1.0.0-alpha.1')) // => -1
compare(tn.parse('1.0.0-alpha.1'), tn.parse('1.0.0-alpha.beta')) // => -1
compare(tn.parse('1.0.0-beta.2'), tn.parse('1.0.0-beta.11')) // => -1
compare(tn.parse('1.0.0-rc.1'), tn.parse('1.0.0')) // => -1
compare(tn.parse('1.0.0-Z'), tn.parse('1.0.0-a')) // => -1
compare(tn.parse('1.0.0+a'), tn.parse('1.0.0+b')) // => 0
compare(tn.parse('2.0.0'), tn.parse('10.0.0')) // => -1
```

The specification's own chain — `1.0.0-alpha < 1.0.0-alpha.1 <
1.0.0-alpha.beta < 1.0.0-beta < 1.0.0-beta.2 < 1.0.0-beta.11 <
1.0.0-rc.1 < 1.0.0`, and `1.0.0 < 2.0.0 < 2.1.0 < 2.1.1` — sits inside
the shared fixture `test/precedence/order.tsv`, which both runtimes check
pairwise in both directions, so transitivity is pinned too;
`equal.tsv` holds pairs that differ at most in build metadata.

## Why every default lexer is off

The engine's defaults are JSON's: it skips whitespace, line ends and
comments, lexes quoted strings, numbers, bare words and keyword values
such as `true` and `null`, and binds `{ } [ ] : ,` as punctuation. Three
of those lexers would let the plugin accept something the specification
rejects: with the space lexer on, `' 1.2.3'` and `'1.2.3 '` parse; with
the line lexer, `'1.2.3\n'`; with the comment lexer, `'1.2.3#comment'`
and `'1.2.3//comment'`. The other four — string, number, text and
value — change no verdict on this grammar, because the engine tries a
grammar's own class and fixed tokens before any default lexer, so a
digit or a letter is the grammar's token first and `1.0.0-true.null`
parses either way; they are off all the same, so that what is accepted
never depends on the order in which the lexer tries its matchers.

So the plugin sets, on the compiled spec's options, `lex: false` for the
space, line, comment, string, number, text and value lexers; unbinds the
six punctuation tokens (`#OB`, `#CB`, `#OS`, `#CS`, `#CL`, `#CA`); and
sets `lex.empty: false`, so an empty source is a parse error rather than
the engine's default answer of `undefined`. What the input can still
produce is exactly the grammar's seven tokens. A character the grammar
does not name — a blank, a tab, a newline, a quote, a `v` prefix — has
no matcher at all and is rejected as `unexpected` at its position, never
skipped, never swallowed. The punctuation is unbound rather than merely
unused so that a `{` in the input is not lexed as JSON's `#OB`: with the
binding in place, `1.2.3{` would fail as a well-formed token the grammar
did not want, expecting only end-of-source; unbound, it fails as an
unknown character, and the diagnostic lists the grammar's own tokens as
what was expected. (The engine's slots for the six still exist — the
debug model lists them, without source text — but nothing in the input
can produce them.) Turn the space, line or comment lexer back on and the
plugin accepts strings the specification rejects; the options ride on
the spec precisely so that they can only arrive together with the
grammar.

```js
import { Tabnas } from '@tabnas/parser'
import { Semver } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

let code
try { tn.parse(' 1.2.3') } catch (e) { code = e.code }
code // => 'unexpected'
try { tn.parse('') } catch (e) { code = e.code }
code // => 'unexpected'
tn.parse('1.0.0-true.null').prerelease // => ['true', 'null']
```

## Why there are no error codes

Every rejection is the engine's base `unexpected` code, raised where the
grammar has no alternative for the next character. The plugin declares
no code of its own — `tabnas.plugin.json` lists an empty `errorCodes` —
and adds only a `hint` for `unexpected` that says what a version has to
look like and links to the specification.

That is a decision, not a gap. The grammar is the sole acceptor, and the
compiler offers no safe place for an error production: a trap
alternative at a leading position is inlined by Paull's substitution
(above), and a nullable trap inlined there would change the accepted
language rather than merely label a rejection. A code that can only be
raised from some positions is worse than none. The fixtures pin the
contract instead: all 141 rows of `test/spec/strict.tsv` expect
`ERROR:unexpected`, compared exactly in both runtimes. The position
reported at a lookahead failure may differ between the runtimes; the
code is the contract, the position is not.

## Conformance

The claim is that `@tabnas/semver` accepts exactly the strings the
semver.org grammar accepts, and produces the parts the specification
names for each. The judge is not this repository: semver.org publishes,
in its FAQ, a regular expression that recognises the language of its
grammar, and `ts/test/oracle.test.ts` (with `go/oracle_test.go` as its
twin) grades every string of a generated corpus against it. The plugin's
verdict must equal the expression's; on every accepted string the value
must match the expression's captures and `format` must return the
input.

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
`compare` chain and the equal pairs. Both runtimes auto-discover and run
every file. A new parse case belongs there; the in-language suites keep
only what a `.tsv` cannot express — `bigint` values, function results,
error details.

## Relationship to the Go port

The plugin ships in two implementations built from the one grammar:
`embed-grammar.js` copies `semver-grammar.abnf` verbatim into both
`src/semver.ts` and `go/semver.go`, and the Go port compiles it with
`github.com/tabnas/abnf/go` at install, sets the same engine options,
and runs the same shared fixtures and the same corpus with the same
pinned census and hash. This TypeScript version is canonical: when the
two disagree on parse behaviour, Go changes to match — unless Go has
exposed a TypeScript defect, in which case TypeScript is fixed first,
as happened with both toolchain fixes below.

The value has the same shape modulo the host language: a
`map[string]any` with the same five keys, `float64` where this side has
`number` and `*big.Int` where it has `bigint`, switching at the same
`MaxSafeInteger`; `prerelease` is a `[]any` of `float64`/`*big.Int` and
`string`, and `build` a `[]any` of `string`. Where this side throws a
`TabnasError`, Go returns a `*tabnas.TabnasError` with
`Code == "unexpected"` as a second value, and `Compare` and `Format`
return an error for an argument that is not a parsed value. Go also has
a `Parse` convenience over one cached instance behind a mutex, which the
TypeScript side deliberately lacks. See
[../../go/doc/concepts.md](../../go/doc/concepts.md).

## A note on two toolchain fixes

The oracle corpus found two defects in the TypeScript toolchain, both
already right in the Go port: `@tabnas/bnf` now marks character-class
tokens eager, so a class can be lexed at any lookahead slot
([tabnas/bnf#33](https://github.com/tabnas/bnf/pull/33)), and the
`@tabnas/parser` lexer now tries the match tokens a rule expects at a
slot before the eager ones it does not
([tabnas/parser#161](https://github.com/tabnas/parser/pull/161)). The
plugin carries the first itself — after compiling, it sets `eager$` on
every class token; the published emitter leaves the flag unset, so there
the loop is what makes the difference, and it is a no-op once the
emitter sets it. The second lives in the engine, and this grammar does
not need it: its three classes and four literals are pairwise disjoint,
so no character can be cut two ways and the order the lexer tries
tokens in cannot matter. So an isolated `npm install` against the
published `@tabnas/bnf` 0.1.10 and `@tabnas/parser` 0.9.0 passes the
whole suite, oracle corpus included. Without the port it rejected
strings such as `1.0.0-01a` and `1.0.0-12a`: the `*digit` helper peeks
two digits, and the letter that ends the run lexed as a fatal bad token
at the second slot. The fleet layout, with sibling checkouts linked into
`node_modules`, gets the same behaviour from the fixed toolchain, and
the Go module never needed either fix.
