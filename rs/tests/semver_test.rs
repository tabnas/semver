// The plugin's own surface: the value shape, the exact-integer boundary,
// `format`, `compare`, and the error contract. Everything expressible as
// `input -> JSON` lives in the shared fixtures (`test/spec/*.tsv`, run by
// all three runtimes); what is here is what a fixture cannot say --
// integers above 2^53, function results, error details -- mirrored case
// for case with `ts/test/semver.test.ts` and `go/semver_test.go`.

mod common;

use std::cmp::Ordering;

use tabnas::{Tabnas, Value};
use tabnas_semver::{compare, format, make, parse, GRAMMAR, MAX_SAFE_INTEGER, VERSION};

use common::parse_fresh;

fn parser() -> Tabnas {
    make()
}

fn must_parse(parser: &Tabnas, src: &str) -> Value {
    parser
        .parse(src)
        .unwrap_or_else(|error| panic!("parse({src:?}): {error}"))
}

fn number(n: u64) -> Value {
    Value::Number(n as f64)
}

/// The expected shape, built the way the parser builds it.
fn version(major: Value, minor: Value, patch: Value, pre: &[Value], build: &[&str]) -> Value {
    let mut fields = indexmap::IndexMap::new();
    fields.insert("major".to_string(), major);
    fields.insert("minor".to_string(), minor);
    fields.insert("patch".to_string(), patch);
    fields.insert("prerelease".to_string(), Value::array(pre.to_vec()));
    fields.insert(
        "build".to_string(),
        Value::array(build.iter().map(|b| Value::String(b.to_string())).collect()),
    );
    Value::object(fields)
}

fn field(value: &Value, key: &str) -> Value {
    match value {
        Value::Object(fields) => fields.get(key).cloned().expect("the field is present"),
        other => panic!("not a version value: {other:?}"),
    }
}

fn items(value: &Value, key: &str) -> Vec<Value> {
    match field(value, key) {
        Value::Array(values) => values.to_vec(),
        other => panic!("{key} is not a list: {other:?}"),
    }
}

fn text(values: &[&str]) -> Vec<Value> {
    values
        .iter()
        .map(|v| Value::String(v.to_string()))
        .collect()
}

#[test]
fn value_shape() {
    let parser = parser();
    assert_eq!(
        must_parse(&parser, "1.2.3-alpha.1+build.5"),
        version(
            number(1),
            number(2),
            number(3),
            &[Value::String("alpha".into()), number(1)],
            &["build", "5"]
        )
    );
    assert_eq!(
        must_parse(&parser, "0.0.0"),
        version(number(0), number(0), number(0), &[], &[])
    );
}

#[test]
fn prerelease_identifier_types() {
    let parser = parser();
    let got = must_parse(&parser, "1.0.0-0.10.a1.1a.01a.-1");
    assert_eq!(
        items(&got, "prerelease"),
        vec![
            number(0),
            number(10),
            Value::String("a1".into()),
            Value::String("1a".into()),
            Value::String("01a".into()),
            Value::String("-1".into()),
        ]
    );
}

#[test]
fn build_identifiers_are_strings() {
    let parser = parser();
    let got = must_parse(&parser, "1.0.0+001.0.10");
    assert_eq!(items(&got, "build"), text(&["001", "0", "10"]));
}

// A component at MAX_SAFE_INTEGER is a number; one past it is the exact
// decimal digits as a string, because the engine's `Value` has one
// numeric variant and it is an `f64`. TypeScript answers a `bigint` here
// and Go a `*big.Int`; the VALUE is the same in all three, the
// representation is not. See DIVERGENCE.md, entry 1.
#[test]
fn exact_integer_boundary() {
    let parser = parser();
    let got = must_parse(&parser, "9007199254740991.9007199254740992.0");
    assert_eq!(field(&got, "major"), Value::Number(MAX_SAFE_INTEGER as f64));
    assert_eq!(
        field(&got, "minor"),
        Value::String("9007199254740992".into())
    );
    assert_eq!(field(&got, "patch"), number(0));
}

