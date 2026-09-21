# Agents Guide: rs/

The Rust port of the canonical TypeScript in [`../ts`](../ts). Read
[`../AGENTS.md`](../AGENTS.md) first: it holds the cross-runtime rules,
the conformance claim, the grammar's gotchas and the error-code
decision. This file covers only what is specific to this crate.
[`../DIVERGENCE.md`](../DIVERGENCE.md) holds the one place this port's
result differs from the canonical one.

## Layout

| Path | |
|---|---|
| `src/lib.rs` | the whole port: the embedded grammar, `semver`, `plugin`, `make`, `make_with`, `parse`, `format`, `compare`, the value builder and the ECMAScript number rendering |
| `tests/parity_test.rs` | every `../test/spec/*.tsv` fixture through `tabnas_support::Runner`, one shared parser for every row |
| `tests/precedence_test.rs` | `../test/precedence/order.tsv` (every pair, so transitivity too) and `equal.tsv` |
| `tests/oracle_test.rs` | the generated 58,449-string corpus graded against the regular expression semver.org publishes, census and hash pinned |
| `tests/semver_test.rs` | the in-language port of `go/semver_test.go` and `ts/test/semver.test.ts`: the value shape, the exact-integer boundary, `format`, `compare`, the error contract, the lexer switches |
| `tests/divergence_test.rs` | the Rust half of every `../DIVERGENCE.md` row |
| `tests/debug_model_test.rs` | the composition test, mirrored from `ts/test/debug-model.test.ts`: the grammar layered with `tabnas-debug`, and the structured model of the installed rule set |
| `tests/perf_test.rs` | instance reuse beats rebuild-per-parse, a long identifier survives, and the installed grammar carries no tree builders |
| `tests/embed_test.rs` | the embedded grammar equals `../semver-grammar.abnf`, in all three runtimes |
| `tests/version_test.rs` | `Cargo.toml` == `VERSION` == `ts/package.json` == the TypeScript and Go constants |
| `tests/common/mod.rs` | shared helpers: the spec directories, JSON flattening, failure conversion |
| `README.md` | the crate front page, prose-gated; its `rust` fences are doctests of this crate |

Crate `tabnas-semver`, library `tabnas_semver`. The engine (`tabnas`),
the ABNF compiler (`tabnas-abnf`), the fixture runner (`tabnas-support`,
dev only) and the introspection plugin (`tabnas-debug`, dev only) are
**path dependencies on sibling checkouts** (`../../parser/rs`,
`../../abnf/rs`, `../../support/rs`, `../../debug/rs`). `tabnas-bnf` has
no entry, because `tabnas-abnf` depends on it, but `../../bnf/rs` must be
on disk all the same: cargo reads the whole manifest graph before it
compiles anything. None is published, so there is no registry version to
fall back on.

```bash
cargo build --all-targets
cargo test --all-targets && cargo test --doc
cargo clippy --all-targets --all-features -- -D warnings
cargo fmt
```

`make test-rs` from the repository root is the fast loop;
`ci/rust/run.sh` is the full gate and adds `fmt --check`, the lockfile
check and the MSRV pin.

## How the plugin is built

`semver(&mut Tabnas)` mirrors the TypeScript `Semver` function top to
bottom, and the order is load-bearing:

1. **The grammar is compiled** by `abnf_convert` with `start: "semver"`
   and `tag: "semver"`.
2. **The one semantic action is attached** to the compiler's
   end-of-source wrapper, the rule named by `options.rule.start`
   (normally `__start__`), never to `semver`. `semver` closes as soon as
   a version has been read, which for `1.2.3f` happens before the engine
   discovers the trailing `f`; a value built there would describe a
   string about to be rejected. `attach_actions` records a rule-phase
   hook under the engine's own `@<rule>-<phase>` name, so the action is
   registered on the INSTANCE with `state_action_ref` and survives the
   strip below, which drops everything that resolves through
   `spec.refs`.
3. **The tree is thrown away.** `to_recognition_spec` returns the same
   rules, the same tokens and the same accepted language as pure data,
   with every AST-building action and every `node$` / `capture$` /
   `fold$` config key removed. The plugin never reads a tree: the action
   above builds the value from the accepted text. `tests/perf_test.rs`
   asserts the installed document carries none of them.
4. **The options are applied** to that document, each key replaced
   rather than merged, exactly as the TypeScript object spread replaces
   it: every default lexer off, the engine's JSON punctuation tokens
   unbound, `lex.empty` off, and the `unexpected` hint. Only
   `fixed.token` keeps what the compiled grammar put there.

The plugin is guarded by the presence of the `semver` rule rather than
by a decoration, which is how a second install is a no-op without
inventing a key the engine does not already answer.

## The value, and the one divergence

`major`, `minor`, `patch` and a numeric pre-release identifier are a
`Value::Number` up to `MAX_SAFE_INTEGER` (2^53 - 1) and the exact
decimal digits in a `Value::String` beyond it. TypeScript answers a
`bigint` there and Go a `*big.Int`; `tabnas::Value` has one numeric
variant and it is an `f64`, so there is nowhere else exact to put the
digits. `../DIVERGENCE.md` entry 1 records it, with the consequence for
a hand-built value, and `tests/divergence_test.rs` pins both halves.

Two helpers exist because of that representation and must stay in step:

- `compare_number` compares two `Value::Number`s as `f64` and anything
  else as canonical decimal digit strings, so a comparison that crosses
  the boundary is exact. A port that compared `f64`s throughout would
  report `9007199254740992` and `9007199254740993` equal.
