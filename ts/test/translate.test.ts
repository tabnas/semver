/* Copyright (c) 2021-2026 Richard Rodger, MIT License */

// The translation parts (admin ADR-27): what the manifest names and what
// the package hands a host are the same texts. ../embed-translate.js
// generates src/translate.ts from tabnas.plugin.json and alchemy/*.alc,
// so a part changed without `npm run embed` fails here.

import * as assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import * as path from 'node:path'
import { test } from 'node:test'

import { translate } from '../dist/semver'

const root = path.resolve(__dirname, '..', '..')
const read = (rel: string) => readFileSync(path.join(root, rel), 'utf8')

// The names a library of alchemy definitions defines, in its order.
function definitions (text: string): string[] {
  return text
    .split('\n')
    .filter((line) => line.startsWith('def '))
    .map((line) => line.slice(4).split(/\s/)[0])
}

test('translation parts expose the manifest, the render and the embedding', () => {
  const parts = translate()
  assert.ok(parts)
  assert.equal(parts.manifest, read('tabnas.plugin.json'))
  assert.equal(parts.lift, undefined)
  assert.equal(parts.render?.entry, 'semver-render')
  assert.equal(parts.render?.source, read('alchemy/render.alc'))
  assert.equal(parts.embed?.entry, 'semver-embed')
  assert.equal(parts.embed?.source, read('alchemy/embed.alc'))
})

test('the manifest says what the parts are', () => {
  const spec = JSON.parse(read('tabnas.plugin.json'))
  assert.equal(spec.languageId, 'semver')
  const t = spec.translate
  assert.equal(t.reads, 'tree')
  assert.equal(t.writes, 'tree')
  assert.equal(t.root, 'object')
  assert.equal(t.schema, 'semver')
  assert.equal(t.embed, 'alchemy/embed.alc')
  assert.equal(t.render, 'alchemy/render.alc')
  assert.equal(t.lift, undefined)
  assert.ok(0 < t.loss.length)
  for (const line of t.loss) {
    assert.match(line, /^[A-Z].*\.$/, `${line} is not a sentence`)
  }
})

// A host links the render and the embedding with its own program and
// other formats' parts into one namespace, so every definition is named
// for the format, neither file defines an export, and no name is defined
// in both.
test('the parts are libraries named for the format', () => {
  const parts = translate()
  const render = definitions(parts?.render?.source ?? '')
  const embed = definitions(parts?.embed?.source ?? '')
  assert.ok(render.includes('semver-render'), render.join(' '))
  assert.ok(embed.includes('semver-embed'), embed.join(' '))
  assert.ok(embed.includes('semver-unembed'), embed.join(' '))
  for (const name of [...render, ...embed]) {
    assert.ok(name.startsWith('semver-'), `${name} is not named for semver`)
  }
  for (const name of embed) {
    assert.ok(!render.includes(name), `${name} is defined by the render too`)
  }
})