#[test]
fn huge_components_are_exact() {
    let parser = parser();
    for (src, key, want) in [
        ("99999999999999999999.0.0", "major", "99999999999999999999"),
        (
            "0.340282366920938463463374607431768211456.0",
            "minor",
            "340282366920938463463374607431768211456",
        ),
        ("0.0.18446744073709551616", "patch", "18446744073709551616"),
    ] {
        let got = must_parse(&parser, src);
        assert_eq!(
            field(&got, key),
            Value::String(want.to_string()),
            "parse({src:?})[{key}]"
        );
    }

    let got = must_parse(&parser, "1.0.0-alpha.9007199254740993");
    assert_eq!(
        items(&got, "prerelease")[1],
        Value::String("9007199254740993".into())
    );

    // A huge build identifier is a string like every other one.
    let got = must_parse(&parser, "1.0.0+9007199254740993");
    assert_eq!(items(&got, "build"), text(&["9007199254740993"]));
}

#[test]
fn format_round_trips_every_parse() {
    let parser = parser();
    for src in [
        "0.0.0",
        "1.2.3",
        "1.0.0-alpha",
        "1.0.0-alpha.1",
        "1.0.0-0.3.7",
        "1.0.0-x.7.z.92",
        "1.0.0-x-y-z.--",
        "1.0.0-alpha+001",
        "1.0.0+20130313144700",
        "1.0.0-beta+exp.sha.5114f85",
        "1.0.0+21AF26D3----117B344092BD",
        // Integers past 2^53 render as plain digits.
        "99999999254740993.0.0",
        "0.0.9007199254740992",
        "1.0.0-9007199254740993.x",
        "9007199254740991.9007199254740991.9007199254740991-9007199254740991+9007199254740991",
    ] {
        let value = must_parse(&parser, src);
        assert_eq!(
            format(&value).unwrap_or_else(|error| panic!("format({src:?}): {error}")),
            src
        );
    }
}

#[test]
fn format_renders_a_hand_built_value() {
    assert_eq!(
        format(&version(
            number(1),
            number(2),
            number(3),
            &[Value::String("rc".into()), number(1)],
            &["sha", "abc"]
        ))
        .expect("a well formed value formats"),
        "1.2.3-rc.1+sha.abc"
    );
    // The exact-digits form is accepted wherever a number is.
    assert_eq!(
        format(&version(
            Value::String("1".into()),
            number(0),
            number(0),
            &[],
            &[]
        ))
        .expect("exact digits format"),
        "1.0.0"
    );
    assert!(format(&Value::String("1.2.3".into())).is_err());
    assert!(format(&version(Value::Number(1.5), number(0), number(0), &[], &[])).is_err());
    assert!(format(&version(
        number(1),
        number(0),
        number(0),
        &[],
        &[] // a non-string build identifier is rejected below
    ))
    .is_ok());

    let mut fields = indexmap::IndexMap::new();
    fields.insert("major".to_string(), number(1));
    fields.insert("minor".to_string(), number(0));
    fields.insert("patch".to_string(), number(0));
    fields.insert("prerelease".to_string(), Value::array(vec![]));
    fields.insert("build".to_string(), Value::array(vec![number(1)]));
    assert!(
        format(&Value::object(fields)).is_err(),
        "a numeric build identifier is not a string"
    );
}

