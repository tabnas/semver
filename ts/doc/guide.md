# How-to guide

Short, task-focused recipes for `@tabnas/semver`. Each one stands on its
own and assumes the package is installed and you have parsed a version
at least once (the [tutorial](tutorial.md) covers that from nothing).
The [reference](reference.md) lists every export, every field of the
value and the complete syntax accepted; the [concepts](concepts.md)
page explains why the plugin is built the way it is.

Every recipe starts from the same two imports and the same instance:

```js
import { Tabnas } from '@tabnas/parser'
import { Semver, compare, format, grammar, VERSION } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)
```

## Install the plugin and reuse the instance

```bash
npm install @tabnas/parser @tabnas/abnf @tabnas/semver
```

`@tabnas/parser` is the engine and `@tabnas/abnf` is the compiler that
turns the plugin's grammar into engine rules; both are peer
dependencies, so install them alongside. `Semver` is a plugin, not a
standalone parser: install it on a bare engine, then call `.parse()`.

```js
import { Tabnas } from '@tabnas/parser'
import { Semver } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

tn.parse('1.2.3-alpha.1+build.5') // => { major: 1, minor: 2, patch: 3, prerelease: ['alpha', 1], build: ['build', '5'] }
tn.parse('1.2.3') // => { major: 1, minor: 2, patch: 3, prerelease: [], build: [] }

Semver.defaults // => {}
```

`new Tabnas({ plugins: [Semver] })` builds the same instance. There are
no options — `Semver.defaults` is `{}` — so `.use(Semver, {})` is
accepted and changes nothing.

Build the instance once and keep it. Installing the plugin compiles the
ABNF grammar into the engine's rule set, which takes about 75 ms; a
parse takes about 100 µs. A module-level constant is the usual shape:

```js
import { Tabnas } from '@tabnas/parser'
import { Semver } from '@tabnas/semver'

// Once, when the module loads.
const tn = new Tabnas().use(Semver)

// Then from anywhere, as often as needed.
const parseVersion = (text) => tn.parse(text)
```

## Validate a string without using the value

There is no boolean API: `tn.parse` returns the value or throws. Wrap
it, and test the error's `code` — every rejection is the engine's
`unexpected` code, so anything else is a bug worth rethrowing, not an
invalid version.

```js
import { Tabnas } from '@tabnas/parser'
import { Semver } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

function isSemver(text) {
  try {
    tn.parse(text)
    return true
  } catch (err) {
    if ('unexpected' === err.code) return false
    throw err
  }
}

isSemver('1.2.3') // => true
isSemver('1.0.0-x-y-z.--') // => true
isSemver('v1.2.3') // => false
isSemver('') // => false
isSemver('1.2.3 ') // => false
isSemver('1.2.3-01') // => false
```

The plugin never trims: a leading or trailing blank or a newline is
rejected like any other character. If the text comes from a file or a
command line that may carry one, `text.trim()` before parsing is your
decision to make, not the parser's.

## Split a version into its parts

The value is a plain object with the five parts the specification
names, always all present. Destructure it:

```js
import { Tabnas } from '@tabnas/parser'
import { Semver } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

const { major, minor, patch, prerelease, build } = tn.parse('2.5.1-rc.3+20260909.git.abcdef0')

major // => 2
minor // => 5
patch // => 1
prerelease // => ['rc', 3]
build // => ['20260909', 'git', 'abcdef0']
```

`prerelease` and `build` are empty arrays when the version has neither.
A pre-release identifier that is all digits comes back as a number and
any other as a string, because the specification compares the two
kinds differently; `typeof` tells them apart. Build identifiers are
always strings, leading zeros included, since they take no part in
precedence.

```js
import { Tabnas } from '@tabnas/parser'
import { Semver } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

tn.parse('1.0.0-x.7.z.92').prerelease // => ['x', 7, 'z', 92]
tn.parse('1.0.0-0.3.7').prerelease // => [0, 3, 7]
tn.parse('1.0.0-beta.11').prerelease.map((id) => typeof id) // => ['string', 'number']
tn.parse('1.0.0-alpha+001').build // => ['001']
tn.parse('1.0.0').prerelease // => []
```

## Sort a list of versions

`compare(a, b)` returns `-1`, `0` or `1` by the specification's
precedence rules (§11) — exactly the comparator `Array.prototype.sort`
takes. Parse the strings first, sort the values, and `format` them back
if you need strings again:

```js
import { Tabnas } from '@tabnas/parser'
import { Semver, compare, format } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

const tags = ['2.0.0', '1.0.0-rc.1', '1.0.0', '1.0.0-alpha', '1.0.0-beta.11', '1.0.0-beta.2']

const ascending = tags.map((s) => tn.parse(s)).sort(compare)
ascending.map(format) // => ['1.0.0-alpha', '1.0.0-beta.2', '1.0.0-beta.11', '1.0.0-rc.1', '1.0.0', '2.0.0']

const descending = tags.map((s) => tn.parse(s)).sort((a, b) => compare(b, a))
descending.map(format) // => ['2.0.0', '1.0.0', '1.0.0-rc.1', '1.0.0-beta.11', '1.0.0-beta.2', '1.0.0-alpha']

const latest = tags.map((s) => tn.parse(s)).reduce((best, v) => 0 < compare(v, best) ? v : best)
format(latest) // => '2.0.0'
```

