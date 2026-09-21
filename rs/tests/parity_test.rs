// The shared conformance fixtures, every one of them.
//
// `test/spec/*.tsv` at the repository root is the parity contract: the
// TypeScript suite (`ts/test/parity.test.ts`), the Go suite
// (`go/parity_test.go` `TestSpec`) and this file run the same rows. A row
// green in one runtime and red in another is a failure, not a
// discrepancy.
//
// Discovery is by LISTING the directory, in all three runtimes, so adding
// a `.tsv` runs it everywhere without touching a runner. That is also why
// there is no exemption list here and no tripwire test beside it: nothing
// can be left unrun.
//
// Every rejection is the engine's base `unexpected` code, compared
// EXACTLY: this plugin declares no codes of its own, so that one code is
// the whole rejection contract (see `AGENTS.md`, "Error codes").

mod common;

use tabnas_support::Runner;

use common::{parse_shared, spec_dir};

#[test]
fn spec() {
    // The exact code, with no allowances, which is what both other
    // runners assert. `Failure` already carries the code, so the default
    // comparison is the one wanted.
    //
    // One instance for every row: the plugin has no options and keeps no
    // per-parse state on the instance, so nothing can leak between rows,
    // and compiling the grammar per row would only make the suite slow.
    Runner::new(parse_shared).dir(spec_dir());
}
