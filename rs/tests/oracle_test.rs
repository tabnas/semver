// Conformance against the specification's own oracle.
//
// semver.org publishes a regular expression that recognises exactly the
// language of its grammar (FAQ: "Is there a suggested regular expression
// (RegEx) to check a SemVer string?"). It is a different formalism from
// the ABNF this plugin compiles, written by different people, and its
// captures name the same five parts the plugin's value carries, which is
// what makes it the judge here. On every string of the corpus below the
// plugin's VERDICT must equal the expression's, and on every accepted
// string the plugin's VALUE must match the expression's captures and
// `format` must give the input back.
//
// The corpus is generated, not committed, and is IDENTICAL to the ones in
// `ts/test/oracle.test.ts` and `go/oracle_test.go`: the same alphabets,
// the same enumeration order, the same xorshift32 stream from the same
// seed, the same mutation operators in the same order. A pinned census
// (how many strings each section accepts) and a pinned FNV-1a hash over
// the whole corpus make sure every runtime graded the same strings.
// Changing the corpus means changing both constants, in all three
// runtimes, in the same commit.

use std::collections::BTreeMap;

use regex::Regex;
use tabnas::{Tabnas, Value};

// semver.org, FAQ -- the numbered-capture-group form. Group 1..3 are the
// version core, 4 the pre-release (without the `-`), 5 the build metadata
// (without the `+`).
//
// Every `\d` of the published expression is written `[0-9]` here. The
// `regex` crate makes `\d` Unicode-aware, so it would also match the
// Arabic-Indic and Devanagari digits, where the JavaScript and RE2 forms
// this is transcribed from match ASCII only. Nothing in the corpus is
// non-ASCII, so the two agree on every string graded; the class is
// spelled out so the judge cannot quietly become a different judge.
const ORACLE: &str = concat!(
    r"^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)",
    r"(?:-((?:0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)",
    r"(?:\.(?:0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*))*))?",
    r"(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$",
);

// ---- the corpus (keep byte-identical with the other two runtimes) ------

const SHORT: &str = "019aZ-.+";
const SHORT_MAX: usize = 5;
const PRE: &str = "01a.";
const BLD: &str = "0a.";
const TAIL_MAX: usize = 3;
const WIDE: &str = "019aZ-.+v _xB250\t/:";
const MUTATIONS: usize = 3000;
const RANDOMS: usize = 1000;
const SEED: u32 = 0x9e37_79b9;

const CORES: [&str; 5] = ["0.0.0", "1.2.3", "01.0.0", "1.0", "1"];

const SEEDS: [&str; 12] = [
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
    "10.20.30-rc.1+build.2",
];

/// The pinned census. A change here is a change to the corpus, and needs
/// the same change in `ts/test/oracle.test.ts` and `go/oracle_test.go`.
const CENSUS: [(&str, usize, usize); 4] = [
    ("exhaustive", 37449, 27),
    ("structured", 17000, 1634),
    ("mutation", 3000, 838),
    ("random", 1000, 0),
];

const CORPUS_HASH: u32 = 0x97bd_27cb;

/// The section names in corpus order, which is the order the hash covers.
const SECTIONS: [&str; 4] = ["exhaustive", "structured", "mutation", "random"];

/// Every string of `alphabet` of exactly `n` characters, in base-n
/// counting order (most significant position first), appended to `out`.
fn strings_of_length(out: &mut Vec<String>, alphabet: &[u8], n: usize) {
    let total = alphabet.len().pow(n as u32);
    let mut buf = vec![0u8; n];
    for index in 0..total {
        let mut rest = index;
        for slot in (0..n).rev() {
            buf[slot] = alphabet[rest % alphabet.len()];
            rest /= alphabet.len();
        }
        out.push(String::from_utf8(buf.clone()).expect("the alphabets are ASCII"));
    }
}

fn up_to(alphabet: &str, min: usize, max: usize) -> Vec<String> {
    let bytes = alphabet.as_bytes();
    let mut out = Vec::new();
    for n in min..=max {
        strings_of_length(&mut out, bytes, n);
    }
    out
}

/// xorshift32, the same stream in every runtime.
struct Rng {
    x: u32,
}

impl Rng {
    fn next(&mut self) -> u32 {
        let mut x = self.x;
        x ^= x << 13;
        x ^= x >> 17;
        x ^= x << 5;
        self.x = x;
        x
    }

    fn below(&mut self, n: usize) -> usize {
        (self.next() % n as u32) as usize
    }

    fn pick(&mut self, alphabet: &str) -> u8 {
        let bytes = alphabet.as_bytes();
        bytes[self.below(bytes.len())]
    }
}

