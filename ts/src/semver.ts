/* Copyright (c) 2026 Richard Rodger and other contributors, MIT License */

/*  semver.ts
 *  @tabnas/semver — Semantic Versioning 2.0.0 (https://semver.org) for the
 *  tabnas engine.
 *
 *  The parser IS the specification's grammar: `semver-grammar.abnf` at the
 *  repository root (embedded below) is the semver.org BNF transcribed into
 *  RFC 5234 ABNF, and @tabnas/abnf compiles it into the engine's rule set
 *  when the plugin is installed. Nothing here decides what a valid version
 *  is — the grammar accepts or rejects — and the only code that runs
 *  during a parse is the one action that turns the accepted text into the
 *  `Version` value.
 *
 *    new Tabnas().use(Semver).parse('1.2.3-alpha.1+build.5')
 *    // => { major: 1, minor: 2, patch: 3,
 *    //      prerelease: ['alpha', 1], build: ['build', '5'] }
 *
 *  `compare` implements the specification's precedence rules (§11) over
 *  two parsed values, and `format` renders a value back to its string.
 */

import type { Tabnas, Plugin, Rule, Context } from '@tabnas/parser'
import { abnfConvert, attachActions, toRecognitionSpec } from '@tabnas/abnf'


// The plugin has no options yet. The type exists so `tn.use(Semver, {})`
// is typed and a future option has a declared home.
type SemverOptions = Record<string, never>

// A numeric component: MAJOR, MINOR, PATCH, or a numeric pre-release
// identifier. A `number` when the value is at most Number.MAX_SAFE_INTEGER
// (2^53 - 1), so it is exact and arithmetic-safe; a `bigint` beyond that,
// because the specification places no upper bound on an integer and a
// parser that silently rounded `9007199254740993.0.0` would report the
// wrong version.
type SemverNumber = number | bigint

// A pre-release identifier: numeric ones (`1` in `alpha.1`) are numbers,
// because the specification compares them numerically and ranks every
// numeric identifier below every alphanumeric one; the rest are strings.
type PrereleaseIdentifier = string | SemverNumber

// The parse result.
type Version = {
  major: SemverNumber
  minor: SemverNumber
  patch: SemverNumber
  // Empty when the version has no pre-release part.
  prerelease: PrereleaseIdentifier[]
  // Empty when the version has no build metadata. Always strings: build
  // identifiers may carry leading zeros (`001`) and take no part in
  // precedence, so a number would lose information for nothing.
  build: string[]
}


