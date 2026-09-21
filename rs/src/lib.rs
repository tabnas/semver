// Copyright (c) 2026 Richard Rodger and other contributors, MIT License

// The engine's error carries a code, position, hint and a formatted
// report, so it is large by design and `Result<_, TabnasError>` trips
// clippy's `result_large_err`. The engine allows the lint at its own crate
// root for the same reason; boxing here would make `parse` return a
// different shape from `Tabnas::parse` and from the other two ports.
#![allow(clippy::result_large_err)]

//! Semantic Versioning 2.0.0 ([semver.org](https://semver.org)) for the
//! `tabnas` parsing engine.
//!
//! The parser IS the specification's grammar. `semver-grammar.abnf` at
//! the repository root (embedded below) is the semver.org BNF transcribed
//! into RFC 5234 ABNF, and [`tabnas_abnf`] compiles it into the engine's
//! rule set when the plugin is installed. Nothing here decides what a
//! valid version is: the grammar accepts or rejects, and the only code
//! that runs during a parse is the one action that turns the accepted
//! text into the value.
//!
//! ```
//! fn main() -> Result<(), Box<dyn std::error::Error>> {
//!     let value = tabnas_semver::parse("1.2.3-alpha.1+build.5")?;
//!     assert_eq!(
//!         value.to_string(),
//!         r#"{"major":1,"minor":2,"patch":3,"prerelease":["alpha",1],"build":["build","5"]}"#
//!     );
//!     Ok(())
//! }
//! ```
//!
//! [`compare`] implements the specification's precedence rules (section
//! 11) over two parsed values, and [`format`] renders a value back to its
//! string.
//!
//! TypeScript is canonical: `ts/src/semver.ts` defines behaviour, the
//! grammar itself is authored once in `semver-grammar.abnf` and embedded
//! into every runtime, and the shared fixtures in `test/spec/*.tsv` and
//! `test/precedence/*.tsv` are the parity contract across TypeScript, Go
//! and Rust. Where this port cannot match the canonical value exactly,
//! `DIVERGENCE.md` at the repository root records it.

use std::cmp::Ordering;
use std::fmt;
use std::sync::{Arc, OnceLock};

use serde_json::{json, Map as JsonMap, Value as Json};
use tabnas::{Context, GrammarError, Plugin, PluginError, Rule, Tabnas, Value};
use tabnas_abnf::{
    abnf_convert, attach_actions, to_recognition_spec, AbnfConvertOptions, ActionFn, GrammarSpec,
    RefAction,
};

/// The README's Rust examples run as doctests, so a stale one fails the
/// gate rather than misleading the reader. Its `toml` and `bash` fences
/// are skipped; rustdoc runs only the `rust` ones.
#[cfg(doctest)]
#[doc = include_str!("../README.md")]
mod readme_examples {}

/// This crate's version. It MUST equal `ts/package.json` "version": the
/// release orchestrator rewrites both, and `tests/version_test.rs` fails
/// the build if they drift. Mirrors `VERSION` in `ts/src/semver.ts` and
/// `const VERSION` in `go/semver.go`.
pub const VERSION: &str = "0.0.2";

/// The plugin's name on an instance, and the key its option bag hangs
/// under.
pub const PLUGIN_NAME: &str = "Semver";

/// The largest integer a numeric component is returned as a
/// [`Value::Number`] for; anything larger is a [`Value::String`] of its
/// decimal digits. It is 2^53 - 1, JavaScript's `Number.MAX_SAFE_INTEGER`,
/// so all three runtimes change representation at the same value. See
/// `DIVERGENCE.md` for what the representation is in each.
pub const MAX_SAFE_INTEGER: u64 = 9_007_199_254_740_991;

/// `MAX_SAFE_INTEGER` as decimal digits, for the comparison that decides
/// the representation without going through an `f64`.
const MAX_SAFE_DIGITS: &str = "9007199254740991";

// --- BEGIN EMBEDDED semver-grammar.abnf ---
const GRAMMAR_TEXT: &str = r#"
; Semantic Versioning 2.0.0 — the grammar of a valid version string.
;
;   https://semver.org/spec/v2.0.0.html
;   (section "Backus–Naur Form Grammar for Valid SemVer Versions")
;
; This file is the single source of truth for @tabnas/semver. It is RFC
; 5234 ABNF, compiled by @tabnas/abnf into a tabnas grammar at plugin
; install time, in both runtimes (ts/src/semver.ts and go/semver.go embed
; it verbatim; "npm run embed" copies it there — never edit the copies).
;
; Every production keeps the name the specification gives it, with the
; specification's spaces written as hyphens ("<version core>" is
; "version-core"), and — with two exceptions explained below — the
; specification's shape. The language accepted is EXACTLY the language of
; the specification's grammar: the two rewrites are equivalences, not
; approximations, and the conformance suite in both runtimes checks the
; result against the regular expression semver.org publishes, on every
; string of a short alphabet up to length five and on thousands of
; mutated versions.
;
; The engine sees one character per token: the plugin switches every
; default lexer (whitespace, line ends, comments, strings, numbers, bare
; words) off, so nothing outside this grammar can be consumed, and a
; blank, a tab, a newline or a "v" prefix is rejected like any other
; character the grammar does not name.

