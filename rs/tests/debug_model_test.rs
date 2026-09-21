// Composition: the semver grammar plugin layered with the official debug
// plugin, mirroring `ts/test/debug-model.test.ts` assertion for
// assertion.
//
// `tabnas-debug` is a declared dev-dependency, so it is always present
// here and this test can never skip. The canonical suite resolves the
// plugin dynamically and SKIPS when it cannot, which removes the whole
// composition suite while the run still reports green; this one fails
// instead. There is no Go equivalent, which the root `AGENTS.md`
// records.
//
// What the model shows is the shape the root `AGENTS.md` gotchas
// describe: the compiler's end-of-source wrapper is the start rule, the
// `semver` alias is the only rule it pushes, every production of the
// grammar is present as a rule under its own name even where Paull's
// substitution means the parse never pushes it, and the lexer carries the
// grammar's four literals and three character classes with every default
// lexer switched off.

use tabnas_debug::{model, DebugModel, DebugOptions};

fn debug_parser() -> tabnas::Tabnas {
    let mut parser = tabnas_semver::make();
    tabnas_debug::apply(&mut parser, DebugOptions::quiet()).expect("the debug plugin installs");
    parser
}

fn described() -> DebugModel {
    model(&debug_parser())
}

#[test]
fn parses_normally_with_the_debug_plugin_installed() {
    let value = debug_parser()
        .parse("1.2.3-rc.1+sha.abc")
        .expect("a version parses with the debug plugin installed");
    assert_eq!(
        value.to_string(),
        r#"{"major":1,"minor":2,"patch":3,"prerelease":["rc",1],"build":["sha","abc"]}"#
    );
}

#[test]
fn the_start_rule_is_the_compilers_end_of_source_wrapper() {
    // The ABNF compiler wraps the start rule in `__start__`, which
    // consumes end-of-source; the user-visible entry is the `semver`
    // alias. The one semantic action hangs on this wrapper.
    assert_eq!(described().config.start, "__start__");
}

#[test]
fn the_plugin_list_names_semver() {
    let described = described();
    assert!(
        described
            .plugins
            .iter()
            .any(|plugin| plugin.name == tabnas_semver::PLUGIN_NAME),
        "plugins should list {}: {:?}",
        tabnas_semver::PLUGIN_NAME,
        described
            .plugins
            .iter()
            .map(|plugin| plugin.name.as_str())
            .collect::<Vec<_>>()
    );
}

#[test]
fn every_production_is_a_rule_under_its_own_name() {
    // Alongside the compiler's synthetic helpers. Paull's substitution
    // inlines the leading references (`version-core`, `major`,
    // `numeric-identifier` under it, the first identifier of each list),
    // so the parse never PUSHES some of these; the rules still exist.
    let described = described();
    let names: std::collections::BTreeSet<&str> = described
        .rules
        .iter()
        .map(|rule| rule.name.as_str())
        .collect();
    for name in [
        "__start__",
        "semver",
        "valid-semver",
        "version-core",
        "major",
        "minor",
        "patch",
        "pre-release",
        "build",
        "pre-release-identifier",
        "alphanumeric-tail",
        "build-identifier",
        "numeric-identifier",
        "identifier-character",
        "non-digit",
        "digit",
        "positive-digit",
        "letter",
    ] {
        assert!(names.contains(name), "rule {name} should be in the model");
    }
}

#[test]
fn the_wrapper_pushes_the_alias_and_the_alias_pushes_the_root() {
    let described = described();
    let edges = |name: &str| {
        described
            .graph
            .iter()
            .find(|edges| edges.name == name)
            .unwrap_or_else(|| panic!("the model has no {name} rule"))
    };
    assert_eq!(edges("__start__").open_push, vec!["semver".to_string()]);
    assert_eq!(edges("semver").open_push, vec!["valid-semver".to_string()]);
}

#[test]
fn the_lexer_carries_the_grammars_own_tokens() {
    // The four fixed literals (`0`, `.`, `-`, `+`) and the three
    // character classes. The engine's default punctuation tins stay
    // registered even though the plugin unbinds their source text, so
    // the model lists them too, with no fixed text.
    let described = described();
    let fixed = |name: &str| {
        described
            .tokens
            .iter()
            .find(|token| token.name == name)
            .unwrap_or_else(|| panic!("token {name} should be in the model"))
            .fixed
            .clone()
    };
    assert_eq!(fixed("#0"), Some("0".to_string()));
    assert_eq!(fixed("#T"), Some(".".to_string()));
    assert_eq!(fixed("#T1"), Some("-".to_string()));
    assert_eq!(fixed("#T2"), Some("+".to_string()));
    for class in [
        "#RX___U0031__U0039",
        "#RX___U0041__U005A",
        "#RX___U0061__U007A",
    ] {
        assert_eq!(fixed(class), None, "{class} is a character class");
    }
    for punctuation in ["#OB", "#CB", "#OS", "#CS", "#CL", "#CA"] {
        assert_eq!(
            fixed(punctuation),
            None,
            "{punctuation} should have no source text: the plugin unbinds it"
        );
    }
}

// Alignment rule 4 in the root `AGENTS.md`, read back from the live
// instance rather than from the options the plugin wrote: every default
// lexer is off, and only the grammar's own fixed tokens are lexed.
#[test]
fn every_default_lexer_is_off() {
    let described = described();
    for (lexer, enabled) in &described.config.lex {
        assert_eq!(
            *enabled,
            lexer == "fixed",
            "lexer {lexer} should be {} on a semver instance",
            if lexer == "fixed" { "on" } else { "off" }
        );
    }
    assert!(
        described.config.lex.contains_key("space") && described.config.lex.contains_key("text"),
        "the model should report the built-in lexers: {:?}",
        described.config.lex
    );
}

#[test]
fn the_live_grammar_renders_back_to_abnf() {
    let described = described();
    assert!(
        described.abnf.contains("valid-semver"),
        "the ABNF rendering should name the specification's root:\n{}",
        described.abnf
    );
}

#[test]
fn the_grammar_portion_is_json_serialisable_and_round_trips() {
    let described = described();
    let grammar = serde_json::json!({
        "tokens": described.tokens,
        "rules": described.rules,
        "graph": described.graph,
        "config": described.config,
    });
    let again: serde_json::Value =
        serde_json::from_str(&grammar.to_string()).expect("the grammar round-trips");
    assert_eq!(
        again["rules"],
        serde_json::to_value(&described.rules).expect("serialises")
    );
    assert_eq!(again, grammar);
}
