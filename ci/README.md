# Staged CI workflows

The files in `workflows/` are the `.github/workflows/` files this
repository should carry, corrected for `@tabnas/semver`. They are staged
here rather than applied because session credentials cannot write
`.github/workflows/*` (tabnas/admin `DECISIONS.md`, ADR-8); a maintainer
promotes them with `admin/rollout/apply-ci-folders.sh`, or by copying
them over.

Until they are promoted, the live workflows are the scaffold's (ZON's).
What differs, and why it matters:

| File | Change | Effect while not promoted |
|---|---|---|
| `ci.yml` | `deps: "parser support bnf abnf debug"` (was `parser support debug json jsonic`) | CI resolves `@tabnas/bnf` and `@tabnas/abnf` from the npm registry instead of the sibling `main` checkouts, so the TypeScript side runs against whatever is published rather than the fleet's current tip. The suite passes on both — see the toolchain note in [`AGENTS.md`](../AGENTS.md#the-tabnas-engine-dependency). `json` and `jsonic` are cloned for nothing. |
| `clib-release.yml` | `libtabnaszon` → `libtabnassemver` (three `lib:` inputs and a header comment) | The release lane would publish the C library artifacts under the wrong name. `clib.yml`, the PR gate, needs no change at all — it runs `go/clib/build.sh`, which knows its own name — so it is not staged here. |
| `release.yml` | the npm package name is read from `ts/package.json` instead of hardcoded, and the header comments name this repository | **The live file gates and skips publication on `@tabnas/zon`.** It reads the *version* from `ts/package.json` but names the package as a literal in three places, so a release run would ask npm about the wrong package: it would refuse a legitimate repair (the version "is not on npm") and, worse, could skip publishing `@tabnas/semver` because a `@tabnas/zon` of that version exists. Reading the name from the same file the version comes from is what stops a copied workflow drifting again. |
| `notify-status.yml`, `scorecard.yml` | header comments only (`Target: tabnas/zon` → `tabnas/semver`) | None — neither body names the package. |

Diff a staged file against its live twin to see exactly what changes:

```bash
diff .github/workflows/ci.yml ci/workflows/ci.yml
```

## Not a workflow file: CodeQL default setup

The `Code Quality` check runs come from the repository's CodeQL default
setup, configured in the code-security settings rather than in a
workflow. It still lists Python, which the scaffold had (the ZON corpus
tooling under `test/zigzon/tools/`) and this repository no longer does, so
`Analyze (python)` fails with "CodeQL could not process any code written
in Python". A maintainer drops Python from the default setup's languages;
there is no file to stage for it.