// --- BEGIN EMBEDDED semver-grammar.abnf ---
const grammarText = `
; Semantic Versioning 2.0.0 — the grammar of a valid version string.
;
;   https://semver.org/spec/v2.0.0.html
;   (section "Backus–Naur Form Grammar for Valid SemVer Versions")
;
; This file is the single source of truth for @tabnas/semver. It is RFC
; 5234 ABNF, compiled by @tabnas/abnf into a tabnas grammar at plugin
; install time, in both runtimes (ts/src/semver.ts and go/semver.go embed
; it verbatim; "npm run embed" copies it there — never edit the copies).
;
; Every production keeps the name the specification gives it, with the
; specification's spaces written as hyphens ("<version core>" is
; "version-core"), and — with two exceptions explained below — the
; specification's shape. The language accepted is EXACTLY the language of
; the specification's grammar: the two rewrites are equivalences, not
; approximations, and the conformance suite in both runtimes checks the
; result against the regular expression semver.org publishes, on every
; string of a short alphabet up to length five and on thousands of
; mutated versions.
;
; The engine sees one character per token: the plugin switches every
; default lexer (whitespace, line ends, comments, strings, numbers, bare
; words) off, so nothing outside this grammar can be consumed, and a
; blank, a tab, a newline or a "v" prefix is rejected like any other
; character the grammar does not name.

; The entry point. A pure alias of the specification's root production;
; it exists so the plugin has one unhyphenated rule name to hang its
; value-building action on (see ts/src/semver.ts, "@semver:ac").
semver = valid-semver

; <valid semver> ::= <version core>
;                  | <version core> "-" <pre-release>
;                  | <version core> "+" <build>
;                  | <version core> "-" <pre-release> "+" <build>
valid-semver = version-core [ "-" pre-release ] [ "+" build ]

; <version core> ::= <major> "." <minor> "." <patch>
version-core = major "." minor "." patch

major = numeric-identifier
minor = numeric-identifier
patch = numeric-identifier

; <pre-release> ::= <dot-separated pre-release identifiers>
; <dot-separated pre-release identifiers> ::= <pre-release identifier>
;   | <pre-release identifier> "." <dot-separated pre-release identifiers>
pre-release = pre-release-identifier *( "." pre-release-identifier )

; <build> ::= <dot-separated build identifiers>
; <dot-separated build identifiers> ::= <build identifier>
;   | <build identifier> "." <dot-separated build identifiers>
build = build-identifier *( "." build-identifier )

; <pre-release identifier> ::= <alphanumeric identifier>
;                            | <numeric identifier>
;
; REWRITE 1 of 2. Both alternatives can begin with a digit ("1" is
; numeric, "1a" is alphanumeric, "01a" is alphanumeric, "01" is nothing),
; and the decision may need every character of the identifier, so the
; specification's shape is not LL(1). It is factored on the first
; character instead:
;
;   "0" alone is the numeric identifier zero; "0" followed by more digits
;   is valid only if a non-digit eventually appears (an alphanumeric
;   identifier such as "007a" — leading zeros are fine there);
;
;   a positive digit starts a numeric identifier, which turns into an
;   alphanumeric one if a non-digit appears after the digits;
;
;   anything else must be a non-digit, and starts an alphanumeric
;   identifier.
;
; The union of the three is exactly <alphanumeric identifier> ∪ <numeric
; identifier>; what it excludes is exactly a digit string with a leading
; zero, which the specification also excludes.
pre-release-identifier = "0" [ *digit alphanumeric-tail ]
                       / positive-digit *digit [ alphanumeric-tail ]
                       / alphanumeric-tail

; <alphanumeric identifier> ::= <non-digit>
;                             | <non-digit> <identifier characters>
;                             | <identifier characters> <non-digit>
;                             | <identifier characters> <non-digit> <identifier characters>
;
; i.e. a non-empty string of identifier characters containing at least one
; non-digit. Split at its FIRST non-digit, such a string is
;
;   *digit alphanumeric-tail
;
; and "alphanumeric-tail" is the part from that first non-digit on. It is
; the form the factored pre-release-identifier above consumes after it
; has already read the leading digits.
alphanumeric-tail = non-digit *identifier-character

; <build identifier> ::= <alphanumeric identifier>
;                      | <digits>
;
; REWRITE 2 of 2. An alphanumeric identifier is a non-empty identifier
; string with a non-digit in it; <digits> is a non-empty identifier
; string with no non-digit in it. Their union is every non-empty
; identifier string, leading zeros included ("001" is a valid build
; identifier).
build-identifier = 1*identifier-character

; <numeric identifier> ::= "0"
;                        | <positive digit>
;                        | <positive digit> <digits>
numeric-identifier = "0" / positive-digit *digit

; <identifier character> ::= <digit>
;                          | <non-digit>
identifier-character = digit / non-digit

; <non-digit> ::= <letter>
;               | "-"
non-digit = letter / "-"

; <digit> ::= "0"
;           | <positive digit>
digit = "0" / positive-digit

; <positive digit> ::= "1" | "2" | "3" | "4" | "5" | "6" | "7" | "8" | "9"
positive-digit = %x31-39

; <letter> ::= "A" | "B" | ... | "Z" | "a" | "b" | ... | "z"
letter = %x41-5A / %x61-7A
`
// --- END EMBEDDED semver-grammar.abnf ---