// An `f64` that is not a finite integer is not a component any parse can
// produce, and every entry point has to say so rather than render it or
// compare against it. An infinity is the one that slips through a naive
// guard: its truncation is itself and it is not negative, so it passes
// both an integer and a sign test.
#[test]
fn non_finite_components_are_rejected() {
    let big = version(
        Value::String("9007199254740993".into()),
        number(0),
        number(0),
        &[],
        &[],
    );
    let one = version(number(1), number(0), number(0), &[], &[]);

    for bad in [f64::INFINITY, f64::NEG_INFINITY, f64::NAN] {
        let value = version(Value::Number(bad), number(0), number(0), &[], &[]);
        assert!(format(&value).is_err(), "format({bad}) should fail");

        let in_prerelease = version(number(1), number(0), number(0), &[Value::Number(bad)], &[]);
        assert!(
            format(&in_prerelease).is_err(),
            "format(prerelease {bad}) should fail"
        );

        // Both orders, and against both an `f64` and an exact-digits
        // operand: only the second reaches the digit conversion.
        for other in [&big, &one] {
            assert!(compare(&value, other).is_err(), "compare({bad}, other)");
            assert!(compare(other, &value).is_err(), "compare(other, {bad})");
        }
    }
}

fn order(parser: &Tabnas, a: &str, b: &str) -> Ordering {
    compare(&must_parse(parser, a), &must_parse(parser, b))
        .unwrap_or_else(|error| panic!("compare({a:?}, {b:?}): {error}"))
}

fn below(parser: &Tabnas, a: &str, b: &str) {
    assert_eq!(
        order(parser, a, b),
        Ordering::Less,
        "{a} should rank below {b}"
    );
    assert_eq!(
        order(parser, b, a),
        Ordering::Greater,
        "{b} should rank above {a}"
    );
}

fn same(parser: &Tabnas, a: &str, b: &str) {
    assert_eq!(order(parser, a, b), Ordering::Equal, "{a} == {b}");
    assert_eq!(order(parser, b, a), Ordering::Equal, "{b} == {a}");
}

#[test]
fn compare_follows_the_specification() {
    let parser = parser();
    // 11.2
    below(&parser, "1.0.0", "2.0.0");
    below(&parser, "2.0.0", "2.1.0");
    below(&parser, "2.1.0", "2.1.1");
    below(&parser, "1.9.0", "1.10.0");
    below(&parser, "1.10.0", "1.11.0");
    below(&parser, "9.0.0", "10.0.0");
    // 11.3
    below(&parser, "1.0.0-alpha", "1.0.0");
    below(&parser, "1.0.0-0", "1.0.0");
    below(&parser, "1.0.0", "1.0.1-0");
    // 11.4, the specification's own chain, every pair.
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
    for (i, low) in chain.iter().enumerate() {
        for high in chain.iter().skip(i + 1) {
            below(&parser, low, high);
        }
    }
    // 11.4.1
    below(&parser, "1.0.0-2", "1.0.0-10");
    below(&parser, "1.0.0-rc.9", "1.0.0-rc.10");
    // 11.4.2
    below(&parser, "1.0.0-A", "1.0.0-a");
    below(&parser, "1.0.0-Z", "1.0.0-a");
    below(&parser, "1.0.0--", "1.0.0-0a");
    below(&parser, "1.0.0-a", "1.0.0-b");
    below(&parser, "1.0.0-a", "1.0.0-aa");
    below(&parser, "1.0.0-alpha", "1.0.0-alpha0");
    // 11.4.3
    below(&parser, "1.0.0-1", "1.0.0-a");
    below(&parser, "1.0.0-9007199254740993", "1.0.0--");
    below(&parser, "1.0.0-1", "1.0.0-1a");
    // 11.4.4
    below(&parser, "1.0.0-alpha", "1.0.0-alpha.1");
    below(&parser, "1.0.0-alpha.1", "1.0.0-alpha.1.0");
    // 10 and 11.1
    same(&parser, "1.0.0", "1.0.0+build");
    same(&parser, "1.0.0+a", "1.0.0+b");
    same(&parser, "1.0.0-alpha+1", "1.0.0-alpha+2");
    for src in ["0.0.0", "1.2.3-rc.1", "1.2.3+x"] {
        same(&parser, src, src);
    }
}