; The entry point. A pure alias of the specification's root production;
; it exists so the plugin has one unhyphenated rule name to hang its
; value-building action on (see ts/src/semver.ts, "@semver:ac").
semver = valid-semver

; <valid semver> ::= <version core>
;                  | <version core> "-" <pre-release>
;                  | <version core> "+" <build>
;                  | <version core> "-" <pre-release> "+" <build>
valid-semver = version-core [ "-" pre-release ] [ "+" build ]

; <version core> ::= <major> "." <minor> "." <patch>
version-core = major "." minor "." patch

major = numeric-identifier
minor = numeric-identifier
patch = numeric-identifier

; <pre-release> ::= <dot-separated pre-release identifiers>
; <dot-separated pre-release identifiers> ::= <pre-release identifier>
;   | <pre-release identifier> "." <dot-separated pre-release identifiers>
pre-release = pre-release-identifier *( "." pre-release-identifier )

; <build> ::= <dot-separated build identifiers>
; <dot-separated build identifiers> ::= <build identifier>
;   | <build identifier> "." <dot-separated build identifiers>
build = build-identifier *( "." build-identifier )

; <pre-release identifier> ::= <alphanumeric identifier>
;                            | <numeric identifier>
;
; REWRITE 1 of 2. Both alternatives can begin with a digit ("1" is
; numeric, "1a" is alphanumeric, "01a" is alphanumeric, "01" is nothing),
; and the decision may need every character of the identifier, so the
; specification's shape is not LL(1). It is factored on the first
; character instead:
;
;   "0" alone is the numeric identifier zero; "0" followed by more digits
;   is valid only if a non-digit eventually appears (an alphanumeric
;   identifier such as "007a" — leading zeros are fine there);
;
;   a positive digit starts a numeric identifier, which turns into an
;   alphanumeric one if a non-digit appears after the digits;
;
;   anything else must be a non-digit, and starts an alphanumeric
;   identifier.
;
; The union of the three is exactly <alphanumeric identifier> ∪ <numeric
; identifier>; what it excludes is exactly a digit string with a leading
; zero, which the specification also excludes.
pre-release-identifier = "0" [ *digit alphanumeric-tail ]
                       / positive-digit *digit [ alphanumeric-tail ]
                       / alphanumeric-tail

; <alphanumeric identifier> ::= <non-digit>
;                             | <non-digit> <identifier characters>
;                             | <identifier characters> <non-digit>
;                             | <identifier characters> <non-digit> <identifier characters>
;
; i.e. a non-empty string of identifier characters containing at least one
; non-digit. Split at its FIRST non-digit, such a string is
;
;   *digit alphanumeric-tail
;
; and "alphanumeric-tail" is the part from that first non-digit on. It is
; the form the factored pre-release-identifier above consumes after it
; has already read the leading digits.
alphanumeric-tail = non-digit *identifier-character

; <build identifier> ::= <alphanumeric identifier>
;                      | <digits>
;
; REWRITE 2 of 2. An alphanumeric identifier is a non-empty identifier
; string with a non-digit in it; <digits> is a non-empty identifier
; string with no non-digit in it. Their union is every non-empty
; identifier string, leading zeros included ("001" is a valid build
; identifier).
build-identifier = 1*identifier-character

; <numeric identifier> ::= "0"
;                        | <positive digit>
;                        | <positive digit> <digits>
numeric-identifier = "0" / positive-digit *digit

; <identifier character> ::= <digit>
;                          | <non-digit>
identifier-character = digit / non-digit

; <non-digit> ::= <letter>
;               | "-"
non-digit = letter / "-"

; <digit> ::= "0"
;           | <positive digit>
digit = "0" / positive-digit

; <positive digit> ::= "1" | "2" | "3" | "4" | "5" | "6" | "7" | "8" | "9"
positive-digit = %x31-39

; <letter> ::= "A" | "B" | ... | "Z" | "a" | "b" | ... | "z"
letter = %x41-5A / %x61-7A
"#;

