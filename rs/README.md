# tabnas-semver (Rust)

The [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html)
grammar plugin for the [`tabnas`](https://github.com/tabnas/parser)
parsing engine, crate `tabnas_semver`.

A version string becomes a value with the five parts the specification
names, `compare` orders two of them by the specification's precedence
rules (section 11), and `format` renders one back to its string, exactly.

The parser IS the specification's grammar.
[`../semver-grammar.abnf`](../semver-grammar.abnf) at the repository root
is the semver.org grammar transcribed into RFC 5234 ABNF, and
[`tabnas-abnf`](https://github.com/tabnas/abnf) compiles it into the
engine's rule set when the plugin is installed. No code here decides what
a valid version is: the grammar accepts or rejects, and the only code
that runs during a parse is the one action that turns the accepted text
into the value.

This is the Rust port of the canonical TypeScript implementation in
[`../ts`](../ts); the TypeScript version is authoritative and this crate
tracks it. The Go port is in [`../go`](../go). All three embed that one
grammar file.

## Use

```rust
fn main() -> Result<(), Box<dyn std::error::Error>> {
    let value = tabnas_semver::parse("1.2.3-alpha.1+build.5")?;
    assert_eq!(
        value.to_string(),
        r#"{"major":1,"minor":2,"patch":3,"prerelease":["alpha",1],"build":["build","5"]}"#
    );
    Ok(())
}
```

`parse` builds one parser on first use and reuses it. Compiling the ABNF
and installing the rule set costs orders of magnitude more than a parse,
so for anything but a one-off call build an instance once and keep it:

```rust
fn main() -> Result<(), Box<dyn std::error::Error>> {
    let parser = tabnas_semver::make();
    for src in ["1.0.0-alpha", "1.0.0-alpha.1", "1.0.0"] {
        let value = parser.parse(src)?;
        assert_eq!(tabnas_semver::format(&value)?, src);
    }
    Ok(())
}
```

The value carries the distinction the specification's precedence rules
need: a numeric pre-release identifier is a number, an alphanumeric one
is a string, and a build identifier is always a string, so `001` keeps
its zeros.

```rust
fn main() -> Result<(), Box<dyn std::error::Error>> {
    let value = tabnas_semver::parse("1.0.0-0.10.a1.01a+001")?;
    assert_eq!(
        value.to_string(),
        r#"{"major":1,"minor":0,"patch":0,"prerelease":[0,10,"a1","01a"],"build":["001"]}"#
    );
    Ok(())
}
```

`compare` implements section 11. Build metadata takes no part in it, a
pre-release version ranks below the associated normal version, a numeric
identifier ranks below every alphanumeric one, and a larger set of
identifiers ranks above a smaller one it matches so far:

```rust
use std::cmp::Ordering;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let parser = tabnas_semver::make();
    let chain = [
        "1.0.0-alpha",
        "1.0.0-alpha.1",
        "1.0.0-alpha.beta",
        "1.0.0-beta",
        "1.0.0-beta.2",
        "1.0.0-beta.11",
        "1.0.0-rc.1",
        "1.0.0",
    ];
    for pair in chain.windows(2) {
        let low = parser.parse(pair[0])?;
        let high = parser.parse(pair[1])?;
        assert_eq!(tabnas_semver::compare(&low, &high)?, Ordering::Less);
    }

    let plain = parser.parse("1.0.0")?;
    let tagged = parser.parse("1.0.0+20130313144700")?;
    assert_eq!(tabnas_semver::compare(&plain, &tagged)?, Ordering::Equal);
    Ok(())
}
```

Nothing outside the grammar is accepted. Every default lexer is off, so a
blank, a newline, a `v` prefix, a quote or a `#` has no matcher and is
rejected rather than skipped. Every rejection is the engine's base
`unexpected` code: this plugin declares none of its own.

```rust
fn main() -> Result<(), Box<dyn std::error::Error>> {
    let parser = tabnas_semver::make();
    for src in ["v1.2.3", "1.2.3 ", "01.2.3", "1.0.0-01", ""] {
        let error = parser.parse(src).expect_err("outside the grammar");
        assert_eq!(error.code, "unexpected");
    }
    Ok(())
}
```

The plugin also installs on an engine a caller already holds, either
through the engine's plugin mechanism or directly:

```rust
fn main() -> Result<(), Box<dyn std::error::Error>> {
    let mut parser = tabnas::Tabnas::new();
    parser.use_plugin(tabnas_semver::plugin(), None)?;
    assert!(parser.rule_names().iter().any(|name| name == "semver"));

    let mut direct = tabnas::Tabnas::new();
    tabnas_semver::semver(&mut direct)?;
    assert!(direct.parse("1.2.3").is_ok());
    Ok(())
}
```

Parse errors are the engine's `TabnasError`, re-exported as `SemverError`,
with `code`, `row`, `col`, a `hint` that says what a version has to look
like, and a report that shows the offending source. `format` and
`compare` take any `tabnas::Value` and answer a `VersionError` for one
that is not a parsed version.

## Install

Neither the engine nor the ABNF compiler is published to a registry, so
both are consumed as **sibling checkouts**, the standard tabnas
development model. Clone `https://github.com/tabnas/parser`,
`https://github.com/tabnas/abnf` and `https://github.com/tabnas/bnf`
next to this repository and point at them:

```toml
[dependencies]
tabnas-semver = { path = "../semver/rs" }
tabnas = { path = "../parser/rs" }
```

`tabnas-bnf` needs no entry of its own, because it is `tabnas-abnf` that
depends on it, but cargo reads the whole manifest graph before it
compiles anything, so the checkout has to be on disk. The `tabnas` entry
is there because a crate's dependencies are not passed on to its
dependents: `tabnas_semver` alone does not put `tabnas::Tabnas` or
`tabnas::Value` in scope, and the examples above that name them would not
resolve. Only `SemverError` is re-exported. The test suite additionally
needs `https://github.com/tabnas/support` beside the repository, for the
shared fixture runner.

## Differences from the canonical TypeScript

Every parse result is the TypeScript one, with the single exception
below: the shared fixtures in [`../test/spec`](../test/spec) and
[`../test/precedence`](../test/precedence), and a generated corpus of
58,449 strings graded against the regular expression semver.org
publishes, hold all three runtimes to it.
[`../DIVERGENCE.md`](../DIVERGENCE.md) records what differs and pins it.

- **An integer above 2^53 is exact decimal digits, in a string.** The
  engine's `Value` has one numeric variant and it is an `f64`, so a
  component past `MAX_SAFE_INTEGER` cannot be a number without losing
  digits. TypeScript answers a `bigint` there and Go a `*big.Int`. The
  VALUE is the same in all three and the digits are exact; the
  representation is not, and `compare` and `format` read both forms.
- **`SemverOptions` is a unit struct.** The plugin has no options in any
  runtime. The type exists so that adding one is not a breaking change.
- **`parse` is a convenience the canonical runtime lacks.** It keeps one
  default instance behind a `OnceLock`, which is the reuse the TypeScript
  suite tells a caller to arrange by hand and the Go port already
  provides. Parsing reads instance state and builds a fresh context per
  call, so the shared instance is safe for concurrent use without the
  mutex the Go convenience needs.
- **`compare` answers an `Ordering`** rather than the canonical
  `-1 | 0 | 1`, and returns it in a `Result` so a value that is not a
  parsed version is an error rather than a silent verdict.

## Build and test

The engine, the ABNF compiler and the fixture runner are path
dependencies on sibling checkouts, so there is nothing to fetch:

```bash
cargo test --all-targets && cargo test --doc
```

Or, from the repository root, `make test-rs`. For what CI would say,
including formatting, clippy and the lockfile check, run
`ci/rust/run.sh`.

The suite runs every shared `../test/spec/*.tsv` fixture through the
shared runner and both `../test/precedence/*.tsv` chains, and grades the
generated corpus against the published regular expression with its census
and hash pinned in all three runtimes. Beside them are the in-language
tests: the value shape, the exact-integer boundary, `format`, `compare`
over the specification's own examples, the error contract, instance
reuse, the version sites, and the embedded grammar against the file on
disk.

Every example in this file is compiled and run as a doctest, so a stale
one fails the build.

## License

MIT.
