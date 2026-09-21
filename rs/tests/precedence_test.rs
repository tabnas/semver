// Precedence (specification section 11), driven by the shared fixtures in
// `test/precedence/` at the repository root: the same files
// `ts/test/precedence.test.ts` and `go/precedence_test.go` run, so
// `compare` cannot drift between the runtimes.
//
//   order.tsv  one version per row, in strictly ascending precedence
//   equal.tsv  pairs that compare equal (build metadata is ignored)
//
// These sit beside, not in, `test/spec/`, whose runner treats every row
// as a parse case.

mod common;

use std::cmp::Ordering;

use tabnas_support::load_spec;

use common::precedence_dir;

#[test]
fn order_is_a_strictly_ascending_chain() {
    let spec = load_spec(
        precedence_dir().join("order.tsv"),
        &tabnas_support::SpecOptions::default(),
    )
    .expect("test/precedence/order.tsv is readable");
    assert!(
        20 < spec.rows.len(),
        "order.tsv has {} rows; expected a real chain",
        spec.rows.len()
    );

    let parser = tabnas_semver::make();
    let mut versions: Vec<String> = Vec::with_capacity(spec.rows.len());
    let mut parsed: Vec<tabnas::Value> = Vec::with_capacity(spec.rows.len());

    for row in &spec.rows {
        let version = row.named("version").to_string();
        let value = parser
            .parse(&version)
            .unwrap_or_else(|error| panic!("{}: {error}", row.location()));
        assert_eq!(
            tabnas_semver::format(&value).expect("a parsed version formats"),
            version,
            "{}: format did not give the input back",
            row.location()
        );
        assert_eq!(
            tabnas_semver::compare(&value, &value).expect("a parsed version compares"),
            Ordering::Equal,
            "{}: {version} should equal itself",
            row.location()
        );
        versions.push(version);
        parsed.push(value);
    }

    // Every pair, not only the adjacent ones, so the file pins
    // transitivity as well as order.
    for (i, low) in parsed.iter().enumerate() {
        for (k, high) in parsed.iter().enumerate().skip(i + 1) {
            assert_eq!(
                tabnas_semver::compare(low, high).expect("parsed versions compare"),
                Ordering::Less,
                "compare({}, {}) should be Less",
                versions[i],
                versions[k]
            );
            assert_eq!(
                tabnas_semver::compare(high, low).expect("parsed versions compare"),
                Ordering::Greater,
                "compare({}, {}) should be Greater",
                versions[k],
                versions[i]
            );
        }
    }
}

#[test]
fn equal_pairs_compare_equal_both_ways() {
    let spec = load_spec(
        precedence_dir().join("equal.tsv"),
        &tabnas_support::SpecOptions::default(),
    )
    .expect("test/precedence/equal.tsv is readable");
    assert!(
        5 < spec.rows.len(),
        "equal.tsv has {} rows",
        spec.rows.len()
    );

    let parser = tabnas_semver::make();
    for row in &spec.rows {
        let (a, b) = (row.named("a"), row.named("b"));
        let va = parser
            .parse(a)
            .unwrap_or_else(|error| panic!("{}: {error}", row.location()));
        let vb = parser
            .parse(b)
            .unwrap_or_else(|error| panic!("{}: {error}", row.location()));
        assert_eq!(
            tabnas_semver::compare(&va, &vb).expect("parsed versions compare"),
            Ordering::Equal,
            "{}: compare({a}, {b})",
            row.location()
        );
        assert_eq!(
            tabnas_semver::compare(&vb, &va).expect("parsed versions compare"),
            Ordering::Equal,
            "{}: compare({b}, {a})",
            row.location()
        );
    }
}