// --- END EMBEDDED semver-grammar.abnf ---

/// The grammar as ABNF text: the same text the plugin compiles.
pub const GRAMMAR: &str = GRAMMAR_TEXT;

/// A parse failure. Every rejection is the engine's base `unexpected`
/// code: this plugin declares no codes of its own (see `AGENTS.md`).
pub use tabnas::TabnasError as SemverError;

/// Plugin options. There are none yet, in any runtime; the type exists so
/// that adding one is not a breaking change. Mirrors `SemverOptions` in
/// `ts/src/semver.ts` and `Defaults` in `go/semver.go`.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct SemverOptions;

/// A value handed to [`compare`] or [`format`] that is not a version this
/// crate can read: a missing or mistyped field, or a number no parse can
/// produce.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct VersionError(pub String);

impl fmt::Display for VersionError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str(&self.0)
    }
}

impl std::error::Error for VersionError {}

/// The hint the engine renders beside every `unexpected` rejection. Same
/// text in all three runtimes.
const UNEXPECTED_HINT: &str = "
The character(s) {src} do not match any rule alternative active at
this position.

A Semantic Version is MAJOR.MINOR.PATCH — three integers with no
leading zeros — optionally followed by -PRERELEASE and +BUILD, each a
dot-separated list of non-empty identifiers made of [0-9A-Za-z-],
where a numeric pre-release identifier has no leading zero. Nothing
else is allowed: no whitespace, no \"v\" prefix, no empty identifier.
See https://semver.org/spec/v2.0.0.html";

/// Install the semver grammar on an engine instance.
///
/// ```
/// fn main() -> Result<(), Box<dyn std::error::Error>> {
///     let mut parser = tabnas::Tabnas::new();
///     tabnas_semver::semver(&mut parser)?;
///     assert_eq!(parser.parse("1.2.3")?.to_string(),
///         r#"{"major":1,"minor":2,"patch":3,"prerelease":[],"build":[]}"#);
///     Ok(())
/// }
/// ```
pub fn semver(parser: &mut Tabnas) -> Result<(), GrammarError> {
    // Guard against re-invocation on the same instance: the grammar is
    // stateless, but compiling and installing it twice is wasted work.
    // The Go port keeps a `semver-init` decoration for this; the engine's
    // own rule list answers the same question without inventing a key.
    if parser.rule_names().iter().any(|name| name == "semver") {
        return Ok(());
    }

    // Compile the specification's grammar into an engine rule set. The
    // start rule is `semver`, a pure alias of the specification's
    // `valid-semver`: the name has no hyphen in it, which used to be what
    // let a lifecycle hook bind on the published TypeScript engine. The
    // hook has since moved to the compiler's end-of-source wrapper
    // (below), so nothing depends on that any more, but the alias stays:
    // it is the name this plugin's grammar, fixtures and diagnostics all
    // use for the entry production.
    let convert = AbnfConvertOptions {
        start: Some("semver".to_string()),
        tag: Some("semver".to_string()),
        ..AbnfConvertOptions::default()
    };
    let mut spec = abnf_convert(GRAMMAR_TEXT, Some(&convert))
        .map_err(|error| GrammarError(format!("semver: grammar failed to compile: {error}")))?;

    // The single semantic action, on the compiler's end-of-source wrapper
    // -- the rule `abnf_convert` names in `options.rule.start`, normally
    // `__start__` (it numbers the name only if the grammar declares one
    // itself, which this one does not). That rule closes on `#ZZ` and
    // nothing else, so it closes exactly when the whole source has been
    // accepted: the context's source is then the accepted text, character
    // for character, because with every default lexer off (below) nothing
    // was skipped on the way in.
    //
    // The wrapper, not `semver`: `semver` closes as soon as a VERSION has
    // been read, which for `1.2.3f` happens before the engine discovers
    // the trailing `f`. Building a value there would report a wrong
    // version on a string the grammar is about to reject.
    let start_rule = start_rule_name(&spec);
    let build_value: ActionFn = Arc::new(|rule: &mut Rule, ctx: &mut Context| {
        *rule.node.borrow_mut() = from_text(&ctx.source);
        Ok(())
    });
    attach_actions(
        &mut spec,
        vec![(format!("@{start_rule}:ac"), vec![build_value])],
    )
    .map_err(|error| GrammarError(format!("semver: {error}")))?;

    // `attach_actions` on a rule phase records the hook under the engine's
    // own `@<rule>-<phase>` name, which the engine auto-installs when that
    // rule is loaded. Register that one ref and no other: `GrammarSpec::bind`
    // would also register the compiler's tree-building closures, and the
    // recognition document built below references none of them.
    for (name, action) in &spec.refs {
        if let RefAction::Phase(actions) = action {
            let actions = actions.clone();
            parser.state_action_ref(name.as_str(), move |rule, ctx| {
                for action in &actions {
                    action(rule, ctx)?;
                }
                Ok(())
            });
        }
    }

    // ... and then throw the tree away. `to_recognition_spec` strips every
    // AST-building action the compiler emitted and returns the same rules,
    // the same tokens and the same accepted language as pure data.
    //
    // This is not an optimisation to taste. Building the `{rule, src,
    // kids}` tree is QUADRATIC in an identifier's length here: each
    // `*`/`1*` repetition compiles to a per-character helper that
    // re-appends its child's `src` and re-copies its `kids` at every
    // nesting level. The plugin never reads that tree -- the action above
    // builds the value from the accepted text -- so nothing is lost, and
    // `tests/perf_test.rs` pins three parts of the repair: a long
    // identifier parses at all, the cost of eight times the input stays
    // inside the profile's bound, and the emitted document carries no
    // tree-building action at all. A release build is then linear, and
    // measured so to 1,000,000 characters; a debug build is quadratic
    // for a reason that lives in the engine rather than here, which
    // `AGENTS.md` records and which is why the test sizes differ by
    // profile.
    let mut document =
        to_recognition_spec(&spec).map_err(|error| GrammarError(format!("semver: {error}")))?;
    apply_options(&mut document)?;

    let engine = tabnas::GrammarSpec::from_value(document)?;
    parser.grammar(&engine)?;
    Ok(())
}

