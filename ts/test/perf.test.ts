/* Copyright (c) 2026 Richard Rodger and other contributors, MIT License */

// Performance regression guard. Mirrors go/perf_test.go
// (TestParseReusesInstance).
//
// The TS plugin has no convenience parse() entry point: it is a plugin users
// install themselves (new Tabnas().use(Semver)). So there is nothing for the
// module to cache — the regression we can guard is the *usage*: build ONE
// instance and reuse it for many parses, never rebuilding the engine and
// compiling the ABNF per parse. Compiling the grammar dominates a parse by
// orders of magnitude.
//
// The check is machine-INDEPENDENT: it compares reuse against a single parse
// and against the rebuild-per-parse anti-pattern on the SAME machine in the
// SAME run, so a slow CI box cannot make it flaky (everything scales
// together). There is deliberately NO absolute wall-clock budget.

import { test, describe } from 'node:test'
import assert from 'node:assert'

import { Tabnas } from '@tabnas/parser'
import { Semver } from '../dist/semver'

const SRC = '1.2.3-alpha.1+build.5'
const N = 2000

describe('perf', () => {
  test('reusing one instance stays linear and beats rebuild-per-parse', () => {
    // Build the reusable instance once (the expensive step).
    const j = new Tabnas().use(Semver)

    // Warm the reuse path so the comparison is steady-state, and sanity-check
    // the parse result en route.
    for (let i = 0; i < 100; i++) {
      assert.deepEqual(j.parse(SRC), {
        major: 1, minor: 2, patch: 3, prerelease: ['alpha', 1], build: ['build', '5'],
      })
    }

    // Time one isolated (already-warmed) parse on the reused instance.
    let t0 = process.hrtime.bigint()
    j.parse(SRC)
    const single = Number(process.hrtime.bigint() - t0)

    // Time N parses reusing the ONE instance.
    t0 = process.hrtime.bigint()
    for (let i = 0; i < N; i++) {
      j.parse(SRC)
    }
    const reuse = Number(process.hrtime.bigint() - t0)

    // Time N/10 parses that REBUILD a fresh instance every call — the
    // anti-pattern this guards against. A tenth of N, scaled below: each
    // rebuild compiles the grammar, which is ~1000x a parse.
    const M = N / 10
    t0 = process.hrtime.bigint()
    for (let i = 0; i < M; i++) {
      const rj = new Tabnas().use(Semver)
      rj.parse(SRC)
    }
    const rebuild = Number(process.hrtime.bigint() - t0) * (N / M)

    const avgReuse = reuse / N

    // Reuse must be linear: the per-parse average must not blow up past a
    // generous multiple of a single warmed parse (allows GC/JIT jitter).
    assert.ok(
      avgReuse < 50 * single + 1e6,
      `reuse is superlinear: avg ${avgReuse}ns/parse vs single ${single}ns`,
    )

    // And reuse must beat rebuild-per-parse by a wide margin.
    assert.ok(
      reuse * 5 < rebuild,
      `reusing an instance should be far cheaper than rebuilding: ` +
        `reuse=${reuse}ns rebuild=${rebuild}ns (scaled) for ${N} parses`,
    )
  })

  // A long identifier is VALID: the specification bounds neither the
  // length of a pre-release or build identifier nor the number of them,
  // and the regular expression semver.org publishes accepts every string
  // below. They arrive from lock files, tags and HTTP headers, so they
  // are attacker-chosen text (see AGENTS.md, "Untrusted input").
  //
  // While the plugin asked the compiler for the `{rule, src, kids}` tree
  // it never read, each of these was QUADRATIC in the identifier's
  // length — the per-character helper chain re-appended its child's
  // `src` and re-copied its `kids` at every level. `1.0.0-` + 16,000
  // letters took ~10 s and 2.7 GB; 32,000 aborted the V8 heap, killing
  // the process (exit 134), which no `try` can catch. This test is that
  // crash: it does not measure time, it measures survival.
  test('a very long identifier parses instead of killing the process', () => {
    const j = new Tabnas().use(Semver)
    const L = 32000
    for (const [what, src] of [
      ['pre-release identifier', '1.0.0-' + 'a'.repeat(L)],
      ['build identifier', '1.0.0+' + 'a'.repeat(L)],
      ['major', '1'.repeat(L) + '.0.0'],
      ['identifier list', '1.0.0-' + Array(L / 2).fill('a').join('.')],
    ] as [string, string][]) {
      const v: any = j.parse(src)
      assert.equal(v.major !== undefined, true, what)
    }
  })

  // ... and the cost of doing it grows with the input, not with its
  // square. Machine-INDEPENDENT like the test above it: the same
  // instance, the same shape, one length against eight times that
  // length in the same run. Linear is ~8x, quadratic ~64x; the bound is
  // 24x, three times linear, so only a return to quadratic trips it.
  test('identifier length costs linear time, not quadratic', () => {
    const j = new Tabnas().use(Semver)
    const time = (src: string) => {
      j.parse(src) // warm
      const t0 = process.hrtime.bigint()
      j.parse(src)
      return Number(process.hrtime.bigint() - t0)
    }
    const small = time('1.0.0-' + 'a'.repeat(2000))
    const large = time('1.0.0-' + 'a'.repeat(16000))
    assert.ok(
      large < 24 * small,
      `identifier cost is superlinear: 2000 chars ${small}ns, ` +
        `16000 chars ${large}ns (${(large / small).toFixed(1)}x for 8x the input)`,
    )
  })
})