Do not sort the strings. A plain string sort ranks `beta.11` before
`beta.2` (numeric identifiers compare numerically, not character by
character) and puts `1.0.0` before its own pre-releases (a pre-release
ranks below its normal version):

```js
const tags = ['2.0.0', '1.0.0-rc.1', '1.0.0', '1.0.0-alpha', '1.0.0-beta.11', '1.0.0-beta.2']

[...tags].sort() // => ['1.0.0', '1.0.0-alpha', '1.0.0-beta.11', '1.0.0-beta.2', '1.0.0-rc.1', '2.0.0']
```

## Check whether a version is a pre-release

A version is a pre-release exactly when `prerelease` is non-empty.
Build metadata on its own does not make one:

```js
import { Tabnas } from '@tabnas/parser'
import { Semver, format } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

const isPrerelease = (v) => 0 < v.prerelease.length

isPrerelease(tn.parse('1.0.0-rc.1')) // => true
isPrerelease(tn.parse('1.0.0-0')) // => true
isPrerelease(tn.parse('1.0.0')) // => false
isPrerelease(tn.parse('1.0.0+build.7')) // => false

const stable = ['1.0.0-rc.1', '1.0.0', '2.0.0-beta.1', '1.5.0+ci.9'].map((s) => tn.parse(s)).filter((v) => !isPrerelease(v))
stable.map(format) // => ['1.0.0', '1.5.0+ci.9']
```

## Ignore build metadata when comparing

Nothing to do: `compare` never looks at `build` (§10 of the
specification). Two versions that differ only in build metadata have
the same precedence, and a version with metadata equals the same
version without it:

```js
import { Tabnas } from '@tabnas/parser'
import { Semver, compare, format } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

compare(tn.parse('1.0.0+a'), tn.parse('1.0.0+b')) // => 0
compare(tn.parse('1.0.0'), tn.parse('1.0.0+20130313144700')) // => 0
compare(tn.parse('1.0.0-alpha+exp.sha.5114f85'), tn.parse('1.0.0-alpha')) // => 0
```

Two consequences to plan for. `Array.prototype.sort` is stable, so
versions of equal precedence keep their input order; and if you need
one key per precedence class — to deduplicate, say — build it from the
value with `build` cleared:

```js
import { Tabnas } from '@tabnas/parser'
import { Semver, compare, format } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

['1.0.0+b', '1.0.0+a', '1.0.0'].map((s) => tn.parse(s)).sort(compare).map(format) // => ['1.0.0+b', '1.0.0+a', '1.0.0']

const precedenceKey = (v) => format({ ...v, build: [] })
precedenceKey(tn.parse('1.0.0-alpha+exp.sha.5114f85')) // => '1.0.0-alpha'
```

When you do want to tell two builds of the same version apart, compare
`build` yourself — or the formatted strings, since `format` keeps it.

## Render a value back to a string

`format` turns a value into its version string. For a value that came
out of the parser it is the exact input, character for character —
the plugin never normalises, because the grammar admits nothing that
could be normalised (no leading zeros in numbers; build metadata keeps
its zeros as strings). The conformance suite checks this round trip on
every accepted string of its corpus.

```js
import { Tabnas } from '@tabnas/parser'
import { Semver, format } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

const text = '1.0.0-beta+exp.sha.5114f85'
format(tn.parse(text)) // => '1.0.0-beta+exp.sha.5114f85'
format(tn.parse(text)) === text // => true
format(tn.parse('1.0.0+21AF26D3----117B344092BD')) // => '1.0.0+21AF26D3----117B344092BD'
```

`format` also renders a value you built by hand, with numbers or
bigints in the numeric slots and strings or numbers in `prerelease`:

```js
import { format } from '@tabnas/semver'

format({ major: 1, minor: 4, patch: 0, prerelease: ['beta', 2], build: [] }) // => '1.4.0-beta.2'
format({ major: 1, minor: 4, patch: 0, prerelease: [], build: ['ci', '42'] }) // => '1.4.0+ci.42'
```

It does not check what you give it. If a hand-built value must be a
valid version, parse the result — the grammar is the only judge:

```js
import { Tabnas } from '@tabnas/parser'
import { Semver, format } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

const risky = format({ major: 1, minor: 2, patch: 3, prerelease: ['01'], build: [] })
risky // => '1.2.3-01'

let valid = true
try { tn.parse(risky) } catch (err) { valid = false }
valid // => false
```