/// The name of the compiler's end-of-source wrapper, from the converted
/// spec's own options. `__start__` unless the grammar declared that name
/// itself, which this one does not.
fn start_rule_name(spec: &GrammarSpec) -> String {
    spec.options
        .get("rule")
        .and_then(|rule| rule.get("start"))
        .and_then(Json::as_str)
        .unwrap_or("__start__")
        .to_string()
}

/// Everything the engine lexes by default is switched off, so a character
/// the grammar does not name -- a blank, a newline, a `v` prefix, a quote,
/// a `#` -- has no matcher and is rejected as `unexpected` rather than
/// skipped as whitespace or a comment. The engine's default punctuation
/// tokens go too: they are JSON's, not semver's, and would otherwise show
/// up in diagnostics and introspection as tokens of this grammar.
///
/// Every key here is replaced rather than merged, exactly as the
/// TypeScript plugin's object spread replaces it; only `fixed.token`
/// keeps what the compiled grammar put there.
fn apply_options(document: &mut Json) -> Result<(), GrammarError> {
    let options = document
        .get_mut("options")
        .and_then(Json::as_object_mut)
        .ok_or_else(|| GrammarError("semver: the compiled grammar has no options".into()))?;

    let mut fixed_token = options
        .get("fixed")
        .and_then(|fixed| fixed.get("token"))
        .and_then(Json::as_object)
        .cloned()
        .unwrap_or_else(JsonMap::new);
    for name in ["#OB", "#CB", "#OS", "#CS", "#CL", "#CA"] {
        fixed_token.insert(name.to_string(), Json::Null);
    }
    options.insert("fixed".to_string(), json!({ "token": fixed_token }));

    for lexer in [
        "space", "line", "comment", "string", "number", "text", "value",
    ] {
        options.insert(lexer.to_string(), json!({ "lex": false }));
    }

    // The empty string is not a version. By default the engine answers an
    // empty source with no value at all, before any rule runs.
    options.insert("lex".to_string(), json!({ "empty": false }));

    // Every rejection is the engine's base `unexpected` code (this plugin
    // declares no codes of its own -- see AGENTS.md); the hint is where a
    // reader learns what a version has to look like.
    options.insert("hint".to_string(), json!({ "unexpected": UNEXPECTED_HINT }));

    Ok(())
}

/// The plugin form of [`semver`], for [`Tabnas::use_plugin`]. Installed
/// this way the grammar is re-applied to derived instances, as every
/// native plugin is.
pub fn plugin() -> Plugin {
    Plugin::new(PLUGIN_NAME, |parser, _options| {
        semver(parser).map_err(|error| PluginError(error.0))
    })
    .with_defaults(Value::object(Default::default()))
}