/// One random edit. Every branch consumes the stream in the same order as
/// its TypeScript and Go twins, including the branches that do nothing to
/// an empty string. Every alphabet is ASCII, so byte slicing is character
/// slicing.
fn mutate(s: &str, rng: &mut Rng) -> String {
    let op = rng.below(5);
    let n = s.len();
    match op {
        0 => {
            let pos = rng.below(n + 1);
            let c = rng.pick(WIDE) as char;
            std::format!("{}{}{}", &s[..pos], c, &s[pos..])
        }
        1 => {
            if n == 0 {
                return s.to_string();
            }
            let pos = rng.below(n);
            std::format!("{}{}", &s[..pos], &s[pos + 1..])
        }
        2 => {
            if n == 0 {
                return s.to_string();
            }
            let pos = rng.below(n);
            let c = rng.pick(WIDE) as char;
            std::format!("{}{}{}", &s[..pos], c, &s[pos + 1..])
        }
        3 => {
            if n == 0 {
                return s.to_string();
            }
            let i = rng.below(n);
            let j = i + 1 + rng.below(n - i);
            let k = rng.below(n + 1);
            std::format!("{}{}{}", &s[..k], &s[i..j], &s[k..])
        }
        _ => {
            let sep = ".-+".as_bytes()[rng.below(3)] as char;
            let count = 1 + rng.below(3);
            let mut segment = String::new();
            for _ in 0..count {
                segment.push(rng.pick(SHORT) as char);
            }
            std::format!("{s}{sep}{segment}")
        }
    }
}

fn corpus() -> BTreeMap<&'static str, Vec<String>> {
    let mut exhaustive = vec![String::new()];
    exhaustive.extend(up_to(SHORT, 1, SHORT_MAX));

    let mut structured = Vec::new();
    let pres = up_to(PRE, 0, TAIL_MAX);
    let blds = up_to(BLD, 0, TAIL_MAX);
    for core in CORES {
        for x in &pres {
            for y in &blds {
                let mut s = core.to_string();
                if !x.is_empty() {
                    s.push('-');
                    s.push_str(x);
                }
                if !y.is_empty() {
                    s.push('+');
                    s.push_str(y);
                }
                structured.push(s);
            }
        }
    }

    let mut rng = Rng { x: SEED };
    let mut mutation = Vec::with_capacity(MUTATIONS);
    for _ in 0..MUTATIONS {
        let mut s = SEEDS[rng.below(SEEDS.len())].to_string();
        let edits = 1 + rng.below(3);
        for _ in 0..edits {
            s = mutate(&s, &mut rng);
        }
        mutation.push(s);
    }

    let mut random = Vec::with_capacity(RANDOMS);
    for _ in 0..RANDOMS {
        let n = 1 + rng.below(12);
        let mut s = String::with_capacity(n);
        for _ in 0..n {
            s.push(rng.pick(WIDE) as char);
        }
        random.push(s);
    }

    let mut sections = BTreeMap::new();
    sections.insert("exhaustive", exhaustive);
    sections.insert("structured", structured);
    sections.insert("mutation", mutation);
    sections.insert("random", random);
    sections
}

/// FNV-1a, 32-bit, over the bytes of every string in corpus order, each
/// followed by a newline byte.
fn fnv1a(sections: &BTreeMap<&'static str, Vec<String>>) -> u32 {
    let mut hash: u32 = 0x811c_9dc5;
    for name in SECTIONS {
        for s in &sections[name] {
            for byte in s.as_bytes() {
                hash = (hash ^ u32::from(*byte)).wrapping_mul(0x0100_0193);
            }
            hash = (hash ^ 0x0a).wrapping_mul(0x0100_0193);
        }
    }
    hash
}

// ---- the judge ---------------------------------------------------------

fn is_digits(s: &str) -> bool {
    !s.is_empty() && s.bytes().all(|b| b.is_ascii_digit())
}

/// The decimal text of a component, and whether it is a NUMBER rather
/// than an alphanumeric identifier.
///
/// This port represents an integer past 2^53 as its exact decimal digits
/// in a `Value::String`, so a string of digits is numeric here where a
/// string in TypeScript or Go never is. No component of this corpus comes
/// near that boundary (the longest is five digits), so the two readings
/// cannot differ on anything graded below.
fn component(value: &Value) -> (String, bool) {
    match value {
        Value::String(text) => (text.clone(), is_digits(text)),
        Value::Number(number) => (std::format!("{number:.0}"), true),
        other => (std::format!("{other}"), false),
    }
}