- `is_numeric_identifier` reads a `Value::String` of ASCII digits in a
  pre-release list as NUMERIC. That is unambiguous for a parsed value,
  because the specification's `<alphanumeric identifier>` contains at
  least one non-digit by definition.

## Character classes are ASCII, spelled out

The grammar's alphabet is `[0-9A-Za-z.+-]` and nothing else, and every
class test in this crate is written against ASCII bytes:
`is_digits` uses `u8::is_ascii_digit`, never `char::is_numeric`; the
oracle's transcription of the published regular expression writes
`[0-9]` for every `\d`, because the `regex` crate makes `\d` match the
Arabic-Indic and Devanagari digits where the JavaScript and RE2 forms
match ASCII. There is no `trim`, no `is_alphanumeric` and no `\s` in the
crate. A new one has to be spelled out the same way.

Byte offsets are safe for the same reason: every index `from_text` takes
(`find('+')`, `find('-')`, `split('.')`) lands inside text the grammar
has already proven is ASCII, so no slice can split a character. A
hand-built value never reaches those functions, which take a `Value`.

## Numbers rendered as text

`js_number_to_string` is ECMA-262 6.1.6.1.20, copied from `js_number` in
`tabnas-bnf` rather than written again. Rust's own shortest formatter is
not a substitute: it breaks an exact decimal midpoint away from zero
where the specification takes the even digit, and it never switches to
exponent form at 1e21 or 1e-7. Every component this crate parses is a
non-negative integer at most 2^53 - 1, where every formatter agrees, so
the cases the copy covers and Rust's does not arrive only from a value
built by hand. The function was graded against node over 60,000 doubles
on 2026-09-21, chosen to cluster on the midpoints (halves, tenths,
hundredths and thousandths up to 2,000), on the powers of ten from
1e-30 to 1e30, on the boundaries at 1e21 and 1e-7 and on random bit
patterns; every one matched. `tests/divergence_test.rs` keeps the
handful that fail if the call is replaced by `to_string`.

## Cost, and why the test sizes are what they are

A RELEASE build is linear in the length of the input, as the other two
runtimes are. Measured on 2026-09-21, one pre-release identifier of `n`
letters:

| n | Rust release | Node (canonical) |
|---|---|---|
| 1,000 | 9.9 ms | 22.8 ms |
| 10,000 | 165 ms | 168 ms |
| 100,000 | 1.35 s | 1.50 s |
| 500,000 | 7.5 s | not measured |
| 1,000,000 | 14.3 s | not measured |

Nothing aborts, at any of those lengths. The same ladder ran the other
three shapes (a long `MAJOR`, a long build identifier, and a list of
one-letter identifiers) at each size, and every one was accepted up to
and including 1,000,000 characters of each; the list of 1,000,000
identifiers, a 2 MB string, took 48.5 s. The only failure in the ladder
was MEMORY, not stack and not time: a list of 2,000,000 identifiers,
a 4 MB string, was killed by the kernel OOM killer on a 15 GB box that
was also running several cargo builds. Memory is linear in the input
with a large constant, so a caller who must bound it bounds the input
length, which is what the root `AGENTS.md` already says. The
value this grammar builds is two levels deep whatever the input, so the
recursion in `Value::to_json` and in the default `Drop` of a `Value`
cannot be driven by a version string, and the parse loop itself is
iterative over a heap stack. This crate therefore needs no depth cap.

A DEBUG build is quadratic, and the reason is in the engine rather than
here: `Context::sync_rule_stack` maintains a shadow copy of the rule
stack and compares every retained frame against the live rule on every
loop iteration, behind `#[cfg(debug_assertions)]`. This grammar's rule
depth grows with the input, because each `*` repetition compiles to a
per-character helper, so that check is O(depth) per character. Measured
the same day, with the same input shape:

| n | Rust debug |
|---|---|
| 250 | 0.6 s |
| 500 | 1.7 s |
| 1,000 | 7.0 s |
| 2,000 | 37 s |

`cargo test` builds with debug assertions on, so every test in this
crate that feeds a long identifier is sized for that profile, and
`tests/perf_test.rs` says so at each constant. A size the Go suite can
afford (32,000) would take hours here and prove nothing the smaller size
does not. The property those tests exist for is not a wall-clock budget:
it is that a long identifier is ACCEPTED and does not take the process
down, plus the structural check that the installed grammar carries no
tree builders, which is what made the canonical runtimes linear in the
first place.

## Composition with the debug plugin

`tests/debug_model_test.rs` installs `tabnas-debug` beside the grammar
and reads the installed rule set back through its structured model, the
way `ts/test/debug-model.test.ts` does with `@tabnas/debug`. The
canonical test resolves the plugin dynamically and skips when it is
absent; here the plugin is a declared dev-dependency, so the test can
never skip. It asserts the start rule is the compiler's end-of-source
wrapper `__start__`, that `Semver` is in the plugin list, that every
production of the grammar is present as a rule under its own name, the
push edges `__start__` to `semver` to `valid-semver`, the grammar's four
literal tokens and three character classes, that every default lexer
reads back as off, that the live grammar renders to ABNF, and that the
model serialises to JSON and round-trips. The Go suite has no
counterpart, which the root `AGENTS.md` records.

## Fixtures

`test/spec` and `test/precedence` at the repository root are shared with
the other two runtimes and are auto-discovered by listing, in all three.
Adding a `.tsv` runs it everywhere. A row green in one runtime and red
in another is a failure, not a discrepancy; see
[`../test/AGENTS.md`](../test/AGENTS.md) for the column contract, and
change nothing there without running `go test ./...` and `npm test` as
well.