/// Build a semver parser: a bare engine with this plugin installed, the
/// counterpart of `new Tabnas().use(Semver)` and the Go `Make()`.
///
/// Compiling the grammar dominates a parse, so build one and reuse it.
///
/// ```
/// fn main() -> Result<(), Box<dyn std::error::Error>> {
///     let parser = tabnas_semver::make();
///     let value = parser.parse("10.20.30-rc.1")?;
///     assert_eq!(tabnas_semver::format(&value)?, "10.20.30-rc.1");
///     Ok(())
/// }
/// ```
pub fn make() -> Tabnas {
    make_with(SemverOptions)
}

/// Build a semver parser with plugin options. [`SemverOptions`] carries
/// nothing yet, in every port; the entry point exists so that adding one
/// is not a breaking change.
pub fn make_with(_options: SemverOptions) -> Tabnas {
    let mut parser = Tabnas::new();
    parser
        .use_plugin(plugin(), None)
        .expect("the embedded semver grammar is fixed and valid");
    parser
}

/// Parse one version string with a shared default parser.
///
/// Compiling the grammar dominates a parse, so the no-options path reuses
/// one instance. Parsing builds a fresh context per call and only reads
/// instance state, so the shared instance is safe for concurrent use.
///
/// ```
/// fn main() -> Result<(), Box<dyn std::error::Error>> {
///     let value = tabnas_semver::parse("1.0.0+21AF26D3----117B344092BD")?;
///     assert_eq!(tabnas_semver::format(&value)?, "1.0.0+21AF26D3----117B344092BD");
///     Ok(())
/// }
/// ```
pub fn parse(src: &str) -> Result<Value, SemverError> {
    static DEFAULT: OnceLock<Tabnas> = OnceLock::new();
    DEFAULT.get_or_init(make).parse(src)
}

// --- the value -----------------------------------------------------------

/// Build the value from accepted text. The grammar has already proven the
/// text well-formed, so every split below is total and unambiguous:
/// version-core contains no `-`, so the first `-` (before any `+`) opens
/// the pre-release; no identifier contains `+`, so the first `+` opens the
/// build metadata; and `.` separates identifiers, which never contain it.
///
/// The accepted language is `[0-9A-Za-z.+-]` only, so every index below is
/// a character boundary.
fn from_text(text: &str) -> Value {
    let mut core = text;

    let mut build: Vec<Value> = Vec::new();
    if let Some(plus) = core.find('+') {
        build = core[plus + 1..]
            .split('.')
            .map(|part| Value::String(part.to_string()))
            .collect();
        core = &core[..plus];
    }

    let mut prerelease: Vec<Value> = Vec::new();
    if let Some(dash) = core.find('-') {
        prerelease = core[dash + 1..].split('.').map(identifier).collect();
        core = &core[..dash];
    }

    let mut parts = core.split('.');
    let major = integer(parts.next().unwrap_or(""));
    let minor = integer(parts.next().unwrap_or(""));
    let patch = integer(parts.next().unwrap_or(""));

    let mut out = indexmap::IndexMap::with_capacity(5);
    out.insert("major".to_string(), major);
    out.insert("minor".to_string(), minor);
    out.insert("patch".to_string(), patch);
    out.insert("prerelease".to_string(), Value::array(prerelease));
    out.insert("build".to_string(), Value::array(build));
    Value::object(out)
}

/// A pre-release identifier: numeric when it is all digits (the grammar
/// has already excluded a leading zero there), a string otherwise.
fn identifier(text: &str) -> Value {
    if is_digits(text) {
        integer(text)
    } else {
        Value::String(text.to_string())
    }
}

/// ASCII digits only, and at least one. `char::is_numeric` and the
/// `regex` crate's `\d` are Unicode-aware; the JavaScript this is ported
/// from tests character codes 48 to 57, so the class is spelled out.
fn is_digits(text: &str) -> bool {
    !text.is_empty() && text.bytes().all(|byte| byte.is_ascii_digit())
}

/// A digit string as an exact integer: a [`Value::Number`] up to
/// [`MAX_SAFE_INTEGER`], and the canonical decimal digits as a
/// [`Value::String`] beyond it, because [`Value`] has one numeric variant
/// and it is an `f64`. See `DIVERGENCE.md`, entry 1.
fn integer(digits: &str) -> Value {
    let canonical = canonical_digits(digits);
    if compare_digits(&canonical, MAX_SAFE_DIGITS) == Ordering::Greater {
        Value::String(canonical)
    } else {
        // At most 2^53 - 1, so the parse and the cast are both exact.
        Value::Number(canonical.parse::<u64>().unwrap_or(0) as f64)
    }
}