// Plugin implementation.
const Semver: Plugin = (tn: Tabnas, _options: SemverOptions) => {
  // Compile the specification's grammar into an engine rule set. The
  // start rule is `semver`, a pure alias of the specification's
  // `valid-semver`: the name has no hyphen in it, which used to be what
  // let a lifecycle hook bind on the published engine, whose TypeScript
  // half derived a phase from `@<rule>-<phase>` by splitting at the
  // first hyphen. The hook has since moved to the compiler's
  // end-of-source wrapper (below), so nothing depends on that any more,
  // but the alias stays: it is the name this plugin's grammar, fixtures
  // and diagnostics all use for the entry production.
  //
  // Then throw the tree away. `toRecognitionSpec` strips every
  // AST-building action the compiler emitted (the `a:` refs into
  // `spec.ref`) and returns the same rules with the same language.
  // Building the `{rule, src, kids}` tree is QUADRATIC in an
  // identifier's length here: each `*`/`1*` repetition compiles to a
  // per-character helper that re-appends its child's `src` and re-copies
  // its `kids` at every nesting level, so `1.0.0-` + 16,000 letters cost
  // ~10 s and 2.7 GB in TS, and 32,000 aborted the process. The plugin
  // never reads that tree — the action below builds the value from the
  // accepted text — so nothing is lost, and the same input now parses in
  // ~0.16 s and ~60 MB.
  const spec = toRecognitionSpec(
    abnfConvert(grammarText, { start: 'semver', tag: 'semver' }))

  // Every character class must be lexable at any lookahead slot. The
  // engine gates match tokens on a per-rule collated column that is
  // path-blind: the `*digit` helper peeks two digits, so at its second
  // slot only a digit is expected, and the letter that ends `01a` or `12a`
  // lexed as a fatal bad token there. Marking a class `eager$` is the
  // opt-out; @tabnas/bnf does it for every class since tabnas/bnf#33 (the
  // Go emitter always did), and this loop is that change ported here so
  // the plugin is correct on the published compiler too — it is a no-op
  // once the emitter already set the flag. Safe for this grammar under
  // either lexer generation because its three classes and four literals
  // are pairwise disjoint: no character can be cut two ways.
  const tokens: Record<string, RegExp & { eager$?: boolean }> =
    (spec.options && spec.options.match && spec.options.match.token) || {}
  for (const name of Object.keys(tokens)) {
    if (tokens[name] instanceof RegExp) tokens[name].eager$ = true
  }

  // The single semantic action, on the compiler's end-of-source wrapper
  // — the rule `abnfConvert` names in `options.rule.start`, normally
  // `__start__` (it numbers the name only if the grammar declares one
  // itself, which this one does not). That rule closes on `#ZZ` and
  // nothing else, so it closes exactly when the whole source has been
  // accepted: `ctx.src()` is then the accepted text, character for
  // character — with every default lexer off (below) nothing was
  // skipped on the way in — and it is the same string the discarded
  // tree's `src` used to hold. Its node is what `parse` returns.
  //
  // The wrapper, not `semver`: `semver` closes as soon as a VERSION has
  // been read, which for `1.2.3f` happens before the engine discovers
  // the trailing `f`, and `ctx.src()` there is the whole input, `f` and
  // all. Building a value from it would report a wrong version (or, for
  // `1.2.3f`, throw a raw `SyntaxError` from `BigInt('3f')`) on a
  // string the grammar is about to reject. Positions and error codes
  // are unaffected either way: this action only runs on success.
  const startRule = (spec.options as any).rule.start
  attachActions(spec, {
    [`@${startRule}:ac`]: (r: Rule, ctx: Context) => {
      r.node = fromText(ctx.src())
    },
  })

  // The compiled spec brings its own tokens (`.`, `-`, `+`, `0` and the
  // three character classes); everything else the engine lexes by default
  // is switched off, so a character the grammar does not name — a blank,
  // a newline, a `v` prefix, a quote, a `#` — has no matcher and is
  // rejected as `unexpected` rather than skipped as whitespace or a
  // comment. The engine's default punctuation tokens go too: they are
  // JSON's, not semver's, and would otherwise show up in diagnostics and
  // introspection as tokens of this grammar.
  spec.options = {
    ...spec.options,
    fixed: {
      token: {
        ...((spec.options && spec.options.fixed && spec.options.fixed.token) || {}),
        '#OB': null,
        '#CB': null,
        '#OS': null,
        '#CS': null,
        '#CL': null,
        '#CA': null,
      },
    },
    space: { lex: false },
    line: { lex: false },
    comment: { lex: false },
    string: { lex: false },
    number: { lex: false },
    text: { lex: false },
    value: { lex: false },
    // The empty string is not a version. By default the engine answers an
    // empty source with `undefined` before any rule runs.
    lex: { empty: false },
    // Every rejection is the engine's base `unexpected` code (this plugin
    // declares no codes of its own — see AGENTS.md); the hint is where a
    // reader learns what a version has to look like.
    hint: {
      unexpected: `
The character(s) {src} do not match any rule alternative active at
this position.

A Semantic Version is MAJOR.MINOR.PATCH — three integers with no
leading zeros — optionally followed by -PRERELEASE and +BUILD, each a
dot-separated list of non-empty identifiers made of [0-9A-Za-z-],
where a numeric pre-release identifier has no leading zero. Nothing
else is allowed: no whitespace, no "v" prefix, no empty identifier.
See https://semver.org/spec/v2.0.0.html`,
    },
  }

  tn.grammar(spec)
}


// Build the value from accepted text. The grammar has already proven the
// text well-formed, so every split below is total and unambiguous:
// version-core contains no `-`, so the first `-` (before any `+`) opens
// the pre-release; no identifier contains `+`, so the first `+` opens the
// build metadata; and `.` separates identifiers, which never contain it.
function fromText(text: string): Version {
  let core = text
  let build: string[] = []
  const plus = core.indexOf('+')
  if (-1 !== plus) {
    build = core.substring(plus + 1).split('.')
    core = core.substring(0, plus)
  }
  let prerelease: PrereleaseIdentifier[] = []
  const dash = core.indexOf('-')
  if (-1 !== dash) {
    prerelease = core.substring(dash + 1).split('.').map(identifier)
    core = core.substring(0, dash)
  }
  const parts = core.split('.')
  return {
    major: integer(parts[0]),
    minor: integer(parts[1]),
    patch: integer(parts[2]),
    prerelease,
    build,
  }
}