// Past 2^53 the representation changes, so every comparison that crosses
// the boundary goes through the exact-digit path. A port that compared
// `f64`s here would report 9007199254740992 and 9007199254740993 equal.
#[test]
fn compare_is_exact_past_the_safe_integer() {
    let parser = parser();
    below(&parser, "9007199254740991.0.0", "9007199254740992.0.0");
    below(&parser, "9007199254740992.0.0", "9007199254740993.0.0");
    same(&parser, "9007199254740993.0.0", "9007199254740993.0.0");
    below(&parser, "1.0.0-9007199254740992", "1.0.0-9007199254740993");
    below(&parser, "1.0.0-9007199254740993", "1.0.0-a");
    // Length decides before the digits do.
    below(&parser, "9999999999999999.0.0", "10000000000000000.0.0");
    below(
        &parser,
        "0.0.0-99999999999999999999999999999998",
        "0.0.0-99999999999999999999999999999999",
    );
}

// A pre-release identifier above 2^53 is the exact digits as a string,
// and the specification still ranks it BELOW every alphanumeric one
// (11.4.3). It is unambiguous: an alphanumeric identifier contains at
// least one non-digit by definition, so a string of digits in the
// pre-release list can only be a numeric identifier.
#[test]
fn a_huge_numeric_identifier_still_ranks_below_an_alphanumeric_one() {
    let parser = parser();
    for alphanumeric in ["1.0.0--", "1.0.0-0a", "1.0.0-A", "1.0.0-a", "1.0.0-zzz"] {
        below(&parser, "1.0.0-99999999999999999999", alphanumeric);
    }
}

// A leading zero is not a numeric identifier. `1.0.0-01` is rejected
// outright, and `01a` is ALPHANUMERIC: it ranks above every numeric
// identifier however large, and beside other strings in ASCII order.
#[test]
fn a_leading_zero_identifier_is_alphanumeric_or_rejected() {
    let parser = parser();
    assert!(parser.parse("1.0.0-01").is_err(), "01 is not an identifier");
    assert!(parser.parse("1.0.0-00").is_err());
    assert!(parser.parse("01.0.0").is_err());
    assert!(parser.parse("1.0.0-0.01").is_err());

    let value = must_parse(&parser, "1.0.0-01a");
    assert_eq!(
        items(&value, "prerelease"),
        text(&["01a"]),
        "01a keeps its zero and stays a string"
    );
    below(&parser, "1.0.0-99999999999999999999", "1.0.0-01a");
    below(&parser, "1.0.0-01a", "1.0.0-1a");

    // Build metadata has no such rule: `001` is a valid build identifier
    // and keeps its zeros.
    let value = must_parse(&parser, "1.0.0+001");
    assert_eq!(items(&value, "build"), text(&["001"]));
}

// The specification bounds neither the length of an identifier nor how
// many there are, so a very long pre-release is a valid version that
// `compare` has to order without running out of stack or time.
//
// The lengths are per profile, and each string is parsed ONCE rather
// than through `below`, which parses both of its arguments twice. A
// debug build carries the engine's `sync_rule_stack` assertion, which is
// O(depth) per loop iteration, and this grammar's depth grows with the
// input, so a debug parse is quadratic where a release parse is linear:
// 1,000 characters cost 7.0 s against 9.9 ms (measured 2026-09-21; see
// `AGENTS.md`). The lengths the Go suite uses would take hours in the
// profile `cargo test` actually builds. `tests/perf_test.rs` carries the
// survival check at the largest size each profile can afford.
#[test]
fn a_very_long_prerelease_compares() {
    let parser = parser();
    let letters = if cfg!(debug_assertions) { 300 } else { 8_000 };
    let count = if cfg!(debug_assertions) { 100 } else { 2_000 };

    let long = "a".repeat(letters);
    let lower = must_parse(&parser, &std::format!("1.0.0-{long}"));
    let higher = must_parse(&parser, &std::format!("1.0.0-{long}b"));
    assert_eq!(
        compare(&lower, &higher).expect("long identifiers compare"),
        Ordering::Less
    );
    assert_eq!(
        compare(&higher, &lower).expect("long identifiers compare"),
        Ordering::Greater
    );

    let many: Vec<String> = (0..count).map(|i| i.to_string()).collect();
    let list = must_parse(&parser, &std::format!("1.0.0-{}", many.join(".")));
    let longer = must_parse(&parser, &std::format!("1.0.0-{}.0", many.join(".")));
    assert_eq!(
        compare(&list, &longer).expect("long lists compare"),
        Ordering::Less
    );
    assert_eq!(
        compare(&longer, &list).expect("long lists compare"),
        Ordering::Greater
    );
    assert_eq!(
        compare(&list, &list).expect("a long list equals itself"),
        Ordering::Equal
    );

    // A shorter identifier set ranks below a longer one it prefixes.
    let short = must_parse(&parser, "1.0.0-0");
    assert_eq!(
        compare(&short, &list).expect("a prefix compares"),
        Ordering::Less
    );
}