/// Decimal digits with any leading zeros removed, keeping one digit for
/// zero itself. The grammar admits a leading zero in neither a version
/// core component nor a numeric pre-release identifier, so this is the
/// identity on accepted text; it is here so the comparison below never
/// has to think about `007`.
fn canonical_digits(digits: &str) -> String {
    let trimmed = digits.trim_start_matches('0');
    if trimmed.is_empty() {
        "0".to_string()
    } else {
        trimmed.to_string()
    }
}

/// Two non-negative decimal digit strings, by value. Both must already be
/// canonical: more digits is larger, and equal lengths compare
/// lexicographically.
fn compare_digits(left: &str, right: &str) -> Ordering {
    left.len().cmp(&right.len()).then_with(|| left.cmp(right))
}

// --- format --------------------------------------------------------------

/// Render a parsed value back to its version string. For a value that
/// came out of [`parse`] this is the exact input text.
///
/// ```
/// fn main() -> Result<(), Box<dyn std::error::Error>> {
///     let value = tabnas_semver::parse("1.0.0-beta+exp.sha.5114f85")?;
///     assert_eq!(tabnas_semver::format(&value)?, "1.0.0-beta+exp.sha.5114f85");
///     Ok(())
/// }
/// ```
pub fn format(version: &Value) -> Result<String, VersionError> {
    let fields = object(version)?;
    let mut out = String::new();

    for (index, key) in ["major", "minor", "patch"].into_iter().enumerate() {
        if 0 < index {
            out.push('.');
        }
        out.push_str(
            &number_text(field(fields, key))
                .map_err(|error| VersionError(format!("semver: {key}: {error}")))?,
        );
    }

    for (index, part) in list(fields, "prerelease")?.iter().enumerate() {
        out.push(if 0 == index { '-' } else { '.' });
        match part {
            Value::String(text) => out.push_str(text),
            other => out.push_str(
                &number_text(Some(other))
                    .map_err(|error| VersionError(format!("semver: prerelease: {error}")))?,
            ),
        }
    }

    for (index, part) in list(fields, "build")?.iter().enumerate() {
        out.push(if 0 == index { '+' } else { '.' });
        match part {
            Value::String(text) => out.push_str(text),
            _ => {
                return Err(VersionError(
                    "semver: build identifier is not a string".into(),
                ))
            }
        }
    }

    Ok(out)
}

/// The decimal text of one numeric component.
///
/// A [`Value::String`] here is the exact-digits form this port uses above
/// [`MAX_SAFE_INTEGER`], so it is written out as it stands. A
/// [`Value::Number`] goes through [`js_number_to_string`], because the
/// canonical runtime renders it with `String(n)` and the two only agree
/// when this reproduces ECMAScript's algorithm.
fn number_text(value: Option<&Value>) -> Result<String, VersionError> {
    match value {
        Some(Value::Number(number)) => {
            // The finite test comes first: an infinity survives both of
            // the others (its truncation is itself, and it is not
            // negative) and would render as "Infinity".
            if !number.is_finite() {
                return Err(VersionError(format!("not a finite number: {number}")));
            }
            if *number != number.trunc() || *number < 0.0 {
                return Err(VersionError(format!(
                    "not a non-negative integer: {number}"
                )));
            }
            Ok(js_number_to_string(*number))
        }
        Some(Value::String(digits)) if is_digits(digits) => Ok(canonical_digits(digits)),
        Some(other) => Err(VersionError(format!("not a number: {other:?}"))),
        None => Err(VersionError("missing".into())),
    }
}

// --- compare -------------------------------------------------------------

/// Order two parsed values by precedence, as the specification defines it
/// (section 11). Build metadata is ignored (sections 10 and 11.1), so
/// `1.0.0+a` and `1.0.0+b` compare equal.
///
/// ```
/// fn main() -> Result<(), Box<dyn std::error::Error>> {
///     use std::cmp::Ordering;
///     let alpha = tabnas_semver::parse("1.0.0-alpha")?;
///     let release = tabnas_semver::parse("1.0.0")?;
///     assert_eq!(tabnas_semver::compare(&alpha, &release)?, Ordering::Less);
///     Ok(())
/// }
/// ```
pub fn compare(left: &Value, right: &Value) -> Result<Ordering, VersionError> {
    let a = object(left)?;
    let b = object(right)?;

    for key in ["major", "minor", "patch"] {
        let order = compare_number(field(a, key), field(b, key))
            .map_err(|error| VersionError(format!("semver: {key}: {error}")))?;
        if order != Ordering::Equal {
            return Ok(order);
        }
    }

    compare_prerelease(list(a, "prerelease")?, list(b, "prerelease")?)
}

