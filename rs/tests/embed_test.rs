// The embedded grammar is the grammar. `semver-grammar.abnf` at the
// repository root is the single source of truth, and `ts/embed-grammar.js`
// copies it verbatim into `ts/src/semver.ts`, `go/semver.go` and
// `rs/src/lib.rs` between the BEGIN/END markers. This holds the Rust copy
// to the file on disk, so an edit to the grammar that forgets the embed
// step, or a hand edit between the markers, fails here rather than
// shipping a runtime that compiles a different language from the other
// two.

use std::fs;
use std::path::Path;

fn repo_root() -> &'static Path {
    Path::new(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .expect("rs/ has a parent")
}

#[test]
fn the_embedded_grammar_is_the_file_on_disk() {
    let on_disk = fs::read_to_string(repo_root().join("semver-grammar.abnf"))
        .expect("semver-grammar.abnf is readable at the repository root");

    // The embed script writes a newline, then the file, into the raw
    // string; the file's own trailing newline closes it.
    let embedded = tabnas_semver::GRAMMAR;
    assert_eq!(
        embedded.strip_prefix('\n').unwrap_or(embedded),
        on_disk,
        "rs/src/lib.rs GRAMMAR_TEXT differs from semver-grammar.abnf: \
         run `node ts/embed-grammar.js`"
    );
}

#[test]
fn the_grammar_text_fits_the_raw_string() {
    // A Rust raw string `r#"..."#` ends at the first `"#`, so the embed
    // script refuses a grammar holding that pair. Pinned here so the two
    // halves of the claim agree.
    assert!(!tabnas_semver::GRAMMAR.contains("\"#"));
}

// The embedded text is also identical in the other two runtimes, which is
// what makes the shared fixtures a parity contract rather than three
// separate suites that happen to agree. Read both copies out of their
// sources and compare them with this one.
#[test]
fn all_three_runtimes_embed_the_same_text() {
    let on_disk = fs::read_to_string(repo_root().join("semver-grammar.abnf"))
        .expect("semver-grammar.abnf is readable");

    for (path, opener, closer) in [
        (
            repo_root().join("ts").join("src").join("semver.ts"),
            "const grammarText = `\n",
            "`\n",
        ),
        (
            repo_root().join("go").join("semver.go"),
            "const grammarText = `\n",
            "`\n",
        ),
    ] {
        let source = fs::read_to_string(&path)
            .unwrap_or_else(|error| panic!("{} is readable: {error}", path.display()));
        let body = source
            .split_once(opener)
            .and_then(|(_, rest)| rest.split_once(closer))
            .map(|(body, _)| body)
            .unwrap_or_else(|| panic!("{} has no embedded grammar", path.display()));

        // The TypeScript copy escapes a backslash, a backtick and a
        // template expression for its template literal; the grammar has
        // none of the three, so both copies are byte identical to the
        // source. Assert that rather than assume it.
        assert_eq!(
            body,
            on_disk,
            "{} embeds a different grammar",
            path.display()
        );
    }
}