#[test]
fn compare_rejects_values_that_are_not_versions() {
    let parser = parser();
    let good = must_parse(&parser, "1.0.0");
    assert!(compare(&Value::String("1.0.0".into()), &good).is_err());

    let mut fields = indexmap::IndexMap::new();
    fields.insert("major".to_string(), Value::String("x".into()));
    assert!(compare(&good, &Value::object(fields)).is_err());
}

#[test]
fn every_rejection_is_the_unexpected_code() {
    let parser = parser();
    let error = parser
        .parse("1.0.0-01")
        .expect_err("1.0.0-01 should be rejected");
    assert_eq!(error.code, "unexpected");
    assert_eq!((error.row, error.col), (1, 9));
    let hint = error.hint.clone();
    assert!(
        hint.contains("semver.org") && hint.contains("leading zero"),
        "the hint should explain the format: {hint:?}"
    );
    let rendered = error.to_string();
    assert!(rendered.contains("unexpected"), "{rendered}");
}

#[test]
fn the_empty_string_is_not_a_version() {
    let parser = parser();
    let error = parser.parse("").expect_err("the empty string is rejected");
    assert_eq!(error.code, "unexpected");
}

#[test]
fn the_error_names_the_character() {
    let parser = parser();
    for (src, col, at) in [
        ("v1.2.3", 1, "v"),
        ("1.2.3 ", 6, " "),
        ("1.2.3-a_b", 8, "_"),
    ] {
        let error = parser.parse(src).expect_err("rejected");
        assert_eq!((error.col, error.src.as_str()), (col, at), "parse({src:?})");
    }
}

#[test]
fn an_instance_is_reusable_after_a_failure() {
    let parser = parser();
    let first = must_parse(&parser, "1.0.0-a");
    assert!(parser.parse("x").is_err());
    assert_eq!(must_parse(&parser, "1.0.0-a"), first);
}

#[test]
fn the_plugin_installs_on_a_bare_engine() {
    let mut parser = Tabnas::new();
    tabnas_semver::semver(&mut parser).expect("the grammar installs");
    assert_eq!(field(&must_parse(&parser, "1.2.3"), "patch"), number(3));
    // Installing twice is a no-op, not a second grammar.
    tabnas_semver::semver(&mut parser).expect("a second install is a no-op");
    assert!(parser.parse("1.2.3").is_ok());
}

#[test]
fn the_plugin_installs_through_use_plugin() {
    let mut parser = Tabnas::new();
    parser
        .use_plugin(tabnas_semver::plugin(), None)
        .expect("the plugin installs");
    assert!(parser.parse("1.2.3").is_ok());
    assert!(parser.rule_names().iter().any(|name| name == "semver"));
}

