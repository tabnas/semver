/* Copyright (c) 2026 Richard Rodger and other contributors, MIT License */

// The compiled grammar. semver-grammar.abnf is compiled once, at build
// time, by `npm run gen-grammar` (ts/gen-grammar.js), into
// semver-grammar.json at the repository root, and each port carries a
// verbatim copy. These tests hold that arrangement:
//
// - the committed file is what @tabnas/abnf compiles today, so a grammar
//   edit or a compiler release that changes the output fails here until
//   the file is regenerated, rather than shipping a stale grammar;
// - every port's copy is the generated file, byte for byte;
// - installing the plugin loads neither @tabnas/abnf nor @tabnas/bnf.
//   Both are devDependencies now, needed only to generate the file.

import { describe, test } from 'node:test'
import assert from 'node:assert'
import { execFileSync } from 'node:child_process'
import Fs from 'node:fs'
import Path from 'node:path'

const TS = Path.join(__dirname, '..')
const ROOT = Path.join(TS, '..')

// The generator, a plain script beside package.json. Required for its
// compile function: requiring it writes nothing.
const gen = require(Path.join(TS, 'gen-grammar.js')) as {
  compileGrammar: () => string
  COPIES: string[]
  SPEC_FILE: string
}

const committed = Fs.readFileSync(gen.SPEC_FILE, 'utf8')

describe('compiled grammar', () => {
  test('semver-grammar.json is what @tabnas/abnf compiles today', () => {
    assert.ok(
      committed === gen.compileGrammar(),
      'semver-grammar.json is stale: run `npm run gen-grammar` (from ts/) ' +
        'and commit the result',
    )
  })

  test('is pure data: no tree builder and no function reference', () => {
    const spec = JSON.parse(committed)
    assert.deepStrictEqual(Object.keys(spec).sort(), ['meta', 'options', 'rule', 'v'])
    // `toRecognitionSpec` dropped every action: the plugin's one action is
    // added at install, and nothing else may run during a parse.
    for (const builder of ['"a":', '"bo":', '"bc":', '@node$', '@capture$',
      '@bubble$', '@fold$', '@bnf_']) {
      assert.ok(!committed.includes(builder), builder)
    }
    assert.equal(spec.options.rule.start, '__start__')
    // Every character class is eager (gen-grammar.js says why).
    const classes = spec.options.match.token
    assert.deepStrictEqual(Object.keys(classes).sort(), [
      '#RX___U0031__U0039',
      '#RX___U0041__U005A',
      '#RX___U0061__U007A',
    ])
    for (const name of Object.keys(classes)) {
      assert.match(classes[name], /^@~\//, name)
    }
  })

  test('every port embeds the generated file byte for byte', () => {
    assert.deepStrictEqual(gen.COPIES, [
      'ts/src/semver-grammar.json',
      'go/semver-grammar.json',
      'rs/semver-grammar.json',
    ])
    for (const rel of gen.COPIES) {
      const copy = Fs.readFileSync(Path.join(ROOT, ...rel.split('/')), 'utf8')
      assert.ok(
        copy === committed,
        `${rel} differs from semver-grammar.json: run \`npm run gen-grammar\` ` +
          '(from ts/), which writes every copy, and never edit one by hand',
      )
    }
  })

  test('installing the plugin loads neither @tabnas/abnf nor @tabnas/bnf', () => {
    // A fresh process, so that nothing this suite loaded (the compiler,
    // above) can hide in the module cache. It records every request for
    // either package, then installs the plugin, parses, and reports which
    // loaded files belong to either package's directory.
    const script = `
      const Module = require('node:module')
      const Fs = require('node:fs')
      const Path = require('node:path')
      const asked = []
      const load = Module._load
      Module._load = function (request, ...rest) {
        if (/^@tabnas\\/(abnf|bnf)(\\/|$)/.test(request)) asked.push(request)
        return load.call(this, request, ...rest)
      }
      const { Tabnas } = require('@tabnas/parser')
      const { Semver } = require(${JSON.stringify(Path.join(TS, 'dist', 'semver.js'))})
      const value = new Tabnas().use(Semver).parse('1.2.3-rc.1+b.5')
      Module._load = load
      // Each package's own directory, wherever npm or a sibling link put it.
      function home(name, from) {
        try {
          const main = Fs.realpathSync(require.resolve(name, { paths: [from] }))
          let dir = Path.dirname(main)
          while (!Fs.existsSync(Path.join(dir, 'package.json'))) dir = Path.dirname(dir)
          return dir + Path.sep
        } catch (e) { return null }
      }
      const abnf = home('@tabnas/abnf', process.cwd())
      const bnf = home('@tabnas/bnf', abnf || process.cwd())
      const loaded = Object.keys(require.cache).filter((f) =>
        [abnf, bnf].some((dir) => null != dir && f.startsWith(dir)))
      process.stdout.write(JSON.stringify({ value, asked, loaded, found: [abnf, bnf] }))
    `
    const out = JSON.parse(
      execFileSync(process.execPath, ['-e', script], { cwd: TS, encoding: 'utf8' }),
    )
    assert.deepStrictEqual(out.value, {
      major: 1, minor: 2, patch: 3, prerelease: ['rc', 1], build: ['b', '5'],
    })
    assert.deepStrictEqual(out.asked, [])
    assert.deepStrictEqual(out.loaded, [])
    // The check is only worth something if it could have failed: the
    // compiler is installed here (a devDependency), so a stray require
    // would have found it.
    assert.ok(out.found[0], '@tabnas/abnf should be resolvable from ts/')
  })
})