/// Section 11.2: numerically. Every value a parse produces is an exact
/// integer, in one representation or the other, so the two are compared
/// as exact decimal digits whenever they are not both `f64`.
fn compare_number(left: Option<&Value>, right: Option<&Value>) -> Result<Ordering, VersionError> {
    if let (Some(Value::Number(x)), Some(Value::Number(y))) = (left, right) {
        // The fast path still has to reject what a parse cannot produce:
        // every comparison with a NaN is false, so without this it would
        // report two versions equal, and an infinity would order against
        // a real version instead of failing as it does below.
        finite(*x)?;
        finite(*y)?;
        return Ok(x.partial_cmp(y).unwrap_or(Ordering::Equal));
    }
    Ok(compare_digits(&digits_of(left)?, &digits_of(right)?))
}

fn finite(number: f64) -> Result<(), VersionError> {
    if number.is_finite() {
        Ok(())
    } else {
        Err(VersionError(format!("not a finite number: {number}")))
    }
}

/// One numeric component as exact decimal digits, whichever
/// representation it arrived in.
fn digits_of(value: Option<&Value>) -> Result<String, VersionError> {
    match value {
        Some(Value::Number(number)) => {
            finite(*number)?;
            if *number != number.trunc() || *number < 0.0 {
                return Err(VersionError(format!(
                    "not a non-negative integer: {number}"
                )));
            }
            // Fixed precision, never the shortest form: `{:.0}` writes
            // every digit of an integral `f64` and never switches to
            // exponent notation, which is what a digit-string comparison
            // needs.
            Ok(canonical_digits(&format!("{number:.0}")))
        }
        Some(Value::String(digits)) if is_digits(digits) => Ok(canonical_digits(digits)),
        Some(other) => Err(VersionError(format!("not a number: {other:?}"))),
        None => Err(VersionError("missing".into())),
    }
}

/// Section 11.3: a pre-release version ranks below the associated normal
/// version. Section 11.4: otherwise identifier by identifier, left to
/// right, and a larger set of identifiers ranks above a smaller one when
/// every preceding identifier is equal.
fn compare_prerelease(left: &[Value], right: &[Value]) -> Result<Ordering, VersionError> {
    match (left.is_empty(), right.is_empty()) {
        (true, true) => return Ok(Ordering::Equal),
        (true, false) => return Ok(Ordering::Greater),
        (false, true) => return Ok(Ordering::Less),
        (false, false) => {}
    }
    for (x, y) in left.iter().zip(right.iter()) {
        let order = compare_identifier(x, y)
            .map_err(|error| VersionError(format!("semver: prerelease: {error}")))?;
        if order != Ordering::Equal {
            return Ok(order);
        }
    }
    Ok(left.len().cmp(&right.len()))
}

/// Whether one pre-release identifier is numeric.
///
/// A [`Value::Number`] always is. A [`Value::String`] is numeric exactly
/// when it is all ASCII digits: that is this port's representation for a
/// numeric identifier above [`MAX_SAFE_INTEGER`], and no ALPHANUMERIC
/// identifier can look like it, because the specification's
/// `<alphanumeric identifier>` requires at least one non-digit.
fn is_numeric_identifier(value: &Value) -> bool {
    match value {
        Value::Number(_) => true,
        Value::String(text) => is_digits(text),
        _ => false,
    }
}

/// Section 11.4.1: numeric identifiers numerically. Section 11.4.2:
/// alphanumeric identifiers lexically in ASCII sort order. Section
/// 11.4.3: a numeric identifier always ranks below an alphanumeric one.
fn compare_identifier(left: &Value, right: &Value) -> Result<Ordering, VersionError> {
    match (is_numeric_identifier(left), is_numeric_identifier(right)) {
        (true, true) => compare_number(Some(left), Some(right)),
        (true, false) => Ok(Ordering::Less),
        (false, true) => Ok(Ordering::Greater),
        (false, false) => match (left, right) {
            (Value::String(x), Value::String(y)) => Ok(compare_utf16(x, y)),
            _ => Err(VersionError(
                "identifier is neither a number nor a string".into(),
            )),
        },
    }
}

/// Two strings in JavaScript's `<` order, which compares UTF-16 code
/// units. Rust's `str` ordering compares scalar values, and the two
/// disagree above U+FFFF: a JavaScript comparison puts an astral
/// character (a surrogate pair, so a leading code unit in U+D800..U+DBFF)
/// below U+E000..U+FFFF, and a scalar comparison puts it above. Every
/// identifier the grammar accepts is ASCII, where the two agree; this is
/// for a value built by hand.
fn compare_utf16(left: &str, right: &str) -> Ordering {
    if left.is_ascii() && right.is_ascii() {
        return left.as_bytes().cmp(right.as_bytes());
    }
    left.encode_utf16().cmp(right.encode_utf16())
}