// Every default lexer is off, so a character the grammar does not name is
// rejected rather than skipped as whitespace, swallowed as a comment or
// lexed as a JSON punctuation token.
#[test]
fn nothing_outside_the_grammar_is_lexed() {
    let parser = parser();
    for src in [
        " 1.2.3",
        "1.2.3\n",
        "\t1.2.3",
        "1.2.3#c",
        "\"1.2.3\"",
        "{1.2.3}",
        "[1.2.3]",
        "1.2.3,",
        "1.2.3:",
        "v1.2.3",
        "=1.2.3",
        "1.2.3 ",
    ] {
        let error = parser
            .parse(src)
            .map(|value| value.to_string())
            .expect_err(&std::format!("{src:?} should be rejected"));
        assert_eq!(error.code, "unexpected", "parse({src:?})");
    }
}

// The character classes the compiler emits must be lexable at any
// lookahead slot: the `*digit` helper peeks two digits, so without that
// the letter ending `01a` or `12a` is a fatal bad token at the second
// slot and no alternative can recover. The Rust emitter marks every
// class eager, so the plugin carries no port of the TypeScript fix; this
// is the observable half of that claim.
#[test]
fn a_letter_after_digits_lexes() {
    let parser = parser();
    for src in [
        "1.0.0-01a",
        "1.0.0-12a",
        "1.0.0-007a",
        "1.0.0-0a",
        "1.0.0-1a",
    ] {
        assert!(parser.parse(src).is_ok(), "{src} should parse");
    }
}

// The grammar's alphabet is `[0-9A-Za-z.+-]` and nothing else, so every
// non-ASCII character is outside it. Three things have to hold and none
// of them is free in Rust:
//
//   1. The rejection is the same `unexpected` code, at the same column,
//      as the canonical runtime gives. Every column below was measured
//      against the canonical TypeScript under Node 22 on 2026-09-21.
//   2. Nothing PANICS. A multibyte character sitting where the port
//      takes a byte offset is the panic this fleet has already found in
//      another crate, and the error path carries the offending source.
//   3. A Unicode-aware digit class would ACCEPT `1\u{6F2}.3.4`, whose
//      second character is the Arabic-Indic digit two. The `regex`
//      crate's `\d` matches it; the JavaScript being ported does not,
//      and neither does this port, because every class in it is spelled
//      out as ASCII.
//
// A lone surrogate is not in this list: a Rust `&str` cannot hold one,
// which is a property of the language rather than of this plugin, and
// the engine records it.
#[test]
fn odd_unicode_is_rejected_rather_than_mishandled() {
    let parser = parser();
    for (src, col) in [
        ("1.2.3\u{e9}", 6),
        ("\u{e9}", 1),
        ("1.2.3\u{4e2d}", 6),
        ("1.2.3\u{1F600}", 6),
        ("\u{1F600}1.2.3", 1),
        ("1.2.3\u{0}", 6),
        ("\u{0}", 1),
        ("1.2.3\u{feff}", 6),
        ("1.2.3\u{a0}", 6),
        ("1.2.3-\u{e9}", 7),
        ("1.2.3+\u{e9}", 7),
        // The Arabic-Indic digit two. A Unicode-aware `\d` would take it
        // for a digit and accept the version.
        ("1\u{6F2}.3.4", 2),
    ] {
        let error = parser
            .parse(src)
            .map(|value| value.to_string())
            .expect_err(&std::format!("{src:?} should be rejected"));
        assert_eq!(error.code, "unexpected", "parse({src:?})");
        assert_eq!(error.col, col, "parse({src:?}) column");
        // The report renders the offending source, which is where a byte
        // offset into a multibyte character would panic.
        assert!(!error.to_string().is_empty(), "parse({src:?}) report");
    }
}

