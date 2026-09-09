/* Copyright (c) 2026 Richard Rodger and other contributors, MIT License */

// Conformance against the specification's own oracle.
//
// semver.org publishes a regular expression that recognises exactly the
// language of its grammar (FAQ: "Is there a suggested regular expression
// (RegEx) to check a SemVer string?"). It is a different formalism from
// the ABNF this plugin compiles, written by different people, and its
// captures name the same five parts the plugin's value carries — which
// makes it the judge here. On every string of the corpus below the
// plugin's VERDICT must equal the expression's, and on every accepted
// string the plugin's VALUE must match the expression's captures and
// `format` must give the input back.
//
// The corpus is generated, not committed, and is IDENTICAL in
// go/oracle_test.go: the same alphabets, the same enumeration order, the
// same xorshift32 stream from the same seed, the same mutation operators
// in the same order. A pinned census (how many strings each section
// accepts) and a pinned FNV-1a hash over the whole corpus make sure both
// runtimes graded the same strings. Changing the corpus means changing
// both constants, in both runtimes, in the same commit.
//
//   exhaustive  the empty string, and EVERY string of length 1..5 over
//               an 8-character alphabet that covers each character class
//               of the grammar and both separators
//   structured  five version-core shapes × every pre-release tail of
//               length 0..3 over "01a." × every build tail of length 0..3
//               over "0a." — pre-release and build identifiers need more
//               than five characters, so exhaustion alone never reaches
//               them
//   mutation    valid versions with one to three random edits (insert,
//               delete, replace, duplicate a slice, append a segment)
//   random      short random strings over a wider alphabet, whitespace,
//               a `v` prefix and `_` included

import { describe, test } from 'node:test'
import assert from 'node:assert'

import { Tabnas } from '@tabnas/parser'
import { Semver, format } from '../dist/semver'
import type { Version } from '../dist/semver'


// semver.org, FAQ — the numbered-capture-group form. Group 1..3 are the
// version core, 4 the pre-release (without the `-`), 5 the build metadata
// (without the `+`).
const ORACLE =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$/


// ---- the corpus (keep byte-identical with go/oracle_test.go) ---------

const SHORT = '019aZ-.+'
const SHORT_MAX = 5
const CORES = ['0.0.0', '1.2.3', '01.0.0', '1.0', '1']
const PRE = '01a.'
const BLD = '0a.'
const TAIL_MAX = 3
const WIDE = '019aZ-.+v _xB250\t/:'
const SEEDS = [
  '0.0.0',
  '1.2.3',
  '1.0.0-alpha',
  '1.0.0-alpha.1',
  '1.0.0-0.3.7',
  '1.0.0-x.7.z.92',
  '1.0.0-x-y-z.--',
  '1.0.0-alpha+001',
  '1.0.0+20130313144700',
  '1.0.0-beta+exp.sha.5114f85',
  '1.0.0+21AF26D3----117B344092BD',
  '10.20.30-rc.1+build.2',
]
const MUTATIONS = 3000
const RANDOMS = 1000
const SEED = 0x9e3779b9

// The pinned census and hash. A change here is a change to the corpus,
// and needs the same change in go/oracle_test.go.
const CENSUS = {
  exhaustive: { total: 37449, accepted: 27 },
  structured: { total: 17000, accepted: 1634 },
  mutation: { total: 3000, accepted: 838 },
  random: { total: 1000, accepted: 0 },
}
const HASH = 0x97bd27cb

// Every string of `alphabet` of exactly `len` characters, in base-n
// counting order (most significant position first).
function* strings(alphabet: string, len: number): Generator<string> {
  const n = alphabet.length
  const total = Math.pow(n, len)
  for (let idx = 0; idx < total; idx++) {
    let s = ''
    let rest = idx
    for (let k = 0; k < len; k++) {
      s = alphabet[rest % n] + s
      rest = Math.floor(rest / n)
    }
    yield s
  }
}

function* upTo(alphabet: string, min: number, max: number): Generator<string> {
  for (let len = min; len <= max; len++) yield* strings(alphabet, len)
}

// xorshift32, the same stream in both runtimes.
class Rng {
  x: number
  constructor(seed: number) {
    this.x = seed >>> 0
  }
  next(): number {
    let x = this.x
    x = (x ^ (x << 13)) >>> 0
    x = (x ^ (x >>> 17)) >>> 0
    x = (x ^ (x << 5)) >>> 0
    this.x = x
    return x
  }
  // In [0, n).
  int(n: number): number {
    return this.next() % n
  }
  pick(alphabet: string): string {
    return alphabet[this.int(alphabet.length)]
  }
}

// One random edit. Every branch consumes the stream in the same order as
// its Go twin, including the branches that do nothing to an empty string.
function mutate(s: string, rng: Rng): string {
  const op = rng.int(5)
  const len = s.length
  if (0 === op) {
    const pos = rng.int(len + 1)
    return s.substring(0, pos) + rng.pick(WIDE) + s.substring(pos)
  }
  if (1 === op) {
    if (0 === len) return s
    const pos = rng.int(len)
    return s.substring(0, pos) + s.substring(pos + 1)
  }
  if (2 === op) {
    if (0 === len) return s
    const pos = rng.int(len)
    return s.substring(0, pos) + rng.pick(WIDE) + s.substring(pos + 1)
  }
  if (3 === op) {
    if (0 === len) return s
    const i = rng.int(len)
    const j = i + 1 + rng.int(len - i)
    const k = rng.int(len + 1)
    return s.substring(0, k) + s.substring(i, j) + s.substring(k)
  }
  const sep = '.-+'[rng.int(3)]
  const n = 1 + rng.int(3)
  let seg = ''
  for (let c = 0; c < n; c++) seg += rng.pick(SHORT)
  return s + sep + seg
}

