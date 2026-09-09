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
})
