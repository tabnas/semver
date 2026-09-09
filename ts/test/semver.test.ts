/* Copyright (c) 2026 Richard Rodger and other contributors, MIT License */

// The plugin's own surface: the value shape, the number/bigint boundary,
// `format`, `compare`, and the error contract. Everything expressible as
// `input → JSON` lives in the shared fixtures (test/spec/*.tsv, run by
// both runtimes); what is here is what a fixture cannot say — bigint
// values, function results, error details — mirrored case for case in
// go/semver_test.go.

import { describe, test } from 'node:test'
import assert from 'node:assert'

import { Tabnas } from '@tabnas/parser'
import { Semver, compare, format, grammar, VERSION } from '../dist/semver'
import type { Version } from '../dist/semver'

const tn = new Tabnas().use(Semver)
const parse = (src: string): Version => tn.parse(src)

const MAX = Number.MAX_SAFE_INTEGER // 9007199254740991 = 2^53 - 1

describe('semver', () => {
  test('parses the value shape the README promises', () => {
    assert.deepStrictEqual(parse('1.2.3-alpha.1+build.5'), {
      major: 1,
      minor: 2,
      patch: 3,
      prerelease: ['alpha', 1],
      build: ['build', '5'],
    })
    assert.deepStrictEqual(parse('0.0.0'), {
      major: 0,
      minor: 0,
      patch: 0,
      prerelease: [],
      build: [],
    })
  })

  test('the value is a plain object with own, enumerable keys in order', () => {
    const v = parse('1.2.3')
    assert.equal(Object.getPrototypeOf(v), Object.prototype)
    assert.deepStrictEqual(Object.keys(v), ['major', 'minor', 'patch', 'prerelease', 'build'])
  })

  test('numeric pre-release identifiers are numbers, alphanumeric ones strings', () => {
    const v = parse('1.0.0-0.10.a1.1a.01a.-1')
    assert.deepStrictEqual(v.prerelease, [0, 10, 'a1', '1a', '01a', '-1'])
    assert.equal(typeof v.prerelease[0], 'number')
    assert.equal(typeof v.prerelease[2], 'string')
  })

  test('build identifiers are always strings, leading zeros kept', () => {
    assert.deepStrictEqual(parse('1.0.0+001.0.10').build, ['001', '0', '10'])
    assert.equal(typeof parse('1.0.0+1').build[0], 'string')
  })

  describe('integers beyond 2^53 - 1 are bigint, exactly', () => {
    test('the boundary: MAX_SAFE_INTEGER is a number, one more is a bigint', () => {
      const v = parse(`${MAX}.9007199254740992.0`)
      assert.strictEqual(v.major, MAX)
      assert.strictEqual(v.minor, 9007199254740992n)
      assert.strictEqual(v.patch, 0)
    })

    test('a huge major, minor or patch is exact', () => {
      const v = parse('99999999999999999999.0.0')
      assert.strictEqual(v.major, 99999999999999999999n)
      assert.strictEqual(parse('0.340282366920938463463374607431768211456.0').minor,
        340282366920938463463374607431768211456n)
      assert.strictEqual(parse('0.0.18446744073709551616').patch, 18446744073709551616n)
    })

    test('a huge numeric pre-release identifier is exact, and still numeric', () => {
      const v = parse('1.0.0-alpha.9007199254740993')
      assert.deepStrictEqual(v.prerelease, ['alpha', 9007199254740993n])
    })

    test('a huge build identifier stays a string', () => {
      assert.deepStrictEqual(parse('1.0.0+9007199254740993').build, ['9007199254740993'])
    })

    test('format renders a bigint as plain digits', () => {
      for (const s of [
        '99999999254740993.0.0',
        '0.0.9007199254740992',
        '1.0.0-9007199254740993.x',
        `${MAX}.${MAX}.${MAX}-${MAX}+${MAX}`,
      ]) {
        assert.equal(format(parse(s)), s)
      }
    })

    test('compare orders a bigint against a number and a bigint', () => {
      const small = parse('9007199254740991.0.0')
      const big = parse('9007199254740992.0.0')
      const bigger = parse('9007199254740993.0.0')
      assert.equal(compare(small, big), -1)
      assert.equal(compare(big, small), 1)
      assert.equal(compare(big, bigger), -1)
      assert.equal(compare(bigger, bigger), 0)
      assert.equal(compare(parse('1.0.0-9007199254740992'), parse('1.0.0-9007199254740993')), -1)
      assert.equal(compare(parse('1.0.0-9007199254740993'), parse('1.0.0-a')), -1)
    })
  })

  describe('format', () => {
    test('renders every fixture-shaped value back to its input', () => {
      for (const s of [
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
      ]) {
        assert.equal(format(parse(s)), s)
      }
    })

    test('accepts a hand-built value', () => {
      assert.equal(
        format({ major: 1, minor: 2, patch: 3, prerelease: ['rc', 1], build: ['sha', 'abc'] }),
        '1.2.3-rc.1+sha.abc',
      )
      assert.equal(format({ major: 1n, minor: 0, patch: 0, prerelease: [], build: [] }), '1.0.0')
    })
  })

  describe('compare (specification §11)', () => {
    const lt = (a: string, b: string) => {
      assert.equal(compare(parse(a), parse(b)), -1, `${a} < ${b}`)
      assert.equal(compare(parse(b), parse(a)), 1, `${b} > ${a}`)
    }
    const eq = (a: string, b: string) => {
      assert.equal(compare(parse(a), parse(b)), 0, `${a} == ${b}`)
      assert.equal(compare(parse(b), parse(a)), 0, `${b} == ${a}`)
    }

    test('§11.2: major, then minor, then patch, numerically', () => {
      lt('1.0.0', '2.0.0')
      lt('2.0.0', '2.1.0')
      lt('2.1.0', '2.1.1')
      lt('1.9.0', '1.10.0')
      lt('1.10.0', '1.11.0')
      lt('9.0.0', '10.0.0')
    })

    test('§11.3: a pre-release ranks below its normal version', () => {
      lt('1.0.0-alpha', '1.0.0')
      lt('1.0.0-0', '1.0.0')
      lt('1.0.0', '1.0.1-0')
    })

    test('§11.4: the specification\'s own chain', () => {
      const chain = [
        '1.0.0-alpha',
        '1.0.0-alpha.1',
        '1.0.0-alpha.beta',
        '1.0.0-beta',
        '1.0.0-beta.2',
        '1.0.0-beta.11',
        '1.0.0-rc.1',
        '1.0.0',
      ]
      for (let i = 0; i < chain.length; i++) {
        for (let j = i + 1; j < chain.length; j++) lt(chain[i], chain[j])
      }
    })

    test('§11.4.1: numeric identifiers compare numerically, not lexically', () => {
      lt('1.0.0-2', '1.0.0-10')
      lt('1.0.0-rc.9', '1.0.0-rc.10')
    })

    test('§11.4.2: alphanumeric identifiers compare in ASCII order', () => {
      lt('1.0.0-A', '1.0.0-a')
      lt('1.0.0-Z', '1.0.0-a')
      lt('1.0.0--', '1.0.0-0a')
      lt('1.0.0-a', '1.0.0-b')
      lt('1.0.0-a', '1.0.0-aa')
      lt('1.0.0-alpha', '1.0.0-alpha0')
    })

    test('§11.4.3: numeric identifiers rank below alphanumeric ones', () => {
      lt('1.0.0-1', '1.0.0-a')
      lt('1.0.0-9007199254740993', '1.0.0--')
      lt('1.0.0-1', '1.0.0-1a')
    })

    test('§11.4.4: a larger set of equal-prefixed identifiers ranks higher', () => {
      lt('1.0.0-alpha', '1.0.0-alpha.1')
      lt('1.0.0-alpha.1', '1.0.0-alpha.1.0')
    })

    test('§10 / §11.1: build metadata is ignored', () => {
      eq('1.0.0', '1.0.0+build')
      eq('1.0.0+a', '1.0.0+b')
      eq('1.0.0-alpha+1', '1.0.0-alpha+2')
    })

    test('a version equals itself', () => {
      for (const s of ['0.0.0', '1.2.3-rc.1', '1.2.3+x']) eq(s, s)
    })
  })

  describe('errors', () => {
    test('a rejection is the engine\'s `unexpected` code, with position and hint', () => {
      let err: any
      try {
        tn.parse('1.0.0-01')
      } catch (e) {
        err = e
      }
      assert.ok(err, 'expected a throw')
      assert.equal(err.code, 'unexpected')
      assert.equal(err.lineNumber, 1)
      assert.equal(err.columnNumber, 9)
      const json = JSON.parse(JSON.stringify(err))
      assert.equal(json.code, 'unexpected')
      assert.equal(json.status, 'failure')
      assert.match(json.hint, /semver\.org/)
      assert.match(json.hint, /leading zero/)
    })

    test('the empty string is rejected, not answered with undefined', () => {
      assert.throws(() => tn.parse(''), (e: any) => 'unexpected' === e.code)
    })

    test('the error names the character that broke the parse', () => {
      const at = (src: string) => {
        try {
          tn.parse(src)
        } catch (e: any) {
          return { col: e.columnNumber, src: e.internal?.token?.src ?? '' }
        }
        throw new Error(`${src} parsed`)
      }
      assert.deepStrictEqual(at('v1.2.3'), { col: 1, src: 'v' })
      assert.deepStrictEqual(at('1.2.3 '), { col: 6, src: ' ' })
      assert.deepStrictEqual(at('1.2.3-a_b'), { col: 8, src: '_' })
      // A leading zero (`01.2.3`) is rejected by both runtimes, but they
      // point at different characters: TS at the `1` that cannot follow a
      // complete `0`, Go at the `0` whose lookahead never matched. The
      // code is the contract; a position at a lookahead failure is not
      // (see the parser repo's DIVERGENCE.md), so it is not pinned here.
    })
  })

  describe('plugin surface', () => {
    test('the instance is reusable and stateless across parses', () => {
      const a = parse('1.0.0-a')
      const b = parse('2.0.0+b')
      assert.deepStrictEqual(parse('1.0.0-a'), a)
      assert.deepStrictEqual(parse('2.0.0+b'), b)
      assert.throws(() => tn.parse('x'))
      assert.deepStrictEqual(parse('1.0.0-a'), a)
    })

    test('installs on a fresh engine with or without an options object', () => {
      assert.deepStrictEqual(new Tabnas().use(Semver, {}).parse('1.2.3').patch, 3)
      assert.deepStrictEqual(new Tabnas({ plugins: [Semver] }).parse('1.2.3').patch, 3)
    })

    test('has no options yet, and says so', () => {
      assert.deepStrictEqual(Semver.defaults, {})
    })

    test('exports the ABNF grammar text it compiles', () => {
      assert.equal(typeof grammar, 'string')
      assert.match(grammar, /^valid-semver = version-core \[ "-" pre-release \] \[ "\+" build \]$/m)
      assert.match(grammar, /^semver = valid-semver$/m)
      assert.match(grammar, /^positive-digit = %x31-39$/m)
    })

    test('exports a semver-shaped VERSION', () => {
      assert.match(VERSION, /^\d+\.\d+\.\d+$/)
      assert.equal(format(parse(VERSION)), VERSION)
    })
  })
})
