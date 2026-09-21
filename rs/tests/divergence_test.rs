// The Rust half of every row in `../DIVERGENCE.md`, executed.
//
// A divergence recorded in prose alone goes stale the first time
// somebody repairs it, and nothing notices. These assertions fail in
// BOTH directions: they pin the REPRESENTATION this port uses, not just
// the value, so a port that starts agreeing with the canonical runtime
// goes red and names the row to delete, exactly as a port that starts
// disagreeing does.
//
// The shared register under `../test/spec` cannot carry the entry. Every
// row there is a parse case compared after a JSON round trip, and the
// value this entry is about is the one JSON cannot carry:
// `JSON.stringify` throws on a `bigint`. `../test/AGENTS.md` records the
// same limit, which is why the canonical suites keep these cases in
// `ts/test/semver.test.ts` and `go/semver_test.go` rather than in a
// fixture.
//
// The canonical cells in `../DIVERGENCE.md` were measured on 2026-09-21
// by running the canonical TypeScript under Node 22 and the Go port
// through its own module. They are not asserted here: this repository's
// `ts/` has no committed build and no committed `node_modules`, so a
// Rust gate that ran the canonical would be red in every checkout that
// has not run `npm install` first. The canonical halves are asserted by
// the canonical suites, case for case with these.

mod common;

use std::cmp::Ordering;

use tabnas::Value;
use tabnas_semver::{compare, format, make, MAX_SAFE_INTEGER};

use common::spec_dir;

fn fields(pairs: Vec<(&str, Value)>) -> Value {
    let mut map = indexmap::IndexMap::new();
    for (key, value) in pairs {
        map.insert(key.to_string(), value);
    }
    Value::object(map)
}

fn version(major: Value, prerelease: Vec<Value>) -> Value {
    fields(vec![
        ("major", major),
        ("minor", Value::Number(0.0)),
        ("patch", Value::Number(0.0)),
        ("prerelease", Value::array(prerelease)),
        ("build", Value::array(vec![])),
    ])
}

fn field(value: &Value, key: &str) -> Value {
    match value {
        Value::Object(map) => map.get(key).cloned().expect("the field is present"),
        other => panic!("not a version value: {other:?}"),
    }
}

fn items(value: &Value, key: &str) -> Vec<Value> {
    match field(value, key) {
        Value::Array(values) => values.to_vec(),
        other => panic!("{key} is not a list: {other:?}"),
    }
}

// --- entry 1, the parse table ------------------------------------------

// Row by row from `../DIVERGENCE.md`, entry 1. A component at
// MAX_SAFE_INTEGER is a NUMBER; one past it is the exact decimal digits
// in a STRING. TypeScript answers a `bigint` above the boundary and Go a
// `*big.Int`.
#[test]
fn the_representation_changes_at_the_safe_integer() {
    let parser = make();

    let at = parser.parse("9007199254740991.0.0").expect("parses");
    assert_eq!(
        field(&at, "major"),
        Value::Number(MAX_SAFE_INTEGER as f64),
        "at the boundary the component is a number in all three runtimes"
    );

    for (src, digits) in [
        ("9007199254740992.0.0", "9007199254740992"),
        ("99999999999999999999.0.0", "99999999999999999999"),
    ] {
        let value = parser.parse(src).expect("parses");
        assert_eq!(
            field(&value, "major"),
            Value::String(digits.to_string()),
            "{src}: past the boundary this port carries the exact digits as a \
             string, where TypeScript has a bigint and Go a *big.Int. If this \
             is now a number, DIVERGENCE.md entry 1 has changed and the row \
             must be rewritten or deleted."
        );
    }

    let pre = parser
        .parse("1.0.0-alpha.9007199254740993")
        .expect("parses");
    assert_eq!(
        items(&pre, "prerelease")[1],
        Value::String("9007199254740993".to_string()),
        "a numeric pre-release identifier past the boundary takes the same form"
    );

    // The row that does NOT diverge: a build identifier is a string in
    // every runtime, however long.
    let build = parser.parse("1.0.0+9007199254740993").expect("parses");
    assert_eq!(
        items(&build, "build"),
        vec![Value::String("9007199254740993".to_string())],
        "a build identifier is a string in every runtime"
    );
}