## Handle integers beyond 2^53

The specification puts no upper bound on an integer. `major`, `minor`,
`patch` and a numeric pre-release identifier are a `number` up to
`Number.MAX_SAFE_INTEGER` (2^53 − 1 = 9007199254740991) and a `bigint`
beyond it, so no digit is ever rounded away. Each slot switches on its
own value:

```js
import { Tabnas } from '@tabnas/parser'
import { Semver, compare, format } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

const v = tn.parse('9007199254740992.0.0')
typeof v.major // => 'bigint'
v.major // => 9007199254740992n
typeof v.minor // => 'number'
typeof tn.parse('9007199254740991.0.0').major // => 'number'
tn.parse('1.0.0-18446744073709551616').prerelease // => [18446744073709551616n]
```

`compare` and `format` take both representations, and JavaScript's
`<` and `>` compare a `number` with a `bigint` correctly. Arithmetic
does not mix them — `v.major + 1` throws a `TypeError` — so convert
with `BigInt()`, which is exact for a safe integer too:

```js
import { Tabnas } from '@tabnas/parser'
import { Semver, compare, format } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

const v = tn.parse('9007199254740992.0.0')
compare(v, tn.parse('9007199254740991.0.0')) // => 1
format(v) // => '9007199254740992.0.0'
v.major > Number.MAX_SAFE_INTEGER // => true
BigInt(v.major) + 1n // => 9007199254740993n
BigInt(tn.parse('1.2.3').major) + 1n // => 2n
```

`JSON.stringify` throws on a `bigint`. If a value may carry one and
has to be serialised, supply a replacer that renders it as a digit
string (which is also what `format` does):

```js
import { Tabnas } from '@tabnas/parser'
import { Semver } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

const asDigits = (key, x) => 'bigint' === typeof x ? x.toString() : x
JSON.stringify(tn.parse('9007199254740992.0.0'), asDigits) // => '{"major":"9007199254740992","minor":0,"patch":0,"prerelease":[],"build":[]}'
```

## Read a structured diagnostic

The thrown error is a `SyntaxError` carrying `code`, `lineNumber`,
`columnNumber` and a multi-line `message` that includes the plugin's
hint about what a version has to look like. For a machine-readable
report, `JSON.stringify` the error: the result has `status`, `code`,
the one-line `message`, the `hint`, the position as `row`, `col`,
`pos` and `len`, the input as `src`, and the grammar `rule` that was
active at the failure (which may be a compiler-generated helper name).

```js
import { Tabnas } from '@tabnas/parser'
import { Semver } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

let report
try {
  tn.parse('v1.2.3')
} catch (err) {
  err instanceof SyntaxError // => true
  err.code // => 'unexpected'
  err.lineNumber // => 1
  err.columnNumber // => 1
  report = JSON.parse(JSON.stringify(err))
}

report.status // => 'failure'
report.code // => 'unexpected'
report.message // => 'unexpected character(s): v'
report.row // => 1
report.col // => 1
report.pos // => 0
report.len // => 1
report.src // => 'v1.2.3'
report.hint.includes('https://semver.org/spec/v2.0.0.html') // => true
```

The position is the first character the grammar could not place — for
`'1.2.3 '` that is the trailing blank at column 6:

```js
import { Tabnas } from '@tabnas/parser'
import { Semver } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

let col
try { tn.parse('1.2.3 ') } catch (err) { col = err.columnNumber }
col // => 6
```

There are no plugin-specific error codes: `unexpected` is the code for
every rejection, so branch on the position and the input, not on the
code. Treat `src` and every identifier in a value as untrusted text —
see [AGENTS.md](../../AGENTS.md#untrusted-input).

## Get the ABNF text for tooling

The `grammar` export is the RFC 5234 ABNF the plugin compiles at
install time — the repository's [`semver-grammar.abnf`](../../semver-grammar.abnf),
comments included, embedded verbatim. `VERSION` is the package version.

```js
import { grammar, VERSION } from '@tabnas/semver'

typeof grammar // => 'string'
typeof VERSION // => 'string'
grammar.includes('valid-semver = version-core [ "-" pre-release ] [ "+" build ]') // => true

const productions = grammar.split('\n').filter((line) => /^[a-z-]+\s*=/.test(line)).map((line) => line.split(/\s*=/)[0])
productions.length // => 17
productions.slice(0, 3) // => ['semver', 'valid-semver', 'version-core']
productions.includes('pre-release-identifier') // => true
```

Write it out for any tool that reads ABNF, or hand it to
`@tabnas/abnf` yourself — `abnfConvert(grammar, { start: 'semver' })`
is the compile step the plugin performs, before it adds its one action
and switches the engine's default lexers off (the
[concepts](concepts.md) page has the details):

```js
import { writeFileSync } from 'node:fs'
import { grammar } from '@tabnas/semver'

writeFileSync('semver.abnf', grammar)
```
