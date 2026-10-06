#!/usr/bin/env node

// Compile semver-grammar.abnf into the engine's serialized grammar, at
// build time, so that no runtime loads the ABNF compiler.
//
//   semver-grammar.abnf  --(@tabnas/abnf, here)-->  semver-grammar.json
//
// The output is the engine's portable GrammarSpec document: options and
// rules as plain JSON, with the character classes written as `@~/…/`
// regular expressions and no function in it. Every engine loads that
// form (TS `tn.grammar()`, Go `GrammarSpecFromJSON`, Rust
// `GrammarSpec::from_value`), so ONE compiled file serves all three
// ports. It is written to the repository root, beside the ABNF it comes
// from, and copied verbatim into each port, which cannot read outside its
// own package:
//
//   ts/src/semver-grammar.json   imported by src/semver.ts
//   go/semver-grammar.json       //go:embed in go/semver.go
//   rs/semver-grammar.json       include_str! in rs/src/lib.rs
//
// The compile is the one the plugin used to run at every install:
// `abnfConvert` with the `semver` start rule and tag, then
// `toRecognitionSpec`, which drops every tree-building action the
// compiler emitted (see "The plugin asks for no parse tree" in AGENTS.md).
// The plugin's own options (every default lexer off, the hint) and its
// one action are applied at install, on top of this file, in each port.
//
// Run via: npm run gen-grammar (from ts/), after changing
// semver-grammar.abnf or after an @tabnas/abnf or @tabnas/bnf release
// that changes what they emit. The output is deterministic: the same
// compiler on the same grammar writes the same bytes on every machine.
// ts/test/grammar-spec.test.ts compiles again in memory and fails when
// the committed file is stale; it, go/grammar_spec_test.go and
// rs/tests/embed_test.rs fail when a port's copy differs from it.
//
// Not part of `npm run build`. A build that regenerated the file would
// make the staleness test compare the compiler with itself.

const fs = require('fs')
const path = require('path')

const ROOT = path.join(__dirname, '..')
const GRAMMAR_FILE = path.join(ROOT, 'semver-grammar.abnf')
const SPEC_FILE = path.join(ROOT, 'semver-grammar.json')

// The embedded copies, relative to the repository root. A port that is
// absent in this checkout is skipped, as embed-grammar.js skips it.
const COPIES = [
  'ts/src/semver-grammar.json',
  'go/semver-grammar.json',
  'rs/semver-grammar.json',
]

// The compiled grammar as the text of semver-grammar.json.
function compileGrammar() {
  // Resolved here rather than at the top, so that requiring this file for
  // its constants does not load the compiler.
  const { abnfConvert, toRecognitionSpec, toJsonic } = require('@tabnas/abnf')

  const text = fs.readFileSync(GRAMMAR_FILE, 'utf8')
  const spec = toRecognitionSpec(
    abnfConvert(text, { start: 'semver', tag: 'semver' }))

  // Every character class must be lexable at any lookahead slot. The
  // engine gates match tokens on a per-rule collated column that is
  // path-blind: the `*digit` helper peeks two digits, so at its second
  // slot only a digit is expected, and the letter that ends `01a` or
  // `12a` lexed as a fatal bad token there. Marking a class `eager$` is
  // the opt-out, and it serialises as `@~/…/`. @tabnas/bnf sets it on
  // every class since tabnas/bnf#33 (the Go and Rust emitters always
  // did), so this loop is a no-op on today's compiler; it stays so that
  // the file cannot lose the flag to an older one. Safe for this grammar
  // under either lexer generation because its three classes and four
  // literals are pairwise disjoint: no character can be cut two ways.
  const tokens = (spec.options && spec.options.match &&
    spec.options.match.token) || {}
  for (const name of Object.keys(tokens)) {
    if (tokens[name] instanceof RegExp) tokens[name].eager$ = true
  }

  return toJsonic(spec, { strict: true }) + '\n'
}

function main() {
  const json = compileGrammar()
  fs.writeFileSync(SPEC_FILE, json)
  console.log('Compiled', path.relative(ROOT, GRAMMAR_FILE), 'into',
    path.relative(ROOT, SPEC_FILE))
  for (const rel of COPIES) {
    const file = path.join(ROOT, ...rel.split('/'))
    if (!fs.existsSync(path.dirname(file))) {
      console.log('No port at', path.dirname(rel), '- skipped')
      continue
    }
    fs.writeFileSync(file, json)
    console.log('Copied it to', rel)
  }
}

module.exports = { compileGrammar, COPIES, SPEC_FILE }

if (require.main === module) main()
