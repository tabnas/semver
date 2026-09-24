# ci/

The scripts the CI workflows run. `rust/run.sh` is the Rust gate:
`.github/workflows/rust.yml` runs it, and so can you.

To change CI, edit `.github/workflows/` in a reviewed pull request.
Session credentials push workflow files (admin `DECISIONS.md` ADR-8, as
amended 2026-09-24), so staging a workflow here first for a maintainer
to promote is optional. Sessions still cannot push tags, so a maintainer
pushes any tag that a tag-triggered workflow needs.

Some of the workflows are maintained in admin as well, and an edit made
only in this repository does not last:

- A workflow with a template in admin `rollout/workflows/`, named
  `semver__<file>`, changes in that template too, in a pull request to
  admin. Today that is `release.yml` and `crates-release.yml`. Admin
  `scripts/verify.sh` reports a deployed copy that differs from its
  template, and the next `rollout/apply-workflows.sh --apply` writes the
  template back over it.
- `clib.yml` and `clib-release.yml` are stamped from admin
  `tasks/clib-template/`, together with `go/clib/`. Change the template
  and restamp with admin `tasks/adopt-clib.sh`, which writes both
  workflows straight into `.github/workflows/`. The new stamp lands in
  this repository's own reviewed pull request. Admin `scripts/verify.sh`
  reports a stamped file that differs from its template.

## Promoted

The Rust gate staged here has been promoted and now lives in
`.github/workflows/rust.yml`:

- **`rust.yml`** — the Rust gate for `rs/`: formatting, build,
  tests, doctests and clippy with `-D warnings`, plus the lockfile check,
  all of them inside `ci/rust/run.sh` so this file and a contributor's
  local run cannot say different things.

  Standalone rather than an arm of `ci.yml`, because `ci.yml` calls the
  org-shared polyglot workflow and that takes no Rust input: it needs no
  change in `tabnas/.github`. It clones the five sibling checkouts the
  crate resolves by path (`parser`, `abnf`, `bnf`, `support`, `debug`)
  and pins the toolchain to the MSRV in `rs/Cargo.toml`. Its `paths:`
  lists name everything the gate reads, the grammar file, the embedder
  and the shared fixtures included.
  `make test-rs` runs the inner loop locally; `ci/rust/run.sh` runs the
  whole thing.