fn field(value: &Value, key: &str) -> Value {
    match value {
        Value::Object(fields) => fields.get(key).cloned().unwrap_or(Value::Undefined),
        _ => Value::Undefined,
    }
}

fn list(value: &Value, key: &str) -> Vec<Value> {
    match field(value, key) {
        Value::Array(values) => values.to_vec(),
        _ => Vec::new(),
    }
}

/// Grade one string; return a description of the disagreement, or `None`.
fn grade(parser: &Tabnas, oracle: &Regex, s: &str) -> Option<String> {
    let captures = oracle.captures(s);
    let parsed = parser.parse(s);

    let Some(captures) = captures else {
        return match parsed {
            Ok(_) => Some(std::format!("accepted, oracle rejects: {s:?}")),
            Err(error) if error.code != "unexpected" => Some(std::format!(
                "rejected with {}, not unexpected: {s:?}",
                error.code
            )),
            Err(_) => None,
        };
    };

    let value = match parsed {
        Ok(value) => value,
        Err(error) => {
            return Some(std::format!(
                "rejected ({}), oracle accepts: {s:?}",
                error.code
            ))
        }
    };

    match tabnas_semver::format(&value) {
        Ok(out) if out == s => {}
        Ok(out) => return Some(std::format!("format gave {out:?} for {s:?}")),
        Err(error) => return Some(std::format!("format failed ({error}) for {s:?}")),
    }

    for (index, key) in ["major", "minor", "patch"].into_iter().enumerate() {
        let (text, is_number) = component(&field(&value, key));
        let want = captures.get(index + 1).map_or("", |m| m.as_str());
        if !is_number || text != want {
            return Some(std::format!("{key} {text:?} != {want:?} in {s:?}"));
        }
    }

    let want_pre: Vec<&str> = match captures.get(4) {
        Some(m) if !m.as_str().is_empty() => m.as_str().split('.').collect(),
        _ => Vec::new(),
    };
    let got_pre = list(&value, "prerelease");
    if got_pre.len() != want_pre.len() {
        return Some(std::format!("prerelease length in {s:?}"));
    }
    for (index, want) in want_pre.iter().enumerate() {
        let (text, is_number) = component(&got_pre[index]);
        if text != *want {
            return Some(std::format!(
                "prerelease[{index}] {text:?} != {want:?} in {s:?}"
            ));
        }
        if is_number != is_digits(want) {
            return Some(std::format!(
                "prerelease[{index}] {want:?} has the wrong type in {s:?}"
            ));
        }
    }

    let want_build: Vec<&str> = match captures.get(5) {
        Some(m) if !m.as_str().is_empty() => m.as_str().split('.').collect(),
        _ => Vec::new(),
    };
    let got_build = list(&value, "build");
    if got_build.len() != want_build.len() {
        return Some(std::format!("build length in {s:?}"));
    }
    for (index, want) in want_build.iter().enumerate() {
        match &got_build[index] {
            Value::String(text) if text == want => {}
            other => {
                return Some(std::format!(
                    "build[{index}] {other:?} != {want:?} in {s:?}"
                ))
            }
        }
    }

    None
}

#[test]
fn the_corpus_is_pinned() {
    let sections = corpus();
    for (name, total, _) in CENSUS {
        assert_eq!(sections[name].len(), total, "{name} section size");
    }
    let hash = fnv1a(&sections);
    assert_eq!(
        hash, CORPUS_HASH,
        "corpus hash {hash:#x}, want {CORPUS_HASH:#x} -- the generator changed; \
         re-pin the hash and census here, in ts/test/oracle.test.ts and in \
         go/oracle_test.go"
    );
}

#[test]
fn the_plugin_agrees_with_the_oracle() {
    let oracle = Regex::new(ORACLE).expect("the published expression compiles");
    let sections = corpus();
    let parser = tabnas_semver::make();

    for (name, _, want_accepted) in CENSUS {
        let mut failures: Vec<String> = Vec::new();
        let mut accepted = 0usize;
        for s in &sections[name] {
            match grade(&parser, &oracle, s) {
                Some(why) => {
                    if failures.len() < 20 {
                        failures.push(why);
                    }
                }
                None => {
                    if oracle.is_match(s) {
                        accepted += 1;
                    }
                }
            }
        }
        assert!(
            failures.is_empty(),
            "disagreements with the oracle in {name} (first {}):\n  {}",
            failures.len(),
            failures.join("\n  ")
        );
        assert_eq!(
            accepted, want_accepted,
            "{name} accepted {accepted}; the census pins {want_accepted}"
        );
    }
}
