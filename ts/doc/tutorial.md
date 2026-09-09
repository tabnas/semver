# Tutorial — your first semver parse

This walks you from nothing to a working parse, then through a
comparison and a parse error. Follow it in order; each step builds on
the last. When you finish you will have installed the plugin, parsed a
plain version and one with pre-release and build parts, put versions in
precedence order, and handled a string the specification rejects.

For a recipe-style index of individual tasks, see the
[how-to guide](guide.md). For exhaustive signatures and the full
syntax, see the [reference](reference.md). For how it all works, see
[concepts](concepts.md).

## 1. Install

`@tabnas/semver` is a grammar plugin: it has no parser of its own. It
runs on the Tabnas engine, and its grammar — the Semantic Versioning
2.0.0 grammar, written as ABNF — is compiled by `@tabnas/abnf` when the
plugin is installed. Install all three:

```bash
npm install @tabnas/parser @tabnas/abnf @tabnas/semver
```

`@tabnas/parser` and `@tabnas/abnf` are peer dependencies.

## 2. Build a parser

Create a Tabnas engine and install the plugin on it. The result is a
reusable parser instance:

```js
import { Tabnas } from '@tabnas/parser'
import { Semver } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

tn.parse('1.2.3') // => { major: 1, minor: 2, patch: 3, prerelease: [], build: [] }
```

Installing the plugin compiles the grammar into the engine's rule set,
which takes about 75 ms; a parse then takes about 100 µs. Build the
instance once and keep it rather than creating one per parse. The
plugin has no options, so `use(Semver)` is the whole configuration.

## 3. Read the five parts

Every parse returns a plain object with the five parts the
specification names, as five keys in this order:

```js
import { Tabnas } from '@tabnas/parser'
import { Semver } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)
const v = tn.parse('2.1.0')

v.major // => 2
v.minor // => 1
v.patch // => 0
v.prerelease // => []
v.build // => []
```

`major`, `minor` and `patch` are numbers. The two arrays are empty when
the version has no pre-release or build part, so you can always index
into them without checking first. (An integer larger than
`Number.MAX_SAFE_INTEGER` comes back as a `bigint` rather than a rounded
number; the [reference](reference.md) covers that case.)

## 4. Parse a pre-release and build version

A `-` after the version core opens the pre-release part and a `+` opens
the build metadata; inside each, `.` separates identifiers:

```js
import { Tabnas } from '@tabnas/parser'
import { Semver, format } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)
const v = tn.parse('1.2.3-alpha.1+build.5')

v.prerelease // => ['alpha', 1]
v.build // => ['build', '5']
format(v) // => '1.2.3-alpha.1+build.5'

tn.parse('1.0.0-x.7.z.92').prerelease // => ['x', 7, 'z', 92]
tn.parse('1.0.0-alpha+001').build // => ['001']
```

Look at the two `1`s. In the pre-release part, an identifier made only
of digits is *numeric* and becomes a number — the specification
compares numeric identifiers numerically, so the value keeps the
distinction — while an identifier containing a letter or a hyphen is
*alphanumeric* and stays a string. Build identifiers are always
strings: `5` comes back as `'5'`, and `001` keeps its zeros, because
build metadata plays no part in ordering. `format` renders a value
back to its string; for a parsed value that is exactly the text you
parsed.

## 5. Compare two versions

`compare` implements the specification's precedence rules (§11) over two
parsed values and returns `-1`, `0` or `1`. Here is the ascending chain
the specification gives as its own example, then a major bump and a
pair that differ only in build metadata:

```js
import { Tabnas } from '@tabnas/parser'
import { Semver, compare, format } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)
const p = (s) => tn.parse(s)

compare(p('1.0.0-alpha'), p('1.0.0-alpha.1')) // => -1
compare(p('1.0.0-alpha.1'), p('1.0.0-alpha.beta')) // => -1
compare(p('1.0.0-alpha.beta'), p('1.0.0-beta')) // => -1
compare(p('1.0.0-beta'), p('1.0.0-beta.2')) // => -1
compare(p('1.0.0-beta.2'), p('1.0.0-beta.11')) // => -1
compare(p('1.0.0-beta.11'), p('1.0.0-rc.1')) // => -1
compare(p('1.0.0-rc.1'), p('1.0.0')) // => -1
compare(p('2.0.0'), p('1.0.0')) // => 1
compare(p('1.0.0+a'), p('1.0.0+b')) // => 0

const ordered = ['1.0.0', '1.0.0-beta.11', '1.0.0-alpha', '1.0.0-beta.2'].map(p).sort(compare).map(format)
ordered // => ['1.0.0-alpha', '1.0.0-beta.2', '1.0.0-beta.11', '1.0.0']
```

Reading down the chain: major, minor and patch compare numerically; a
pre-release version ranks below its normal version (`1.0.0-rc.1` <
`1.0.0`); pre-release identifiers compare left to right — numeric ones
numerically (`beta.2` < `beta.11`, not alphabetically), alphanumeric
ones in ASCII order, and a numeric identifier always below an
alphanumeric one — and when every shared identifier is equal the longer
list ranks higher (`alpha` < `alpha.1`). Build metadata is ignored,
which is why `1.0.0+a` and `1.0.0+b` compare equal. Because the result
is `-1`, `0` or `1`, `compare` works directly as a sort comparator.

## 6. Catch a parse error

The parser accepts exactly what the specification's grammar accepts, and
nothing else. A `v` prefix is not part of a version, so parsing one
throws:

```js
import { Tabnas } from '@tabnas/parser'
import { Semver } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

let err
try {
  tn.parse('v1.2.3')
} catch (e) {
  err = e
}

err instanceof SyntaxError // => true
err.code // => 'unexpected'
err.lineNumber // => 1
err.columnNumber // => 1
err.message.includes('unexpected character(s): v') // => true

const diag = JSON.parse(JSON.stringify(err))
diag.status // => 'failure'
diag.hint.includes('https://semver.org/spec/v2.0.0.html') // => true

let code
try { tn.parse('1.2.3-01') } catch (e) { code = e.code }
code // => 'unexpected'
```

Every rejection carries the same code, `unexpected` — a leading zero in
a numeric pre-release identifier, whitespace anywhere, an empty
identifier and the empty string all raise it. The plugin declares no
error codes of its own: the grammar is the sole judge of validity, so
every error is the engine reporting the character it could not place.
What tells a reader what went wrong is the hint, a short statement of
what a version has to look like with a link to the specification. It is
printed as part of `err.message`, which is formatted for a terminal with
the source line and a caret under the offending character, and it is
the `hint` field of the structured diagnostic that `JSON.stringify(err)`
produces, alongside `status`, `code`, `row`, `col`, `pos` and `len`.

## Where to go next

- [How-to guide](guide.md) — focused recipes for individual tasks.
- [Reference](reference.md) — the public API, the value shape, every
  accepted syntax, and the error fields.
- [Concepts](concepts.md) — how the specification's grammar becomes the
  parser, and why the values look the way they do.
- [`semver-grammar.abnf`](../../semver-grammar.abnf) — the grammar
  itself, which is also exported as `grammar`.
