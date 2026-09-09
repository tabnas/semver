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
| `ci.yml` | `deps: "parser support bnf abnf debug"` (was `parser support debug json jsonic`) | CI resolves `@tabnas/bnf` and `@tabnas/abnf` from the npm registry instead of sibling checkouts, so the TypeScript side runs against whatever is published — see the toolchain note in [`AGENTS.md`](../AGENTS.md#the-tabnas-engine-dependency). `json` and `jsonic` are cloned for nothing. |
| `clib.yml`, `clib-release.yml` | `libtabnaszon` → `libtabnassemver` | The clib PR gate builds fine either way (it runs `go/clib/build.sh`, which knows its own name); the release lane would publish artifacts under the wrong library name. |
| `release.yml`, `notify-status.yml`, `scorecard.yml` | header comments only (`Target: tabnas/zon` → `tabnas/semver`) | None — the bodies read the package name from `ts/package.json`. |

Diff a staged file against its live twin to see exactly what changes:

```bash
diff .github/workflows/ci.yml ci/workflows/ci.yml
```
