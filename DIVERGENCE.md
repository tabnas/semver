# Divergences

Where a port produces a different result from the canonical TypeScript
in `ts/`, for the same input. Every row below was MEASURED, on
2026-09-21, by running the three implementations over the input in its
first column; nothing here is inferred from reading the source.

Each entry names who owns the repair. An entry that closes must be
deleted, and the test that pins it fails until it is, so this file
cannot go stale unnoticed.

There is ONE entry. Everything else the three runtimes do with a version
string is identical, and that is not a claim made in prose: the shared
fixtures in `test/spec/*.tsv` (five files) and `test/precedence/*.tsv`
(two files) run row for row in all three suites, and each suite grades
the same generated corpus of 58,449 strings against the regular
expression semver.org publishes, with the same pinned census and the
same pinned FNV-1a hash (`0x97bd27cb`).

## Where the divergence is pinned

There is no executable register under `test/spec` for it. That directory
is auto-discovered by all three parity runners and every row in it is a
parse case compared after a JSON round trip, and the value this entry is
about is exactly the value JSON cannot carry: `JSON.stringify` throws on
a `bigint`. `test/AGENTS.md` records the same limit, which is why the
canonical suites keep these cases in `ts/test/semver.test.ts` and
`go/semver_test.go` rather than in a fixture.

So the entry is pinned by `rs/tests/divergence_test.rs`, which asserts
the Rust half of every row below, and by the mirrored cases in the two
canonical suites, which assert theirs. A port that starts agreeing fails
as loudly as one that starts disagreeing, because the Rust test asserts
the recorded representation rather than merely the recorded value.

## 1. A numeric component above `Number.MAX_SAFE_INTEGER`

The specification places no upper bound on an integer, and a parser that
rounded `9007199254740993.0.0` would report the wrong version. All three
runtimes keep every digit; they carry the digits in different types,
because the engine's Rust `Value` has one numeric variant and it is an
`f64`.

`MAJOR`, `MINOR`, `PATCH` and a numeric pre-release identifier are all
affected. A build identifier is not: it is a string in every runtime.

| input | TypeScript | Go | Rust |
|---|---|---|---|
| `9007199254740991.0.0` major | `9007199254740991` (number) | `9007199254740991` (float64) | `9007199254740991` (`Value::Number`) |
| `9007199254740992.0.0` major | `9007199254740992n` (bigint) | `*big.Int` 9007199254740992 | `"9007199254740992"` (`Value::String`) |
| `99999999999999999999.0.0` major | `99999999999999999999n` | `*big.Int` | `"99999999999999999999"` |
| `1.0.0-alpha.9007199254740993` prerelease[1] | `9007199254740993n` | `*big.Int` | `"9007199254740993"` |
| `1.0.0+9007199254740993` build[0] | `"9007199254740993"` | `"9007199254740993"` | `"9007199254740993"` |

The boundary is the same value in all three: at `2^53 - 1` the component
is a number, and one past it changes representation.

**Reason.** `tabnas::Value` in Rust is `Undefined | Null | Bool | Number(f64)
| String | Array | Object | Text | ...`, with no arbitrary-precision
variant, so there is nowhere for `9007199254740992` to go as a number
without losing a digit. The decimal digits in a `Value::String` are
exact, which the `f64` would not be, and they are what the Rust
`compare` and `format` read. JavaScript has `bigint` and Go has
`math/big`; Rust's engine value type has neither, and adding one is a
change to the engine, not to this plugin.

**Consequence, measured the same day.** Because a string of digits IS a
number to this port, `compare` reads a hand-built digit STRING in a
pre-release list as a NUMERIC identifier, where the canonical runtimes
read any string there as an alphanumeric one. No parse produces such a
value: the grammar turns an all-digit pre-release identifier into a
number below the boundary and into the exact-digits form above it, and
an alphanumeric identifier contains at least one non-digit by
definition. The difference is reachable only from a value assembled by
hand.

| input (two hand-built values) | TypeScript | Go | Rust |
|---|---|---|---|
| `compare(pre ["1"], pre [2])` | `1` | `1` | `Ordering::Less` |
| `compare(pre ["1"], pre ["a"])` | `-1` | `-1` | `Ordering::Less` |
| `compare(pre [1], pre ["a"])` | `-1` | `-1` | `Ordering::Less` |

Only the first row differs. The second and third agree by arithmetic
rather than by accident: `"1"` sorts below `"a"` in ASCII order, and a
numeric identifier ranks below an alphanumeric one, so both readings
reach the same verdict.

**Owner.** The tabnas engine, `github.com/tabnas/parser`. This closes
when `tabnas::Value` grows an exact-integer variant; until then the
representation is the best a port can do, and the VALUE is never wrong.

## Not divergences

Three differences show up when the three runtimes are read side by side
and are not recorded above, because none of them changes a result:

- **The API shape.** `compare` answers a `std::cmp::Ordering` in a
  `Result` rather than `-1 | 0 | 1`; `parse` keeps one shared instance,
  which the canonical runtime leaves to the caller; `SemverOptions` is a
  unit struct where TypeScript has `Record<string, never>`. These are
  the same behaviour in the host language's own terms, and
  `rs/README.md` lists them.
- **An error's row and column at a lookahead failure.** All three
  runtimes reject the same strings with the same `unexpected` code, and
  the three columns agree on every case the suites check
  (`v1.2.3` at column 1, `1.2.3 ` at 6, `1.2.3-a_b` at 8, `1.0.0-01` at
  9, measured the same day). The root `AGENTS.md` records that a column
  at a lookahead failure is where the engine gave up rather than a
  promise, so the agreement is checked but not contracted.
- **Parse cost under `debug_assertions`.** A release build of this crate
  is linear in the length of the input, as the other two runtimes are: a
  1,000 character pre-release identifier parses in 9.9 ms, against
  22.8 ms in Node, and a 1,000,000 character one in 14.3 s with nothing
  aborting. A DEBUG build is quadratic, because the engine's
  `Context::sync_rule_stack` compares every retained stack frame against
  the live rule on every loop iteration behind `#[cfg(debug_assertions)]`,
  and this grammar's rule depth grows with the input. Same answers, same
  language, different cost profile, and `rs/AGENTS.md` records the
  numbers and what they mean for the sizes in `rs/tests/perf_test.rs`.