// The VALUE is never wrong, which is the other half of the entry: the
// digits are exact, `format` gives the input back, and `compare` orders
// across the boundary without going through an f64.
#[test]
fn the_value_is_exact_on_both_sides_of_the_boundary() {
    let parser = make();
    for src in [
        "9007199254740992.0.0",
        "0.0.18446744073709551616",
        "0.340282366920938463463374607431768211456.0",
        "1.0.0-9007199254740993.x",
        "99999999999999999999.99999999999999999999.99999999999999999999",
    ] {
        let value = parser.parse(src).expect("parses");
        assert_eq!(format(&value).expect("formats"), src, "format({src})");
    }

    let pairs = [
        ("9007199254740991.0.0", "9007199254740992.0.0"),
        ("9007199254740992.0.0", "9007199254740993.0.0"),
        ("1.0.0-9007199254740992", "1.0.0-9007199254740993"),
        ("9999999999999999.0.0", "10000000000000000.0.0"),
    ];
    for (low, high) in pairs {
        let a = parser.parse(low).expect("parses");
        let b = parser.parse(high).expect("parses");
        assert_eq!(
            compare(&a, &b).expect("compares"),
            Ordering::Less,
            "{low} should rank below {high}; a port comparing f64s would call \
             them equal"
        );
        assert_eq!(compare(&b, &a).expect("compares"), Ordering::Greater);
    }
}

// --- entry 1, the consequence table -------------------------------------

// Because a string of digits IS a number to this port, `compare` reads a
// hand-built digit string in a pre-release list as NUMERIC, where the
// canonical runtimes read any string there as alphanumeric. No parse
// produces such a value; the difference is reachable only by hand.
#[test]
fn a_hand_built_digit_string_is_numeric_here() {
    // The one row that differs. TypeScript and Go both answer 1.
    let digits = version(Value::Number(1.0), vec![Value::String("1".to_string())]);
    let number = version(Value::Number(1.0), vec![Value::Number(2.0)]);
    assert_eq!(
        compare(&digits, &number).expect("compares"),
        Ordering::Less,
        "a hand-built \"1\" is a numeric identifier here, so it compares \
         against 2 numerically. TypeScript and Go read it as alphanumeric and \
         answer 1. If this is now Greater, DIVERGENCE.md entry 1's consequence \
         table has closed."
    );
    assert_eq!(
        compare(&number, &digits).expect("compares"),
        Ordering::Greater
    );

    // The two rows that agree, and why: "1" sorts below "a" in ASCII
    // order, and a numeric identifier ranks below an alphanumeric one,
    // so both readings reach the same verdict.
    let alpha = version(Value::Number(1.0), vec![Value::String("a".to_string())]);
    assert_eq!(compare(&digits, &alpha).expect("compares"), Ordering::Less);
    let one = version(Value::Number(1.0), vec![Value::Number(1.0)]);
    assert_eq!(compare(&one, &alpha).expect("compares"), Ordering::Less);
}

// A PARSED value cannot reach the difference above, which is what makes
// it a consequence rather than a defect: the grammar's `<alphanumeric
// identifier>` contains at least one non-digit by definition, so a
// string in a parsed pre-release list is all digits only when it is the
// exact-digits form of a number past the boundary.
#[test]
fn a_parsed_prerelease_string_is_never_all_digits_below_the_boundary() {
    let parser = make();
    for src in [
        "1.0.0-0.10.a1.1a.01a.-1",
        "1.0.0-alpha.1.beta.2",
        "1.0.0-0a.00a.007a",
        "1.0.0-x.7.z.92",
    ] {
        let value = parser.parse(src).expect("parses");
        for item in items(&value, "prerelease") {
            if let Value::String(text) = &item {
                assert!(
                    !text.bytes().all(|byte| byte.is_ascii_digit()),
                    "{src}: the parsed identifier {text:?} is all digits and \
                     below the boundary, which would make the numeric reading \
                     wrong for a value a parse can produce"
                );
            }
        }
    }
}