// --- value access --------------------------------------------------------

type Fields = indexmap::IndexMap<String, Value>;

fn object(value: &Value) -> Result<&Fields, VersionError> {
    match value {
        Value::Object(fields) => Ok(fields),
        _ => Err(VersionError(
            "semver: not a parsed version (want an object)".into(),
        )),
    }
}

fn field<'v>(fields: &'v Fields, key: &str) -> Option<&'v Value> {
    fields.get(key)
}

fn list<'v>(fields: &'v Fields, key: &str) -> Result<&'v [Value], VersionError> {
    match fields.get(key) {
        Some(Value::Array(values)) => Ok(values.as_slice()),
        _ => Err(VersionError(format!("semver: {key} is not a list"))),
    }
}

// --- ECMAScript number rendering ----------------------------------------

/// JavaScript's `String(n)` for an `f64`: ECMA-262 6.1.6.1.20,
/// `Number::toString` with radix 10.
///
/// The canonical runtime renders a numeric component with `String(n)`, so
/// [`format`] only equals TypeScript's output when this reproduces it.
/// Rust's own shortest formatter does not: it breaks an exact decimal
/// midpoint away from zero where the specification takes the even digit,
/// and it never switches to exponent form at 1e21. Copied from
/// `js_number` in `tabnas-bnf` (`rs/src/spec.rs`) rather than written a
/// seventh time; every port in this fleet that renders a number carries
/// the same function.
///
/// A value this crate parses is a non-negative integer at most
/// `2^53 - 1`, where every formatter agrees. The cases below it covers
/// and Rust's does not can only arrive in a value built by hand.
fn js_number_to_string(number: f64) -> String {
    if number.is_nan() {
        return "NaN".to_string();
    }
    if number.is_infinite() {
        return if number > 0.0 {
            "Infinity"
        } else {
            "-Infinity"
        }
        .to_string();
    }
    // Covers -0.0, which JavaScript prints as "0".
    if number == 0.0 {
        return "0".to_string();
    }
    let magnitude = number.abs();
    // The specification's `s` and `n`: the fewest digits that read back
    // as this same `f64`, correctly rounded. Rust's fixed-precision
    // `{:e}` is correctly rounded and breaks ties to even, which is the
    // rule the specification states when two digit strings are equally
    // close, so the first width that round-trips gives the specification's
    // digits. Plain `{:e}` (shortest) is NOT a substitute: it breaks those
    // ties the other way, and prints 137839762462415.63 where JavaScript
    // prints 137839762462415.62. Seventeen digits always suffice for an
    // `f64`.
    let (digits, exponent) = (0..17usize)
        .map(|precision| format!("{magnitude:.precision$e}"))
        .find(|text| text.parse::<f64>() == Ok(magnitude))
        .map(|text| {
            let (mantissa, exponent) = text.split_once('e').expect("{:e} emits an exponent");
            (
                mantissa.chars().filter(|c| *c != '.').collect::<String>(),
                exponent
                    .parse::<i32>()
                    .expect("{:e} emits an integer exponent"),
            )
        })
        .expect("17 significant digits round-trip every finite f64");
    let k = digits.len() as i32;
    let n = exponent + 1;

    let body = if k <= n && n <= 21 {
        // 12 -> "12", 1e19 -> "10000000000000000000"
        format!("{}{}", digits, "0".repeat((n - k) as usize))
    } else if 0 < n && n <= 21 {
        // 1.5 -> "1.5"
        format!("{}.{}", &digits[..n as usize], &digits[n as usize..])
    } else if -6 < n && n <= 0 {
        // 1e-6 -> "0.000001"
        format!("0.{}{}", "0".repeat(-n as usize), digits)
    } else {
        // 1e21 -> "1e+21", 1e-7 -> "1e-7"
        let e = n - 1;
        let head = if k == 1 {
            digits.clone()
        } else {
            format!("{}.{}", &digits[..1], &digits[1..])
        };
        format!("{}e{}{}", head, if e < 0 { '-' } else { '+' }, e.abs())
    };
    if number < 0.0 {
        format!("-{body}")
    } else {
        body
    }
}

/// The ECMAScript number rendering, for the test that compares it with
/// node. Not part of the plugin's surface.
#[doc(hidden)]
pub fn js_number_text(number: f64) -> String {
    js_number_to_string(number)
}
