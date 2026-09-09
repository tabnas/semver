/* Copyright (c) 2026 Richard Rodger, MIT License */

// The exported VERSION must equal package.json "version".
//
// This is the CI check for version drift. It exists because the constant HAS
// drifted: jsonic-cli shipped 0.4.1 and 0.4.2 while its const sat at 0.4.0,
// and @tabnas/json exported Version = '1.0.0' for several releases because
// nothing ever rewrote it. Both were invisible until someone read the file. A
// release that bumps package.json and forgets the constant now fails here.
//
// `go/version_test.go` checks the Go `const VERSION` against the SAME
// package.json, so the two runtimes cannot drift apart either.

import { describe, test } from 'node:test'
import assert from 'node:assert'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

// At runtime this file is loaded from `dist-test/`, so `..` is the package
// root: the version has to be reachable the way a consumer reaches it, not
// just from `../dist/zon`.
const api = require('..')

// Read package.json rather than importing it, so an unreadable file throws
// here and FAILS the test. A version check that silently does not run is the
// exact failure mode this test exists to prevent, so there is no skip path.
function readPackageJson(): { name: string; version: string } {
  const path = join(__dirname, '..', 'package.json')
  let raw: string
  try {
    raw = readFileSync(path, 'utf8')
  } catch (err) {
    throw new Error(`cannot read ${path}, so VERSION cannot be checked: ${err}`)
  }
  const pkg = JSON.parse(raw)
  if ('string' !== typeof pkg.version || '' === pkg.version) {
    throw new Error(`${path} has no version field, so VERSION cannot be checked`)
  }
  return pkg
}

describe('version', () => {
  test('VERSION matches package.json', () => {
    const pkg = readPackageJson()
    assert.equal(
      api.VERSION,
      pkg.version,
      `VERSION drift: ${pkg.name} exports ${api.VERSION} but package.json is ` +
        `${pkg.version}. Both are rewritten by admin/publish.sh at release; ` +
        `if you bumped one by hand, bump the other.`,
    )
  })

  test('VERSION is exported and looks like a semver', () => {
    assert.equal(typeof api.VERSION, 'string', 'VERSION must be exported as a string')
    assert.match(api.VERSION, /^\d+\.\d+\.\d+/, 'VERSION must be a semver')
  })
})
