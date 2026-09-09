# Reference

The complete public surface of `@tabnas/semver` (TypeScript): the
package, every export, the parse entry, the value, `compare`, `format`,
`grammar`, the exact syntax accepted, the tokens, the errors. For a
guided introduction see the [tutorial](tutorial.md); for task recipes
see the [how-to guide](guide.md); for how it works and why see
[concepts](concepts.md).

## Package

```bash
npm install @tabnas/parser @tabnas/abnf @tabnas/semver
```

| | |
|---|---|
| Package | `@tabnas/semver` |
| Module type | CommonJS (`main: dist/semver.js`, types `dist/semver.d.ts`) |
| Peer deps | `@tabnas/parser` >= 0, `@tabnas/abnf` >= 0 |
| Engine | `@tabnas/parser` (Tabnas) |
| Underlying compiler | `@tabnas/abnf` (RFC 5234 ABNF to engine rules, over `@tabnas/bnf`) |
| Node | >= 24 |
| CLI | none |

The grammar is compiled from ABNF when the plugin is installed, not at
build time: `@tabnas/abnf` must be resolvable at runtime.

## Exports

| Export | Kind | Description |
|---|---|---|
| `Semver` | `Plugin` | The plugin function. Register with `engine.use(Semver)`. |
| `compare` | `(a: Version, b: Version) => -1 \| 0 \| 1` | Precedence per specification section 11. See [compare](#compare). |
| `format` | `(v: Version) => string` | A value back to its version string. See [format](#format). |
| `grammar` | `string` | The ABNF text the plugin compiles. See [grammar](#grammar). |
| `VERSION` | `string` | This package's version, always equal to `package.json` "version" (currently `'0.1.0'`). |
| `Version` | type | The parse result (see [The value](#the-value)). |
| `PrereleaseIdentifier` | type | `string \| SemverNumber` — one pre-release identifier. |
| `SemverNumber` | type | `number \| bigint` — one integer component. |
| `SemverOptions` | type | `Record<string, never>` — the (empty) options shape. |

```typescript
type SemverNumber = number | bigint
type PrereleaseIdentifier = string | SemverNumber
type Version = {
  major: SemverNumber
  minor: SemverNumber
  patch: SemverNumber
  prerelease: PrereleaseIdentifier[]
  build: string[]
}
type SemverOptions = Record<string, never>
```

`Semver.defaults` (a `SemverOptions`) is `{}`.

## Parse entry

The plugin has **no convenience `parse()` function** of its own. You
parse by building a Tabnas engine, installing `Semver`, and calling the
engine's `.parse()`:

```js
import { Tabnas } from '@tabnas/parser'
import { Semver } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

tn.parse('1.2.3-alpha.1+build.5') // => { major: 1, minor: 2, patch: 3, prerelease: ['alpha', 1], build: ['build', '5'] }
```

`new Tabnas({ plugins: [Semver] })` is equivalent to `new Tabnas().use(Semver)`.

### `engine.use(Semver, options?)`

Registers and immediately applies the plugin. Returns the engine, so
registrations chain. `options` is optional and, when given, must be
`{}` (see [Options](#options)). Installing compiles the embedded ABNF
(`grammar`) with `@tabnas/abnf` — start rule `semver`, group tag
`semver` — into the engine's rule set, attaches the one semantic action
(an after-close hook on `semver`, `@semver:ac`, which replaces the parse
tree with the `Version` built from the accepted text), applies the lexer
settings under [Tokens](#tokens), and sets the `hint` under
[Errors](#errors). Compiling is the expensive step; build one instance
and reuse it (see [Performance](#performance)).

### `engine.parse(src)`

Parses one version string and returns a `Version`. The whole of `src`
must be a single version: there is no leading or trailing whitespace, no
line end, no prefix and nothing after the version. A rejected string,
the empty string included, throws (see [Errors](#errors)).

## Options

There are none. `SemverOptions` is `Record<string, never>`, so
`tn.use(Semver, {})` type-checks and any key is a type error;
`Semver.defaults` is `{}`.

## The value

`engine.parse` returns a plain object (prototype `Object.prototype`)
with exactly these five keys, in this order:

| Field | Type | Contents |
|---|---|---|
| `major` | `SemverNumber` | MAJOR, an integer. |
| `minor` | `SemverNumber` | MINOR, an integer. |
| `patch` | `SemverNumber` | PATCH, an integer. |
| `prerelease` | `PrereleaseIdentifier[]` | The pre-release identifiers, left to right; `[]` when the version has no `-` part. |
| `build` | `string[]` | The build identifiers, left to right; `[]` when the version has no `+` part. |

```js
import { Tabnas } from '@tabnas/parser'
import { Semver } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

tn.parse('1.0.0') // => { major: 1, minor: 0, patch: 0, prerelease: [], build: [] }
tn.parse('1.0.0-0.3.7') // => { major: 1, minor: 0, patch: 0, prerelease: [0, 3, 7], build: [] }
tn.parse('1.0.0-x-y-z.--') // => { major: 1, minor: 0, patch: 0, prerelease: ['x-y-z', '--'], build: [] }
tn.parse('1.0.0-alpha+001') // => { major: 1, minor: 0, patch: 0, prerelease: ['alpha'], build: ['001'] }
tn.parse('1.0.0+21AF26D3----117B344092BD') // => { major: 1, minor: 0, patch: 0, prerelease: [], build: ['21AF26D3----117B344092BD'] }
Object.keys(tn.parse('1.0.0')) // => ['major', 'minor', 'patch', 'prerelease', 'build']
```

### Integers: `number` up to 2^53 - 1, `bigint` beyond

`major`, `minor`, `patch` and every numeric pre-release identifier are
a `number` when the value is at most `Number.MAX_SAFE_INTEGER`
(9007199254740991) and a `bigint` above it. The switch is per
component, so one value can mix the two. The specification places no
upper bound on an integer; the plugin never rounds.

```js
import { Tabnas } from '@tabnas/parser'
import { Semver } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

tn.parse('9007199254740991.0.0').major // => 9007199254740991
tn.parse('9007199254740992.0.0').major // => 9007199254740992n
typeof tn.parse('9007199254740992.0.0').major // => 'bigint'
tn.parse('1.0.0-9007199254740993').prerelease // => [9007199254740993n]
```

`JSON.stringify` throws on a `bigint`; use `format` when a value with a
`bigint` component has to travel as text.

### Pre-release identifiers: numeric or string

A pre-release identifier made only of digits is numeric (a `number` or
`bigint`, as above); the grammar has already rejected a leading zero in
that case (`1.2.3-01` does not parse). Every other identifier is a
string, including ones that begin with digits (`'1a'`, `'01a'`,
`'007a'`) and ones that are only hyphens (`'-'`, `'--'`). The
distinction is the specification's own: section 11.4 compares the two
kinds differently.

### Build identifiers: always strings

`build` holds strings only, whatever the identifier looks like:
`'001'`, `'0'`, `'20130313144700'`. Leading zeros are kept, and build
metadata takes no part in precedence.

## compare

```typescript
function compare(a: Version, b: Version): -1 | 0 | 1
```

Returns `-1` when `a` has lower precedence than `b`, `1` when higher,
`0` when the two have the same precedence. The rules are those of
specification section 11, applied in this order:

1. `major`, then `minor`, then `patch`, compared numerically (11.2). A
   `number` and a `bigint` compare correctly with each other.
2. With an equal core, a version that has a pre-release part ranks
   **below** the version without one (11.3).
3. With both having a pre-release part, identifiers are compared left to
   right, stopping at the first difference (11.4):
   - two numeric identifiers compare numerically (11.4.1);
   - two alphanumeric identifiers compare lexically in ASCII order
     (11.4.2) — so `'Z'` ranks below `'a'`;
   - a numeric identifier ranks below an alphanumeric one (11.4.3);
   - when every identifier of the shorter list equals its counterpart,
     the longer list ranks higher (11.4.4).
4. `build` is ignored entirely (sections 10 and 11.1): `1.0.0+a` and
   `1.0.0+b` compare `0`.

```js
import { Tabnas } from '@tabnas/parser'
import { Semver, compare } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

compare(tn.parse('1.0.0-alpha'), tn.parse('1.0.0-alpha.1')) // => -1
compare(tn.parse('1.0.0-alpha.1'), tn.parse('1.0.0-alpha.beta')) // => -1
compare(tn.parse('1.0.0-beta.2'), tn.parse('1.0.0-beta.11')) // => -1
compare(tn.parse('1.0.0-1'), tn.parse('1.0.0-a')) // => -1
compare(tn.parse('1.0.0-Z'), tn.parse('1.0.0-a')) // => -1
compare(tn.parse('1.0.0-rc.1'), tn.parse('1.0.0')) // => -1
compare(tn.parse('1.0.0+a'), tn.parse('1.0.0+b')) // => 0
compare(tn.parse('2.1.1'), tn.parse('2.1.0')) // => 1
```

The specification's own chain holds in full: `1.0.0-alpha` <
`1.0.0-alpha.1` < `1.0.0-alpha.beta` < `1.0.0-beta` < `1.0.0-beta.2` <
`1.0.0-beta.11` < `1.0.0-rc.1` < `1.0.0`, and `1.0.0` < `2.0.0` <
`2.1.0` < `2.1.1`. `compare` is a total order over versions and can be
passed to `Array.prototype.sort` as is. It does not validate its
arguments: pass values that came from `parse` or that satisfy `Version`.

## format

```typescript
function format(v: Version): string
```

Renders `major.minor.patch`, then `-` and the pre-release identifiers
joined with `.` when `prerelease` is non-empty, then `+` and the build
identifiers joined with `.` when `build` is non-empty. A `bigint`
renders as plain digits (no `n`). For a value that came out of `parse`
the result is exactly the input string. `format` does not validate its
argument.

```js
import { Tabnas } from '@tabnas/parser'
import { Semver, format } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

format(tn.parse('1.0.0-beta+exp.sha.5114f85')) // => '1.0.0-beta+exp.sha.5114f85'
format(tn.parse('9007199254740992.0.0')) // => '9007199254740992.0.0'
format({ major: 1n, minor: 0, patch: 0, prerelease: ['rc', 1n], build: ['a'] }) // => '1.0.0-rc.1+a'
```

## grammar

```typescript
const grammar: string
```

The complete text of [`semver-grammar.abnf`](../../semver-grammar.abnf),
comments included, exactly as the plugin compiles it. It is exported for
tooling (documentation, railroad diagrams, a second compiler); the plugin
itself reads the embedded copy, so changing this string changes nothing.

```js
import { grammar, VERSION } from '@tabnas/semver'

typeof grammar // => 'string'
/^valid-semver = version-core \[ "-" pre-release \] \[ "\+" build \]$/m.test(grammar) // => true
/^\d+\.\d+\.\d+$/.test(VERSION) // => true
```

## Syntax accepted

The language is exactly that of the specification's grammar (section
"Backus–Naur Form Grammar for Valid SemVer Versions"). These are the
productions the plugin compiles, comments stripped; every name is the
specification's, with spaces written as hyphens:

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

`semver` is an alias of `valid-semver` and the start rule. Two
productions differ from the specification in shape but not in language:
`pre-release-identifier` is factored on its first character, and
`build-identifier` is `1*identifier-character`, the union of the
specification's `<alphanumeric identifier>` and `<digits>`. The grammar
file explains both.

Accepted, with the value produced:

| Input | `prerelease` | `build` |
|---|---|---|
| `1.0.0` | `[]` | `[]` |
| `1.0.0-alpha` | `['alpha']` | `[]` |
| `1.0.0-alpha.1` | `['alpha', 1]` | `[]` |
| `1.0.0-0.3.7` | `[0, 3, 7]` | `[]` |
| `1.0.0-x.7.z.92` | `['x', 7, 'z', 92]` | `[]` |
| `1.0.0-x-y-z.--` | `['x-y-z', '--']` | `[]` |
| `1.0.0-1a` | `['1a']` | `[]` |
| `1.0.0-01a` | `['01a']` | `[]` |
| `1.0.0-alpha+001` | `['alpha']` | `['001']` |
| `1.0.0+20130313144700` | `[]` | `['20130313144700']` |
| `1.0.0-beta+exp.sha.5114f85` | `['beta']` | `['exp', 'sha', '5114f85']` |
| `1.0.0+21AF26D3----117B344092BD` | `[]` | `['21AF26D3----117B344092BD']` |
| `1.0.0+01` | `[]` | `['01']` |

Rejected, every one with error code `unexpected`:

| Input | Why |
|---|---|
| `` (empty) | Not a version. |
| `1`, `1.2`, `1.2.3.4` | The core is exactly three components. |
| `01.2.3`, `1.02.3` | Leading zero in a core component. |
| `v1.2.3` | No prefix of any kind. |
| ` 1.2.3`, `1.2.3 `, `1.2.3\n` | No whitespace or line end anywhere. |
| `1.2.3-`, `1.2.3+`, `1.2.3-a..b` | No empty identifier. |
| `1.2.3-01` | Leading zero in a numeric pre-release identifier. |
| `1.2.3-a_b`, `1.2.3-α` | Only `[0-9A-Za-z-]` in an identifier. |
| `1.2.3+a+b` | One `+` only. |

The shared fixtures under [`test/spec/`](../../test/spec/) list many
more of each kind.

Note: the published `@tabnas/bnf` (0.1.10) does not yet mark
character-class tokens eager, which is what lets the letter in
`1.0.0-01a` or `1.0.0-12a` (digits then a non-digit in a pre-release
identifier) be lexed after a digit run. The plugin sets the flag itself
after compiling, so an isolated `npm install` from the registry accepts
them as the grammar says; the upstream fixes and the reason the port is
enough for this grammar are in [`AGENTS.md`](../../AGENTS.md), "The
tabnas engine dependency".

## Tokens

The compiled grammar brings seven tokens of its own. One character is
one token; there are no multi-character tokens.

| Token | Source | Kind | Grammar use |
|---|---|---|---|
| `#0` | `0` | fixed | the literal `"0"` |
| `#T` | `.` | fixed | the separator `"."` |
| `#T1` | `-` | fixed | the pre-release opener and the identifier character `"-"` |
| `#T2` | `+` | fixed | the build opener `"+"` |
| `#RX___U0031__U0039` | `[1-9]` | match (regex class) | `positive-digit` |
| `#RX___U0041__U005A` | `[A-Z]` | match (regex class) | `letter` |
| `#RX___U0061__U007A` | `[a-z]` | match (regex class) | `letter` |

Everything the engine lexes by default is switched off: `space`,
`line`, `comment`, `string`, `number`, `text` and `value` are all
`lex: false`. The engine's six JSON punctuation tokens — `#OB` `{`,
`#CB` `}`, `#OS` `[`, `#CS` `]`, `#CL` `:`, `#CA` `,` — are unbound
(their names stay registered, so introspection still lists them, but no
source text produces them). `lex.empty` is `false`, so the empty string
is an error rather than the engine's default `undefined`.

The effect: a character the grammar does not name — a blank, a tab, a
newline, a quote, a `#`, a `v` — has no matcher at all and is reported
as `unexpected` where it stands, never skipped as whitespace or
swallowed as a comment or string.

## Grammar group tag

Every alternative the plugin installs carries the group tag `semver`
(the `tag` passed to the ABNF compiler). Excluding the group with the
engine's `rule.exclude` option removes all of them, after which every
parse fails:

```js
import { Tabnas } from '@tabnas/parser'
import { Semver } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)
tn.options({ rule: { exclude: 'semver' } })

let code
try { tn.parse('1.2.3') } catch (e) { code = e.code }
code // => 'unexpected'
```

`engine.options(...)` returns the merged options object, not the
engine. The plugin adds nothing to any other group.

## Errors

Every rejection throws the engine's `TabnasError`, a subclass of
`SyntaxError` (`err.name === 'SyntaxError'`), and every rejection has
the same code, **`unexpected`**: the grammar had no alternative for the
next character. Fields on the thrown object:

| Field | Type | Meaning |
|---|---|---|
| `code` | `string` | Always `'unexpected'`. |
| `lineNumber` | `number` | Line of the offending character, 1-based. |
| `columnNumber` | `number` | Column of the offending character, 1-based. |
| `message` | `string` | Multi-line: a header `[tabnas/unexpected]: unexpected character(s): <char>`, a source extract with a caret, then the hint. ANSI-coloured by default; the engine option `color: { active: false }` turns colour off. |

`JSON.stringify(err)` gives the structured diagnostic:

| Key | Contents |
|---|---|
| `status` | `'failure'` |
| `code` | `'unexpected'` |
| `message` | The one-line description, `unexpected character(s): <char>`. |
| `hint` | The plugin's explanation of what a version must look like (below). |
| `row`, `col` | Position of the offending character, 1-based. |
| `pos` | Offset of the offending character, 0-based. |
| `len` | Length of the offending text (`0` when the input ended too early). |
| `rule` | The rule active at the failure. |
| `ruleStack` | The rule names from `__start__` down to `rule`. |
| `token` | `{ name, src }`: the token the lexer produced and its source text. |
| `expected` | The token names the active rule could have accepted. |
| `src` | The source text. |
| `plugins` | `['Semver']`. |
| `version` | The engine version. |

```js
import { Tabnas } from '@tabnas/parser'
import { Semver } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

let err
try { tn.parse('1.2.3-a_b') } catch (e) { err = e }

err instanceof SyntaxError // => true
err.code // => 'unexpected'
err.lineNumber // => 1
err.columnNumber // => 8
err.message.includes('unexpected character(s): _') // => true
err.message.includes('https://semver.org/spec/v2.0.0.html') // => true

const diag = JSON.parse(JSON.stringify(err))
diag.status // => 'failure'
diag.message // => 'unexpected character(s): _'
diag.col // => 8
diag.pos // => 7
diag.token // => { name: '#BD', src: '_' }
diag.src // => '1.2.3-a_b'

let empty
try { tn.parse('') } catch (e) { empty = e.code }
empty // => 'unexpected'
```

The hint, with `{src}` replaced by the offending character(s):

```text
The character(s) {src} do not match any rule alternative active at
this position.

A Semantic Version is MAJOR.MINOR.PATCH — three integers with no
leading zeros — optionally followed by -PRERELEASE and +BUILD, each a
dot-separated list of non-empty identifiers made of [0-9A-Za-z-],
where a numeric pre-release identifier has no leading zero. Nothing
else is allowed: no whitespace, no "v" prefix, no empty identifier.
See https://semver.org/spec/v2.0.0.html
```

The code is the contract; the position is not. At a lookahead failure
(`01.2.3`, say) the reported column may differ from the Go port's and
may move with a compiler change; the shared fixtures pin
`ERROR:unexpected` only.

There are **no plugin-specific error codes**, deliberately. The grammar
is the sole acceptor, and the ABNF compiler offers no safe place for an
error production: a trap alternative in a leading position is inlined by
the compiler's substitution pass, and a nullable one inlined there would
change the accepted language rather than merely label a rejection. A
code that could only be raised from some positions would be worse than
none, so every rejection is the engine's base `unexpected` and the hint
carries the explanation. `tabnas.plugin.json` lists an empty
`errorCodes`; [`AGENTS.md`](../../AGENTS.md) records the decision.

## Performance

Installing the plugin compiles the ABNF into the engine's rule set (150
rules): about 75 ms. A parse on an installed engine is on the order of
100 µs. The two differ by roughly three orders of magnitude, so build
one instance — at module load, say — and reuse it for every parse; the
instance holds no per-parse state. There is no module-level cached
instance and no convenience `parse()` in this package; the engine is
yours to build and keep.
