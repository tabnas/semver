# Agents Guide — shared test data

Two kinds of shared, cross-runtime data live here. All three runtimes read
both, so a change to either affects TypeScript, Go and Rust together — edit
with that in mind.

| Directory | What it is | Who runs it |
|---|---|---|
| `spec/` | Parse fixtures: `input → expected`, one case per line. Every file is auto-discovered and run by `ts/test/parity.test.ts`, `go/parity_test.go` and `rs/tests/parity_test.rs`. | the shared `@tabnas/support` runner, in all three languages |
| `precedence/` | Precedence fixtures for `compare` (specification §11): `order.tsv` is a strictly ascending chain, `equal.tsv` pairs that compare equal. Run by `ts/test/precedence.test.ts`, `go/precedence_test.go` and `rs/tests/precedence_test.rs`. | a dozen lines of loop in each runtime, over the shared loader |

The third instrument is not a file at all: `ts/test/oracle.test.ts`,
`go/oracle_test.go` and `rs/tests/oracle_test.rs` generate an identical
corpus of ~58,000 strings at run time and grade the plugin against the
regular expression semver.org publishes. See
[`AGENTS.md`](../AGENTS.md#conformance-claim) at the repo root for what it
measures and how its census is pinned.

There is no `divergent.tsv` here, deliberately. Every file in `spec/` is
auto-discovered as PARSE cases by all three runners, so a register would
be run as parse fixtures; and the one recorded divergence is a value JSON
cannot carry (see below). It is pinned by `rs/tests/divergence_test.rs`
instead, and [`DIVERGENCE.md`](../DIVERGENCE.md) says so.

## Format

Tab-separated, one case per line, with a header row naming the columns.
Blank lines are skipped, and so are comment lines — a line starting with
`#` that contains no tab. (A data row always has at least one tab.)

### `spec/*.tsv`

| Column | Meaning |
|---|---|
| `input` | The version string. Escapes `\n` `\r` `\t` `\\` are decoded, so a newline or tab inside a case is writable. A leading or trailing blank is significant and is written as itself. An empty input is an empty first cell (the row is just a tab and the expectation). |
| `expected` | A JSON value (the parse result), or `ERROR` / `ERROR:<code>` for inputs that must fail. The code is compared **exactly** — it is the error's code, not a substring of its message. |

`expected` is **not** escape-decoded — it is raw JSON, so JSON's own escape
rules apply. There is no `opts` column: the plugin has no options.

Results are compared after a JSON round-trip, so key order and the object
representation do not affect the comparison. That is also what a fixture
**cannot** express: a component above `Number.MAX_SAFE_INTEGER` (2^53 − 1)
is a `bigint` in TypeScript and a `*big.Int` in Go, which JSON cannot carry.
Those cases live in `ts/test/semver.test.ts`, `go/semver_test.go` and
`rs/tests/semver_test.rs`, mirrored case for case. The Rust port carries
the digits in a string there, which is the single entry in
[`DIVERGENCE.md`](../DIVERGENCE.md).

### `precedence/order.tsv`

One column, `version`. Rows are in strictly ascending precedence. Both
runners check every pair `(i, j)` with `i < j` compares `-1` and the reverse
`1`, and that every row equals itself — so the file pins transitivity, not
only adjacent pairs. Keep it sorted; a row out of order fails in both
runtimes.

### `precedence/equal.tsv`

Two columns, `a` and `b`. Each pair compares `0` both ways.

## Rules

- Prefer adding a fixture here over a one-off in-language assertion when a
  case is expressible as input → output. That is what keeps the two
  runtimes honest against each other.
- **Every rejection is `ERROR:unexpected`.** This plugin declares no error
  codes of its own (see the root `AGENTS.md`), so `unexpected` — the
  engine's base code — is the whole rejection contract, and every row in
  `spec/strict.tsv` pins it as a code, never as a bare `ERROR`. All three
  runtimes compare it exactly.
- A verdict is never a judgement call: the specification's grammar decides,
  and the regular expression semver.org publishes is the oracle the
  `oracle` suites re-check against. If you add a row, check it against the
  expression first; if the two disagree, the fixture is wrong.
- TypeScript is canonical. If the runtimes disagree, the TS behaviour is
  the expected value — unless a port has exposed a genuine TS defect, in
  which case fix TS first and pin the corrected behaviour here.
- A new fixture must pass in ALL THREE runtimes: run `go test ./...` (from
  `go/`), `npm test` (from `ts/`) and `cargo test --all-targets` (from
  `rs/`) before considering it done.