// --- the register -------------------------------------------------------

// Stated out loud rather than assumed from a file nobody notices is
// missing: this repository has no executable divergence register,
// because the one entry cannot be written as a fixture row. The shared
// spec directory is checked to be what the parity runner reads and
// nothing else.
#[test]
fn there_is_no_executable_register_and_that_is_deliberate() {
    let dir = spec_dir();
    let mut files: Vec<String> = std::fs::read_dir(&dir)
        .expect("test/spec is readable")
        .filter_map(|entry| entry.ok())
        .map(|entry| entry.file_name().to_string_lossy().to_string())
        .filter(|name| name.ends_with(".tsv"))
        .collect();
    files.sort();
    assert_eq!(
        files,
        vec![
            "build.tsv",
            "core.tsv",
            "prerelease.tsv",
            "spec-examples.tsv",
            "strict.tsv",
        ],
        "test/spec holds the parse fixtures and nothing else. A divergent.tsv \
         here would be run as parse cases by all three parity runners, which \
         is why DIVERGENCE.md entry 1 is pinned by this file instead."
    );
    tabnas_support::no_divergences(
        "test/spec: the one recorded divergence is a value JSON cannot carry, \
         so it is pinned by rs/tests/divergence_test.rs",
    );
}

// --- the ECMAScript number rendering ------------------------------------

// `format` renders a `Value::Number` with `js_number_to_string`, because
// the canonical runtime renders it with `String(n)`. Rust's own shortest
// formatter is not the same function, and these are the cases where the
// two part company: a decimal midpoint, where the specification takes
// the even digit and Rust rounds away from zero, and the switch to
// exponent form at 1e21 and 1e-7, which Rust never makes.
//
// The whole function was graded against node over 60,000 doubles on
// 2026-09-21, every one of them matching; what is kept here is the part
// that fails if somebody replaces the call with `to_string`.
#[test]
fn numbers_render_as_javascript_renders_them() {
    for (value, want) in [
        (137839762462415.62_f64, "137839762462415.62"),
        (1e21, "1e+21"),
        (1e-7, "1e-7"),
        (1e20, "100000000000000000000"),
        (1e-6, "0.000001"),
        (0.0, "0"),
        (-0.0, "0"),
        (9007199254740991.0, "9007199254740991"),
        (5e-324, "5e-324"),
        (f64::MAX, "1.7976931348623157e+308"),
    ] {
        assert_eq!(
            tabnas_semver::js_number_text(value),
            want,
            "js_number_to_string({value:?})"
        );
    }
    assert_eq!(tabnas_semver::js_number_text(f64::NAN), "NaN");
    assert_eq!(tabnas_semver::js_number_text(f64::INFINITY), "Infinity");
    assert_eq!(
        tabnas_semver::js_number_text(f64::NEG_INFINITY),
        "-Infinity"
    );

    // Rust's own formatter really does disagree on the first two, so the
    // test above is measuring something rather than restating it.
    assert_ne!(
        137839762462415.62_f64.to_string(),
        tabnas_semver::js_number_text(137839762462415.62)
    );
    assert_ne!(1e21_f64.to_string(), tabnas_semver::js_number_text(1e21));

    // And it is reachable through the public surface, on a value built
    // by hand: a parsed component is always an integer below 2^53, where
    // every formatter agrees.
    let huge = version(Value::Number(1e21), vec![]);
    assert_eq!(format(&huge).expect("formats"), "1e+21.0.0");
}
