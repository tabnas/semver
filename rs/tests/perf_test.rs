// The performance traps this package exposes, pinned the way
// `go/perf_test.go` and `ts/test/perf.test.ts` pin them.
//
// Every check here is machine-INDEPENDENT: each compares two ways of
// doing the same work on the SAME machine in the SAME run, so a slow or
// busy box cannot make it flaky. There is deliberately NO absolute
// wall-clock budget.
//
// The three measurements live in ONE `#[test]`, and that is
// load-bearing. The test harness runs a binary's tests in parallel
// unless it is told otherwise, and `ci/rust/run.sh` runs a plain `cargo
// test --all-targets` with no `--test-threads`. Split into separate test
// functions, one measurement's expensive loop overlaps another's cheap
// one, and every asserted ratio becomes a function of core count rather
// than of the code. That was observed here, not imagined: all three took
// over a minute and none of them measured what it claimed to.
//
// SIZES. `cargo test` builds with debug assertions on, and under them
// the engine's `Context::sync_rule_stack` compares every retained stack
// frame against the live rule on every loop iteration. This grammar's
// rule depth grows with the input, because each `*` repetition compiles
// to a per-character helper, so that check turns a linear parse into a
// quadratic one: 1,000 characters cost 7.0 s in a debug build and 9.9 ms
// in a release one (measured 2026-09-21; `AGENTS.md` carries the whole
// ladder, up to the 1,000,000 character string that a release build
// accepts in 14.3 s). So every size below is chosen per profile. The Go
// suite affords 32,000 in the only profile it has; here that would take
// hours in the profile the gate actually runs, and would prove nothing
// the smaller size does not.

use std::time::{Duration, Instant};

use tabnas::Tabnas;
use tabnas_abnf::{abnf_convert, to_recognition_spec, AbnfConvertOptions};

const SRC: &str = "1.2.3-alpha.1+build.5";

/// Iterations in each half of the reuse comparison. Small enough that
/// the unoptimised profile stays quick, large enough to average out
/// scheduler noise: one rebuild compiles the whole ABNF and installs the
/// rule set, which is three orders of magnitude more than a parse, so
/// the margin here is never close. Ten rebuilds cost about ten seconds
/// in a debug build, and thirty cost thirty for no more confidence.
const N: usize = 10;

/// The ratio instance reuse has to beat. The real margin is orders of
/// magnitude; 4x is the floor a regression has to stay above, and it is
/// the figure the Go and TypeScript suites use.
const REUSE_WANT: u32 = 4;

/// A long identifier and the short one it is measured against, 8x apart.
const SHORT_LEN: usize = if cfg!(debug_assertions) { 100 } else { 2_000 };
const LONG_LEN: usize = if cfg!(debug_assertions) { 800 } else { 16_000 };

/// The bound on `time(LONG) / time(SHORT)` for 8x the input.
///
/// A release build is linear, so 24x is three times linear and only a
/// return to quadratic trips it, which is the bound the Go and
/// TypeScript suites use. A debug build is ALREADY quadratic for the
/// reason at the top of this file, so linear is not the claim there: 8x
/// the input costs about 68x (measured), and 96x is that with headroom.
/// The debug bound therefore catches a cost growing faster than the
/// square, and the release bound one growing faster than linearly.
const COST_WANT: f64 = if cfg!(debug_assertions) { 96.0 } else { 24.0 };

/// The length used for the survival checks, where no ratio is measured.
const SURVIVE_LEN: usize = if cfg!(debug_assertions) { 400 } else { 32_000 };

fn time_parses(mut parse_once: impl FnMut()) -> Duration {
    let started = Instant::now();
    for _ in 0..N {
        parse_once();
    }
    started.elapsed()
}

fn time_one(parser: &Tabnas, src: &str) -> Duration {
    let started = Instant::now();
    parser.parse(src).expect("a long identifier parses");
    started.elapsed()
}