// A pre-release identifier: numeric when it is all digits (the grammar has
// already excluded a leading zero there), a string otherwise.
function identifier(text: string): PrereleaseIdentifier {
  return isDigits(text) ? integer(text) : text
}

function isDigits(text: string): boolean {
  for (let i = 0; i < text.length; i++) {
    const c = text.charCodeAt(i)
    if (c < 48 || 57 < c) return false
  }
  return 0 < text.length
}

// A digit string as an exact integer: a `number` up to
// Number.MAX_SAFE_INTEGER, a `bigint` beyond.
function integer(digits: string): SemverNumber {
  const num = Number(digits)
  return Number.isSafeInteger(num) ? num : BigInt(digits)
}


// Render a value back to its version string. For a value that came out of
// the parser this is the exact input text: `format(tn.parse(s)) === s`.
function format(v: Version): string {
  let out = numberText(v.major) + '.' + numberText(v.minor) + '.' + numberText(v.patch)
  if (0 < v.prerelease.length) {
    out += '-' + v.prerelease.map((p) => 'string' === typeof p ? p : numberText(p)).join('.')
  }
  if (0 < v.build.length) {
    out += '+' + v.build.join('.')
  }
  return out
}

function numberText(n: SemverNumber): string {
  // A `number` here is a safe integer, so String() is plain digits; a
  // bigint's toString() is plain digits too, with no `n` suffix.
  return 'bigint' === typeof n ? n.toString() : String(n)
}


// Precedence, as the specification defines it (§11): -1 when `a` ranks
// below `b`, 1 when above, 0 when they are the same version. Build
// metadata is ignored (§10, §11.1): `1.0.0+a` and `1.0.0+b` compare equal.
function compare(a: Version, b: Version): -1 | 0 | 1 {
  return (
    compareNumber(a.major, b.major) ||
    compareNumber(a.minor, b.minor) ||
    compareNumber(a.patch, b.patch) ||
    comparePrerelease(a.prerelease, b.prerelease)
  )
}

// §11.2: numerically. `<` and `>` compare a number and a bigint correctly.
function compareNumber(x: SemverNumber, y: SemverNumber): -1 | 0 | 1 {
  return x < y ? -1 : x > y ? 1 : 0
}

// §11.3: a pre-release version ranks below the associated normal version.
// §11.4: otherwise identifier by identifier, left to right, and a larger
// set of identifiers ranks above a smaller one when every preceding
// identifier is equal.
function comparePrerelease(
  x: PrereleaseIdentifier[],
  y: PrereleaseIdentifier[],
): -1 | 0 | 1 {
  if (0 === x.length && 0 === y.length) return 0
  if (0 === x.length) return 1
  if (0 === y.length) return -1
  const n = Math.min(x.length, y.length)
  for (let i = 0; i < n; i++) {
    const c = compareIdentifier(x[i], y[i])
    if (0 !== c) return c
  }
  return compareNumber(x.length, y.length)
}

// §11.4.1: numeric identifiers numerically. §11.4.2: alphanumeric
// identifiers lexically in ASCII sort order (a JavaScript string
// comparison, since identifiers are ASCII). §11.4.3: a numeric identifier
// always ranks below an alphanumeric one.
function compareIdentifier(
  p: PrereleaseIdentifier,
  q: PrereleaseIdentifier,
): -1 | 0 | 1 {
  const pn = 'string' !== typeof p
  const qn = 'string' !== typeof q
  if (pn && qn) return compareNumber(p as SemverNumber, q as SemverNumber)
  if (pn) return -1
  if (qn) return 1
  return p < q ? -1 : p > q ? 1 : 0
}


// Default option values (there are none yet).
Semver.defaults = {} as SemverOptions

// The grammar, as ABNF text — the same text the plugin compiles.
const grammar: string = grammarText

// VERSION is this package's version. It MUST equal package.json "version":
// the release orchestrator rewrites both, and the version test fails the
// build if they drift. Mirrors `const VERSION` in go/semver.go.
const VERSION = '0.1.2'

export { Semver, compare, format, grammar, VERSION }
export type { SemverOptions, Version, PrereleaseIdentifier, SemverNumber }
