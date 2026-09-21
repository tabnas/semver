// Shared test helpers. Cargo compiles this module into EVERY integration
// test binary, so an item only one binary uses is dead code in the
// others; the allow keeps that from being a warning rather than hiding
// anything real.
#![allow(dead_code)]

use std::path::{Path, PathBuf};

use tabnas_support::{find_spec_dir, Failure, Value};

/// The shared `test/spec` directory, found by walking up from the crate
/// rather than by counting `..` hops.
pub fn spec_dir() -> PathBuf {
    find_spec_dir(Some(Path::new(env!("CARGO_MANIFEST_DIR"))))
        .expect("a test/spec directory above rs/")
}

/// The shared `test/precedence` directory. It sits BESIDE `test/spec`,
/// not in it: the parity runner treats every row under `spec/` as a parse
/// case, and these files are pairs and chains for `compare`.
pub fn precedence_dir() -> PathBuf {
    spec_dir()
        .parent()
        .expect("test/spec has a parent")
        .join("precedence")
}

/// The repository root, one level above `test/`.
pub fn repo_dir() -> PathBuf {
    spec_dir()
        .parent()
        .and_then(Path::parent)
        .expect("test/spec sits two levels below the repository root")
        .to_path_buf()
}

/// An engine value as the fixture data model, through JSON. This is the
/// Rust half of the Go runner's `jsonFlatten` and the TypeScript runner's
/// round trip, so a value compares the same way in all three.
pub fn to_value(value: &tabnas::Value) -> Value {
    Value::from(value.to_json())
}

/// A parse error as the runner's failure: the code the fixture pins, and
/// the rendered report for the failure message.
pub fn to_failure(error: tabnas::TabnasError) -> Failure {
    Failure::new(error.code.clone())
        .at(error.row, error.col)
        .with_message(error.to_string())
}

/// Parse one version through the crate's shared default instance, as the
/// Go and TypeScript fixture runners do. Compiling the grammar dominates
/// a parse by orders of magnitude, and the plugin keeps no per-parse
/// state on the instance, so a fixture row cannot reach the next one.
pub fn parse_shared(input: &str) -> Result<Value, Failure> {
    tabnas_semver::parse(input)
        .map(|value| to_value(&value))
        .map_err(to_failure)
}

/// Parse one version with a FRESH parser, for the tests that prove an
/// instance carries nothing between parses.
pub fn parse_fresh(input: &str) -> Result<Value, Failure> {
    tabnas_semver::make()
        .parse(input)
        .map(|value| to_value(&value))
        .map_err(to_failure)
}
