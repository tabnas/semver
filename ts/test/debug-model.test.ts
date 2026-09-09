/* Copyright (c) 2026 Richard Rodger and other contributors, MIT License */

// Composition test: the semver grammar plugin layered with the official
// @tabnas/debug plugin. @tabnas/debug is a devDependency, but this still
// resolves it dynamically and SKIPS when it is absent so the suite stays
// runnable outside the package; TABNAS_DEBUG_PATH can point at a sibling
// checkout's built plugin.

import { describe, test } from 'node:test'
import assert from 'node:assert'

import { Tabnas } from '@tabnas/parser'
import { Semver } from '../dist/semver'

function loadDebug(): any {
  const candidates = [process.env.TABNAS_DEBUG_PATH, '@tabnas/debug'].filter(
    Boolean,
  ) as string[]
  for (const c of candidates) {
    try {
      return require(c).Debug
    } catch {
      /* try next */
    }
  }
  return null
}

const Debug = loadDebug()
const skip = Debug
  ? false
  : '@tabnas/debug not available (set TABNAS_DEBUG_PATH)'

function build(): any {
  const tn = new Tabnas().use(Semver)
  tn.use(Debug, { print: false, trace: false })
  return tn
}

describe('compose: semver + @tabnas/debug', () => {
  test('parses normally with the debug plugin installed', { skip }, () => {
    const tn = build()
    assert.deepStrictEqual(
      JSON.parse(JSON.stringify(tn.parse('1.2.3-rc.1+sha.abc'))),
      { major: 1, minor: 2, patch: 3, prerelease: ['rc', 1], build: ['sha', 'abc'] },
    )
  })

  test('debug.model() returns the compiled semver grammar', { skip }, () => {
    const tn = build()
    const m = tn.debug.model()

    // The ABNF compiler wraps the start rule in `__start__`, which consumes
    // end-of-source; the user-visible entry is the `semver` alias.
    assert.equal(m.config.start, '__start__')
    assert.ok(
      m.plugins.some((p: any) => p.name === 'Semver'),
      'plugins should list Semver',
    )

    // Every production of the grammar is present as a rule under its own
    // name, alongside the compiler's synthetic helpers.
    const names = new Set(m.rules.map((r: any) => r.name))
    for (const name of [
      '__start__', 'semver', 'valid-semver', 'version-core', 'major', 'minor',
      'patch', 'pre-release', 'build', 'pre-release-identifier',
      'alphanumeric-tail', 'build-identifier', 'numeric-identifier',
      'identifier-character', 'non-digit', 'digit', 'positive-digit', 'letter',
    ]) {
      assert.ok(names.has(name), `rule ${name} should be in the model`)
    }

    // The rule-reference graph: the wrapper pushes the alias, the alias
    // pushes the specification's root.
    const edge = (name: string) => m.graph.find((e: any) => e.name === name)
    assert.deepStrictEqual(edge('__start__').openPush, ['semver'])
    assert.deepStrictEqual(edge('semver').openPush, ['valid-semver'])

    // The lexer carries the grammar's own tokens: the four fixed literals
    // (`0`, `.`, `-`, `+`) and the three character classes. (The engine's
    // default punctuation tins stay registered even though the plugin
    // unbinds their source text, so the model lists them too.)
    const tokenNames = new Set(m.tokens.map((t: any) => t.name))
    for (const name of [
      '#0', '#T', '#T1', '#T2',
      '#RX___U0031__U0039', '#RX___U0041__U005A', '#RX___U0061__U007A',
    ]) {
      assert.ok(tokenNames.has(name), `token ${name} should be in the model`)
    }

    // @tabnas/debug renders the live grammar back to ABNF text.
    assert.equal(typeof m.abnf, 'string')
    assert.match(m.abnf, /valid-semver/)

    // The grammar portion is JSON-serialisable and round-trips.
    const grammar = {
      tokens: m.tokens,
      rules: m.rules,
      graph: m.graph,
      config: m.config,
    }
    assert.deepStrictEqual(JSON.parse(JSON.stringify(grammar)).rules, m.rules)
  })
})