#[test]
fn the_cost_model_holds() {
    // --- 1. Instance reuse against rebuild-per-parse ---------------
    //
    // Compiling the ABNF and installing the rule set dominates a parse,
    // so a caller must build ONE instance and reuse it. The crate's own
    // `parse` does that behind a `OnceLock`; a change that rebuilds per
    // parse, or a convenience entry point that forgets to cache, is
    // caught here.
    for _ in 0..3 {
        tabnas_semver::make().parse(SRC).expect("parses");
    }
    let shared = tabnas_semver::make();
    for _ in 0..20 {
        shared.parse(SRC).expect("parses");
    }
    tabnas_semver::parse(SRC).expect("parses");

    let rebuild = time_parses(|| {
        tabnas_semver::make().parse(SRC).expect("rebuild parse");
    });
    let reuse = time_parses(|| {
        shared.parse(SRC).expect("reuse parse");
    });
    let convenience = time_parses(|| {
        tabnas_semver::parse(SRC).expect("shared parse");
    });

    assert!(
        reuse * REUSE_WANT < rebuild,
        "reuse {reuse:?} is not {REUSE_WANT}x cheaper than rebuild {rebuild:?}; \
         the grammar is no longer compiled at install, or `make` became free"
    );
    assert!(
        convenience * REUSE_WANT < rebuild,
        "`parse` {convenience:?} is not {REUSE_WANT}x cheaper than rebuild \
         {rebuild:?}; the shared instance is not being reused"
    );

    // --- 2. A long identifier survives -----------------------------
    //
    // A long identifier is VALID. The specification bounds neither the
    // length of a pre-release or build identifier nor how many there
    // are, and the regular expression semver.org publishes accepts every
    // string below. They arrive from lock files, tags and HTTP headers,
    // so they are attacker-chosen text (see `../AGENTS.md`, "Untrusted
    // input").
    //
    // While the plugin asked the compiler for the `{rule, src, kids}`
    // tree it never read, each of these was quadratic in the
    // identifier's length in EVERY profile: the per-character helper
    // chain re-appended its child's `src` and re-copied its kids at
    // every level. In TypeScript 16,000 letters cost about 10 s and
    // 2.7 GB and 32,000 aborted the process; in Go, 7.2 s and 9.1 GB,
    // and 32,000 was killed by the kernel OOM killer, which no error
    // return can catch. This part does not measure time. It measures
    // survival.
    let list: String = vec!["a"; SURVIVE_LEN / 2].join(".");
    for (what, src) in [
        (
            "pre-release identifier",
            std::format!("1.0.0-{}", "a".repeat(SURVIVE_LEN)),
        ),
        (
            "build identifier",
            std::format!("1.0.0+{}", "a".repeat(SURVIVE_LEN)),
        ),
        ("major", std::format!("{}.0.0", "1".repeat(SURVIVE_LEN))),
        ("identifier list", std::format!("1.0.0-{list}")),
    ] {
        let value = shared
            .parse(&src)
            .unwrap_or_else(|error| panic!("{what}: {error}"));
        assert_eq!(
            tabnas_semver::format(&value).unwrap_or_else(|error| panic!("{what}: {error}")),
            src,
            "{what}: format did not give the input back"
        );
    }

    // --- 3. ... and the cost of doing it stays inside the bound -----
    //
    // The instance is warm by now, so each length is parsed once. See
    // COST_WANT for what each profile's bound means.
    let short = time_one(&shared, &std::format!("1.0.0-{}", "a".repeat(SHORT_LEN)));
    let long = time_one(&shared, &std::format!("1.0.0-{}", "a".repeat(LONG_LEN)));
    let ratio = long.as_secs_f64() / short.as_secs_f64().max(f64::MIN_POSITIVE);
    assert!(
        ratio < COST_WANT,
        "identifier cost grew {ratio:.1}x for 8x the input ({SHORT_LEN} chars \
         {short:?}, {LONG_LEN} chars {long:?}); the bound for this profile is \
         {COST_WANT}x"
    );
}

// The structural half of the same repair, which no timing states as
// plainly: the document the plugin installs carries NO tree-building
// action.
//
// This reproduces the two compiler calls `semver` makes, because the
// engine publishes no way to read an installed rule's actions back. So
// it proves the pipeline yields a clean document for THIS grammar,
// rather than that `src/lib.rs` still runs it. It costs nothing, holds
// no timer and cannot disturb the measurements above, so it stays a test
// of its own.
#[test]
fn the_recognition_document_carries_no_tree_builders() {
    let convert = AbnfConvertOptions {
        start: Some("semver".to_string()),
        tag: Some("semver".to_string()),
        ..AbnfConvertOptions::default()
    };
    let spec = abnf_convert(tabnas_semver::GRAMMAR, Some(&convert)).expect("the grammar compiles");
    let document = to_recognition_spec(&spec).expect("a recognition document");
    let text = document.to_string();
    for builder in [
        "@node$",
        "@capture$",
        "@bubble$",
        "@fold$",
        "@object$",
        "@array$",
        "node$",
        "capture$",
        "fold$",
        "@bnf_",
    ] {
        assert!(
            !text.contains(builder),
            "the recognition document still carries {builder}, so the parse \
             would build the tree the plugin never reads"
        );
    }
}
