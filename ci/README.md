# ci/

Staging area for GitHub Actions workflow changes.

This directory exists because session credentials cannot write
`.github/workflows/*` — see admin `DECISIONS.md` ADR-8. To change CI:

1. Put the intended workflow file in `workflows/`.
2. A maintainer promotes it with the admin `rollout/apply-ci-folders.sh`
   script.

## Pending

- **`workflows/rust.yml`** — the Rust gate for `rs/`: formatting, build,
  tests, doctests and clippy with `-D warnings`, plus the lockfile check,
  all of them inside `ci/rust/run.sh` so this file and a contributor's
  local run cannot say different things.

  Standalone rather than an arm of `ci.yml`, because `ci.yml` calls the
  org-shared polyglot workflow and that takes no Rust input: promoting
  this needs no change in `tabnas/.github`. It clones the five sibling
  checkouts the crate resolves by path (`parser`, `abnf`, `bnf`,
  `support`, `debug`) and pins the toolchain to the MSRV in
  `rs/Cargo.toml`. Its `paths:` lists name everything the gate reads,
  the grammar file, the embedder and the shared fixtures included.
  `make test-rs` runs the inner loop locally; `ci/rust/run.sh` runs the
  whole thing.
