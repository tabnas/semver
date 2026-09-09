/* Copyright (c) 2026 Richard Rodger and other contributors, MIT License */

// Cross-runtime conformance, driven by the shared `test/spec/*.tsv` fixtures
// at the repo root (see ../../test/AGENTS.md).
//
// The fixture loader, the escape codec, the `ERROR:<code>` contract and the
// row loop all come from @tabnas/support, whose Go half `go/parity_test.go`
// uses to run the SAME files — so the two implementations cannot drift
// without one of them going red, and neither can the two loaders.
//
// What is left here is only what is specific to semver: how to build the
// parser. The plugin has no options, so one instance serves every row.

import { Tabnas } from '@tabnas/parser'
import { findSpecDir, makeRunner } from '@tabnas/support'

import { Semver } from '../dist/semver'

const tn = new Tabnas().use(Semver)

makeRunner({
  parse: (input) => tn.parse(input),
})
  // `findSpecDir` walks up from this file — `dist-test/` at runtime — to the
  // repo root's `test/spec`, so moving the suite does not mean recounting
  // `..` hops. `dir` then auto-discovers every fixture in it, so adding a
  // .tsv runs it in both runtimes without touching either runner.
  .dir(findSpecDir(__dirname))
