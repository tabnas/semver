# Reference

The complete public surface of `@tabnas/zon` (TypeScript): exports,
the parse entry, the two options, and the exact ZON syntax accepted.
For a guided introduction see the [tutorial](tutorial.md); for task
recipes see the [how-to guide](guide.md); for how it works see
[concepts](concepts.md).

## Package

```bash
npm install @tabnas/parser @tabnas/jsonic @tabnas/zon
```

| | |
|---|---|
| Package | `@tabnas/zon` |
| Module type | CommonJS (`main: dist/zon.js`, types `dist/zon.d.ts`) |
| Peer deps | `@tabnas/parser` >= 2, `@tabnas/jsonic` >= 2 |
| Engine | `@tabnas/parser` (Tabnas) |
| Underlying grammar | `@tabnas/jsonic` |

## Exports

| Export | Kind | Description |
|---|---|---|
| `Zon` | `Plugin` | The plugin function. Register with `engine.use(Zon, options)`. |
| `VERSION` | `string` | This package's version, always equal to `package.json` "version". |
| `ZonOptions` | type | The options object shape (see [Options](#options)). |

`Zon.defaults` (a `ZonOptions`) holds the merged default options:

```typescript
Zon.defaults = {
  charAsNumber: false,
  enumTag: null,
}
```

## Parse entry

The plugin has **no convenience `parse()` function** of its own. You
parse by building a Tabnas engine, layering the jsonic grammar, then
the `Zon` plugin, and calling the engine's `.parse()`:

```js
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Zon } from '@tabnas/zon'

const j = new Tabnas().use(jsonic).use(Zon)

j.parse('.{ .a = 1 }') // => { a: 1 }
```

### `engine.use(Zon, options?)`

Registers and immediately applies the plugin. Returns the engine, so
registrations chain (`new Tabnas().use(jsonic).use(Zon, opts)`). The
plugin merges `options` over `Zon.defaults`, installs the embedded ZON
grammar, and re-applies its jsonic option overrides (struct/tuple
tokens, `=` separator, identifier keys, Zig escapes, ZON comments, the
strict Zig number lexer, and the five custom lex matchers).

The instance is reusable and stateless across parses; build it once
and reuse it. Building the grammar dominates a parse, so do not
reconstruct the engine per call.

### `engine.parse(src)`

Parses a ZON source string and returns the resulting JavaScript value.
Objects come back as maps built with `Object.create(null)` (no
prototype); arrays are plain arrays; scalars are `number`, `string`,
`boolean`, or `null` — plus `bigint` for an integer literal too large to
be an exact double. A failed parse throws (see [Errors](#errors)).

## Options

`ZonOptions` has exactly two fields:

```typescript
type ZonOptions = {
  charAsNumber: boolean
  enumTag: null | string
}
```

### `charAsNumber`

- **Type:** `boolean`
- **Default:** `false`
- **Effect:** Controls how Zig character literals (`'x'`, `'\n'`,
  `'\x41'`, `'\u{1F600}'`) are parsed.
  - `false` — the literal becomes a one-character string. `'A'` → `'A'`.
  - `true` — the literal becomes its numeric Unicode code point. `'A'`
    → `65`, `'\n'` → `10`, `'\u{1F600}'` → `128512`.

```js
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Zon } from '@tabnas/zon'

const j = new Tabnas().use(jsonic).use(Zon, { charAsNumber: true })
j.parse("'A'") // => 65
```

### `enumTag`

- **Type:** `null | string`
- **Default:** `null`
- **Effect:** Controls how enum-literal *values* (a bare `.foo` used in
  value position) are represented.
  - `null` — the enum literal becomes the bare identifier string.
    `.red` → `'red'`.
  - a string `T` — the enum literal is wrapped in a one-key object
    `{ [T]: name }`, so it can be distinguished from an ordinary
    string. With `enumTag: '$enum'`, `.red` → `{ $enum: 'red' }`.

The tag affects enum literals only when they are *values*. A `.field`
used as a key (before `=`) is always the plain field name regardless of
`enumTag`.

```js
import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Zon } from '@tabnas/zon'

const j = new Tabnas().use(jsonic).use(Zon, { enumTag: '$enum' })
j.parse('.{ .kind = .red, .label = "red" }') // => { kind: { $enum: 'red' }, label: 'red' }
```

## ZON syntax

ZON is **not** a superset of JSON. It uses Zig anonymous-struct
syntax. The plugin disables the bare `{`, `[`, `]` openers and rebinds
the key/value separator to `=`.

### Structs (maps)

A struct literal opens with `.{`, contains `.field = value` pairs
separated by commas, and closes with `}`. Field names are identifiers
(`[A-Za-z_][A-Za-z0-9_]*`), written with a leading dot that is
stripped from the key. A name that is not a legal identifier is written
`.@"..."` — any string literal, with the same escapes — and a struct may
not repeat a field name.

```
.{ .a = 1, .b = 2 }     => { a: 1, b: 2 }
.{ .a = .{ .b = 1 } }   => { a: { b: 1 } }
.{ .@"a b" = 1 }        => { 'a b': 1 }
.{ .a = 1, .a = 2 }     => error: duplicate struct field name
```

### Tuples (lists)

A tuple literal also opens with `.{`, but contains bare values (no
`.field =`), separated by commas, and closes with `}`. It produces an
array.

```
.{ 1, 2, 3 }            => [1, 2, 3]
.{ "a", "b" }           => ['a', 'b']
.{ .{ 1, 2 }, .{ 3, 4 } } => [[1, 2], [3, 4]]
```

The struct-vs-tuple decision is made at lex time by peeking past the
`.{`: if the next significant token is a field name (`.identifier` or
`.@"..."`) followed by `=`, it is a struct; otherwise it is a tuple.

### Empty literal

An empty `.{}` parses as an **empty array** (`[]`), since with no
contents there is no `.field =` to mark it as a struct.

```
.{}                     => []
```

### Trailing commas

A trailing comma before `}` is allowed in both structs and tuples.

```
.{ .a = 1, }            => { a: 1 }
.{ 1, 2, 3, }           => [1, 2, 3]
```

### Scalars

| Construct | Example | Result |
|---|---|---|
| Integer | `42` | `42` |
| Float | `3.14` | `3.14` |
| Hex | `0x2a` | `42` |
| Octal | `0o52` | `42` |
| Binary | `0b101010` | `42` |
| Hex float | `0x1.8p1` | `3` |
| Exponent | `1e5`, `12_3.0E+77` | `100000`, `1.23e+79` |
| Digit separator | `1_000_000` | `1000000` |
| Infinity / NaN | `inf`, `-inf`, `nan` | `Infinity`, `-Infinity`, `NaN` |
| Big integer | `36893488147419103231` | `36893488147419103231n` (a `bigint`) |
| Boolean | `true`, `false` | `true`, `false` |
| Null | `null` | `null` |
| String | `"hello"` | `'hello'` |
| Enum literal | `.red`, `.@"a b"` | `'red'`, `'a b'` (or `{ tag: ... }`) |
| Char literal | `'A'` | `'A'` (or `65`) |

### Numbers are Zig numbers, not relaxed-JSON numbers

The plugin replaces jsonic's number lexer with one that implements Zig's
literal grammar exactly, so ZON's strictness is preserved:

```
+1      .5      5.      0123      00      -0
1__0    1_      _1      0x_2A     0X2A    0O52
0b12    0o18    1abc    1e        0b1.1   0.1.2
```

are all **rejected**, as the zig compiler rejects them. A leading `-` is a
negation prefix and may be separated by space (`- 1`); `-nan` is not a
literal. An integer whose exact value does not fit an IEEE-754 double is
returned as a `bigint` rather than silently rounded — everything else is a
`number`.

### Strings

Double-quoted strings only (single quotes are reserved for char
literals). Zig-flavoured escapes are recognised: `\n`, `\r`, `\t`,
`\\`, `\"`, `\'`. Unknown escapes are an error.

```
"hello"                 => 'hello'
"a\nb"                  => 'a\nb'
"a\\b"                  => 'a\b'
```

### Multi-line strings

Consecutive lines beginning with `\\` form one string. Each line
contributes its text after the `\\`; lines are joined with `\n`. Zig's
tokenizer lexes the whole run as one token and skips the whitespace
between the lines, so **blank lines inside the run continue the literal**
(contributing an empty line) rather than ending it.

```
\\hello
\\world                 => 'hello\nworld'
```

### Character literals

Single-quoted Zig char literals: a single character, or an escape
`'\n'` `'\r'` `'\t'` `'\\'` `'\''` `'\"'` `'\0'`, a hex escape
`'\xNN'`, or a Unicode escape `'\u{...}'`. By default the result is a
one-character string; with `charAsNumber: true` it is the numeric code
point.

```
'A'                     => 'A'   (or 65 with charAsNumber)
'\n'                    => '\n'  (or 10)
'\u{1F600}'             => '😀'  (or 128512)
```

### Comments

`//` line comments only. They are discarded. `//!` and `///` are Zig
**doc** comments, which ZON rejects; `////` (four or more slashes) is an
ordinary comment again.

```
.{
  // a comment
  .name = "x", // trailing comment
}                       => { name: 'x' }
```

(Jsonic's `#` hash comments and `/* */` block comments are disabled by
the plugin.)

## Tokens

The plugin's lexer produces these tokens (as surfaced in the railroad
diagram legend):

| Token | Source | Meaning |
|---|---|---|
| `#OB` | `.{` | start of a struct (map) |
| `#OS` | `.{` | start of a tuple (list) |
| `#CB` | `}` | close of struct or tuple |
| `#CL` | `=` | key/value separator |
| `#TX` | `.ident`, `.@"..."` | field name (key) or enum literal (value) |
| `VAL` | — | a value: number, string, `true`/`false`/`null`, or `.enum` |

`{`, `[`, and `]` are **not** tokens — they are removed, so a bare `{`
is a syntax error.

## Grammar group tag

Every grammar alternate the plugin adds carries the group tag `zon`.
Callers can switch the ZON alts off (restoring plain jsonic) via
`rule.exclude: 'zon'`:

```typescript
const j = new Tabnas().use(jsonic).use(Zon).options({
  rule: { exclude: 'zon' },
})
```

## Errors

A failed parse throws the engine's standard parse error. It carries
the usual fields — an error `code`, the source location (`row`, `col`,
`pos`), the offending `src` fragment, and a formatted multi-line
`message` with a source-context extract. Inputs that are valid jsonic
but not valid ZON (such as a bare `{` opener) are errors.
