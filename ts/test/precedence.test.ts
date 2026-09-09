/* Copyright (c) 2026 Richard Rodger and other contributors, MIT License */

// Precedence (specification §11), driven by the shared fixtures in
// `test/precedence/` at the repo root — the same files go/precedence_test.go
// runs, so `compare` cannot drift between the runtimes.
//
//   order.tsv  one version per row, in strictly ascending precedence
//   equal.tsv  pairs that compare equal (build metadata is ignored)
//
// These sit beside, not in, `test/spec/`, whose runner treats every row as
// a parse case.

import { describe, test } from 'node:test'
import assert from 'node:assert'
import { join } from 'node:path'

import { Tabnas } from '@tabnas/parser'
import { findSpecDir, loadSpec } from '@tabnas/support'

import { Semver, compare, format } from '../dist/semver'

const tn = new Tabnas().use(Semver)
const DIR = join(findSpecDir(__dirname), '..', 'precedence')

describe('precedence', () => {
  test('order.tsv: every earlier row ranks below every later row', () => {
    const rows = loadSpec(join(DIR, 'order.tsv')).rows
    assert.ok(20 < rows.length, `order.tsv has ${rows.length} rows; expected a real chain`)
    const versions = rows.map((row) => row.named('version'))
    const parsed = versions.map((v) => tn.parse(v))
    for (let i = 0; i < parsed.length; i++) {
      assert.equal(format(parsed[i]), versions[i], `${versions[i]} round-trips`)
      assert.equal(compare(parsed[i], parsed[i]), 0, `${versions[i]} equals itself`)
      for (let j = i + 1; j < parsed.length; j++) {
        assert.equal(
          compare(parsed[i], parsed[j]), -1,
          `${rows[i].where()}: ${versions[i]} < ${versions[j]}`,
        )
        assert.equal(
          compare(parsed[j], parsed[i]), 1,
          `${rows[j].where()}: ${versions[j]} > ${versions[i]}`,
        )
      }
    }
  })

  test('equal.tsv: every pair has the same precedence', () => {
    const rows = loadSpec(join(DIR, 'equal.tsv')).rows
    assert.ok(5 < rows.length, `equal.tsv has ${rows.length} rows`)
    for (const row of rows) {
      const a = row.named('a')
      const b = row.named('b')
      assert.equal(compare(tn.parse(a), tn.parse(b)), 0, `${row.where()}: ${a} == ${b}`)
      assert.equal(compare(tn.parse(b), tn.parse(a)), 0, `${row.where()}: ${b} == ${a}`)
    }
  })
})