// The same alphabet argument, from the other side: a long run of
// multibyte text, and a valid version with one multibyte character
// appended, must be rejected rather than panic, at every truncation
// point. The engine takes byte offsets, and a slice that split a
// character would abort.
#[test]
fn a_multibyte_tail_never_panics() {
    let parser = parser();
    let mut cases: Vec<String> = Vec::new();
    for tail in ["\u{e9}", "\u{4e2d}", "\u{1F600}", "\u{e9}\u{4e2d}\u{1F600}"] {
        for base in [
            "", "1", "1.", "1.2", "1.2.3", "1.2.3-a", "1.2.3+a", "1.2.3-",
        ] {
            cases.push(std::format!("{base}{tail}"));
            cases.push(std::format!("{tail}{base}"));
        }
        cases.push(tail.repeat(500));
    }
    for src in &cases {
        // Every truncation at a character boundary, which is what makes
        // a construct unterminated with a multibyte character last.
        for end in (1..=src.len()).filter(|end| src.is_char_boundary(*end)) {
            let piece = &src[..end];
            if let Err(error) = parser.parse(piece) {
                assert_eq!(error.code, "unexpected", "parse({piece:?})");
            }
        }
    }
}

#[test]
fn the_grammar_text_is_the_specification_grammar() {
    for line in [
        "semver = valid-semver",
        r#"valid-semver = version-core [ "-" pre-release ] [ "+" build ]"#,
        "positive-digit = %x31-39",
    ] {
        assert!(
            GRAMMAR.contains(&std::format!("\n{line}\n")),
            "the grammar should contain the line {line:?}"
        );
    }
}

#[test]
fn the_convenience_parse_shares_one_instance() {
    let value = parse("1.2.3-rc.1+x").expect("a version parses");
    assert_eq!(
        value,
        version(
            number(1),
            number(2),
            number(3),
            &[Value::String("rc".into()), number(1)],
            &["x"]
        )
    );
    assert!(parse("nope").is_err());
}

// `parse` and `make` both document the shared instance as safe for
// concurrent use, where the Go convenience serialises callers through a
// mutex. The claim is measured rather than asserted in prose: a parse
// builds a fresh context per call and only reads instance state, so
// threads hammering the same instance, and the shared one behind
// `parse`, must all get the same answers.
#[test]
fn the_shared_instance_is_safe_across_threads() {
    use std::sync::Arc;

    let shared = Arc::new(make());
    let cases = [
        ("1.2.3-alpha.1+build.5", true),
        ("0.0.0", true),
        ("9007199254740992.0.0", true),
        ("v1.2.3", false),
        ("", false),
    ];

    let want: Vec<Option<String>> = cases
        .iter()
        .map(|(src, _)| shared.parse(src).ok().map(|value| value.to_string()))
        .collect();

    let handles: Vec<_> = (0..8)
        .map(|_| {
            let instance = Arc::clone(&shared);
            std::thread::spawn(move || {
                let mut out = Vec::new();
                for _ in 0..25 {
                    out.clear();
                    for (src, ok) in cases {
                        let own = instance.parse(src).ok().map(|value| value.to_string());
                        let sharedly = parse(src).ok().map(|value| value.to_string());
                        assert_eq!(own.is_some(), ok, "parse({src:?}) on a shared instance");
                        assert_eq!(own, sharedly, "parse({src:?}) disagreed with the default");
                        out.push(own);
                    }
                }
                out
            })
        })
        .collect();

    for handle in handles {
        assert_eq!(
            handle.join().expect("no thread panicked"),
            want,
            "a thread saw a different answer"
        );
    }
}

#[test]
fn a_fresh_instance_gives_the_same_answer() {
    let shared = parse("1.0.0-alpha.1+build.5").expect("parses");
    let fresh = parse_fresh("1.0.0-alpha.1+build.5").expect("parses");
    assert_eq!(
        tabnas_support::format_value(&fresh),
        tabnas_support::format_value(&common::to_value(&shared))
    );
}

#[test]
fn the_crate_version_is_itself_a_version() {
    let value = parse(VERSION).expect("VERSION is a version string");
    assert_eq!(format(&value).expect("it formats"), VERSION);
}