function corpus(): Record<keyof typeof CENSUS, string[]> {
  const exhaustive = ['', ...upTo(SHORT, 1, SHORT_MAX)]

  const structured: string[] = []
  const pres = [...upTo(PRE, 0, TAIL_MAX)]
  const blds = [...upTo(BLD, 0, TAIL_MAX)]
  for (const core of CORES) {
    for (const x of pres) {
      for (const y of blds) {
        structured.push(core + ('' === x ? '' : '-' + x) + ('' === y ? '' : '+' + y))
      }
    }
  }

  const rng = new Rng(SEED)
  const mutation: string[] = []
  for (let i = 0; i < MUTATIONS; i++) {
    let s = SEEDS[rng.int(SEEDS.length)]
    const edits = 1 + rng.int(3)
    for (let e = 0; e < edits; e++) s = mutate(s, rng)
    mutation.push(s)
  }

  const random: string[] = []
  for (let i = 0; i < RANDOMS; i++) {
    const len = 1 + rng.int(12)
    let s = ''
    for (let c = 0; c < len; c++) s += rng.pick(WIDE)
    random.push(s)
  }

  return { exhaustive, structured, mutation, random }
}

// FNV-1a, 32-bit, over the UTF-8 bytes of every string in corpus order,
// each followed by a newline byte. Every corpus character is ASCII, so
// the bytes are the characters.
function fnv1a(sections: string[][]): number {
  let h = 0x811c9dc5
  for (const section of sections) {
    for (const s of section) {
      for (let i = 0; i < s.length; i++) {
        h = Math.imul(h ^ s.charCodeAt(i), 0x01000193) >>> 0
      }
      h = Math.imul(h ^ 0x0a, 0x01000193) >>> 0
    }
  }
  return h >>> 0
}


// ---- the judge ---------------------------------------------------------

const tn = new Tabnas().use(Semver)

const isDigits = (s: string) => /^[0-9]+$/.test(s)
const text = (n: number | bigint | string) => 'string' === typeof n ? n : String(n)

// Grade one string; return a description of the disagreement, or null.
function grade(s: string): string | null {
  const m = ORACLE.exec(s)
  let value: Version | null = null
  let code: string | null = null
  try {
    value = tn.parse(s)
  } catch (e: any) {
    code = e && e.code
  }

  if (null == m) {
    if (null != value) return `accepted, oracle rejects: ${JSON.stringify(s)}`
    if ('unexpected' !== code) return `rejected with ${code}, not unexpected: ${JSON.stringify(s)}`
    return null
  }

  if (null == value) return `rejected (${code}), oracle accepts: ${JSON.stringify(s)}`
  const v = value as Version
  if (format(v) !== s) return `format gave ${JSON.stringify(format(v))} for ${JSON.stringify(s)}`
  if (text(v.major) !== m[1] || text(v.minor) !== m[2] || text(v.patch) !== m[3]) {
    return `core ${text(v.major)}.${text(v.minor)}.${text(v.patch)} != ${m[1]}.${m[2]}.${m[3]}`
  }
  for (const n of [v.major, v.minor, v.patch]) {
    if ('string' === typeof n) return `core component is a string in ${JSON.stringify(s)}`
  }
  const pre = null == m[4] ? [] : m[4].split('.')
  if (pre.length !== v.prerelease.length) return `prerelease length in ${JSON.stringify(s)}`
  for (let i = 0; i < pre.length; i++) {
    const p = v.prerelease[i]
    if (text(p) !== pre[i]) return `prerelease[${i}] ${text(p)} != ${pre[i]} in ${JSON.stringify(s)}`
    if (isDigits(pre[i]) === ('string' === typeof p)) {
      return `prerelease[${i}] ${pre[i]} has the wrong type in ${JSON.stringify(s)}`
    }
  }
  const bld = null == m[5] ? [] : m[5].split('.')
  assert.deepStrictEqual(v.build, bld, `build in ${JSON.stringify(s)}`)
  return null
}


describe('oracle: the semver.org regular expression', () => {
  const sections = corpus()

  test('the corpus is the one both runtimes grade', () => {
    for (const name of Object.keys(CENSUS) as (keyof typeof CENSUS)[]) {
      assert.equal(sections[name].length, CENSUS[name].total, `${name} size`)
    }
    assert.equal(
      fnv1a([sections.exhaustive, sections.structured, sections.mutation, sections.random]),
      HASH,
      'corpus hash — the generator changed; re-pin HASH and CENSUS here and in go/oracle_test.go',
    )
  })

  for (const name of Object.keys(CENSUS) as (keyof typeof CENSUS)[]) {
    test(`${name}: verdict and value agree with the oracle on every string`, () => {
      const failures: string[] = []
      let accepted = 0
      for (const s of sections[name]) {
        const why = grade(s)
        if (null != why) failures.push(why)
        else if (ORACLE.test(s)) accepted++
      }
      assert.deepStrictEqual(
        failures.slice(0, 20), [],
        `${failures.length} disagreement(s) with the oracle in ${name}`,
      )
      assert.equal(
        accepted, CENSUS[name].accepted,
        `${name} accepted ${accepted}; the census pins ${CENSUS[name].accepted}`,
      )
    })
  }
})
