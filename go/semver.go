// Copyright (c) 2026 Richard Rodger and other contributors, MIT License

// Package tabnassemver is a tabnas plugin that parses Semantic Versioning
// 2.0.0 version strings (https://semver.org).
//
// The parser IS the specification's grammar: semver-grammar.abnf at the
// repository root (embedded below) is the semver.org BNF transcribed into
// RFC 5234 ABNF, and github.com/tabnas/abnf/go compiles it into the
// engine's rule set when the plugin is installed. Nothing here decides
// what a valid version is — the grammar accepts or rejects — and the only
// code that runs during a parse is the one action that turns the accepted
// text into the result value.
//
//	v, err := tabnassemver.Parse("1.2.3-alpha.1+build.5")
//	// map[string]any{
//	//   "major": float64(1), "minor": float64(2), "patch": float64(3),
//	//   "prerelease": []any{"alpha", float64(1)},
//	//   "build":      []any{"build", "5"},
//	// }
//
// Compare implements the specification's precedence rules (§11) over two
// parsed values, and Format renders a value back to its string.
//
// This is the Go port of the canonical TypeScript implementation in
// ts/src/semver.ts; the two must produce the same value for the same input.
package tabnassemver

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"sync"

	abnf "github.com/tabnas/abnf/go"
	tabnas "github.com/tabnas/parser/go"
)

// VERSION is this module's version. It MUST equal ts/package.json
// "version": the release orchestrator rewrites both, and
// TestVersionMatchesPackageJSON fails the build if they drift.
const VERSION = "0.0.2"

// --- BEGIN EMBEDDED semver-grammar.abnf ---
const grammarText = `
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
`

// --- END EMBEDDED semver-grammar.abnf ---

// Grammar is the grammar as ABNF text — the same text the plugin compiles.
const Grammar = grammarText

// Defaults holds the default plugin options. There are none yet; the map
// exists so UseDefaults(Semver, Defaults) reads like every other plugin
// and a future option has a declared home. Mirrors TS `Semver.defaults`.
var Defaults = map[string]any{}

// MaxSafeInteger is the largest integer a numeric component is returned
// as a float64 for; anything larger is a *big.Int. It is 2^53 - 1,
// JavaScript's Number.MAX_SAFE_INTEGER, so the two runtimes switch
// representation at the same value.
const MaxSafeInteger = 1<<53 - 1

// Semver is the tabnas plugin. Install it on a bare engine:
//
//	j := tabnas.Make()
//	err := j.Use(Semver)
//
// or use Make, which does exactly that.
func Semver(j *tabnas.Tabnas, _ map[string]any) error {
	// Guard against re-invocation on the same instance: the grammar is
	// stateless, but compiling and installing it twice is wasted work.
	if j.Decoration("semver-init") != nil {
		return nil
	}
	j.Decorate("semver-init", true)

	// Compile the specification's grammar into an engine rule set. The
	// start rule is `semver`, a pure alias of the specification's
	// `valid-semver`; see the TS source for why the one hook hangs on an
	// unhyphenated rule name.
	spec, err := abnf.Abnf(grammarText, &abnf.AbnfConvertOptions{
		Start: "semver", Tag: "semver",
	})
	if err != nil {
		return fmt.Errorf("semver: grammar failed to compile: %w", err)
	}

	// ... and then throw the tree away. Building the compiler's
	// {rule, src, kids} tree is QUADRATIC in an identifier's length here:
	// each `*`/`1*` repetition compiles to a per-character helper that
	// re-appends its child's src and re-copies its kids at every nesting
	// level, so `1.0.0-` + 16,000 letters cost 7.2 s and 9.1 GB, and
	// 32,000 was killed by the OOM killer. The plugin never reads that
	// tree — the action below builds the value from the accepted text —
	// so nothing is lost.
	//
	// This is what @tabnas/bnf's recognition mode does (TS calls the
	// library's toRecognitionSpec, which returns an installable spec);
	// the Go ToRecognitionSpec returns serialisable data rather than a
	// *GrammarSpec, so the same strip is done here on the typed spec.
	stripTreeActions(spec)

	// The single semantic action, on the compiler's end-of-source wrapper
	// — the rule abnf.Abnf names in Options.Rule.Start, normally
	// `__start__` (it numbers the name only if the grammar declares one
	// itself, which this one does not). That rule closes on `#ZZ` and
	// nothing else, so it closes exactly when the whole source has been
	// accepted: ctx.Src is then the accepted text, byte for byte — with
	// every default lexer off (below) nothing was skipped on the way in —
	// and it is the same string the discarded tree's `src` used to hold.
	// Its node is what Parse returns.
	//
	// The wrapper, not `semver`: `semver` closes as soon as a VERSION has
	// been read, which for "1.2.3f" happens before the engine discovers
	// the trailing `f`, and ctx.Src there is the whole input, `f` and all.
	startRule := "__start__"
	if spec.Options != nil && spec.Options.Rule != nil && spec.Options.Rule.Start != "" {
		startRule = spec.Options.Rule.Start
	}
	err = abnf.AttachActions(spec, abnf.ActionsMap{
		"@" + startRule + ":ac": {func(r *tabnas.Rule, ctx *tabnas.Context) {
			r.Node = fromText(ctx.Src)
		}},
	})
	if err != nil {
		return fmt.Errorf("semver: %w", err)
	}

	// The compiled spec brings its own tokens (`.`, `-`, `+`, `0` and the
	// three character classes); everything else the engine lexes by
	// default is switched off, so a character the grammar does not name —
	// a blank, a newline, a `v` prefix, a quote, a `#` — has no matcher and
	// is rejected as `unexpected` rather than skipped as whitespace or a
	// comment. The engine's default punctuation tokens go too: they are
	// JSON's, not semver's.
	off := false
	opt := spec.Options
	if opt == nil {
		opt = &tabnas.Options{}
	}
	if opt.Fixed == nil {
		opt.Fixed = &tabnas.FixedOptions{}
	}
	if opt.Fixed.Token == nil {
		opt.Fixed.Token = map[string]*string{}
	}
	for _, name := range []string{"#OB", "#CB", "#OS", "#CS", "#CL", "#CA"} {
		opt.Fixed.Token[name] = nil
	}
	opt.Space = &tabnas.SpaceOptions{Lex: &off}
	opt.Line = &tabnas.LineOptions{Lex: &off}
	opt.Comment = &tabnas.CommentOptions{Lex: &off}
	opt.String = &tabnas.StringOptions{Lex: &off}
	opt.Number = &tabnas.NumberOptions{Lex: &off}
	opt.Text = &tabnas.TextOptions{Lex: &off}
	opt.Value = &tabnas.ValueOptions{Lex: &off}
	// The empty string is not a version. By default the engine answers an
	// empty source with nil before any rule runs.
	opt.Lex = &tabnas.LexOptions{Empty: &off}
	// Every rejection is the engine's base `unexpected` code (this plugin
	// declares no codes of its own — see AGENTS.md); the hint is where a
	// reader learns what a version has to look like. Same text as the TS
	// plugin.
	opt.Hint = map[string]string{
		"unexpected": `
The character(s) {src} do not match any rule alternative active at
this position.

A Semantic Version is MAJOR.MINOR.PATCH — three integers with no
leading zeros — optionally followed by -PRERELEASE and +BUILD, each a
dot-separated list of non-empty identifiers made of [0-9A-Za-z-],
where a numeric pre-release identifier has no leading zero. Nothing
else is allowed: no whitespace, no "v" prefix, no empty identifier.
See https://semver.org/spec/v2.0.0.html`,
	}
	spec.Options = opt

	return j.Grammar(spec)
}

// Make returns a new engine with the Semver plugin installed. Build one
// and reuse it: compiling the grammar dominates a parse. The instance is
// not safe for concurrent Parse calls; see Parse for a shared one.
func Make() *tabnas.Tabnas {
	j := tabnas.Make()
	// The plugin cannot fail on the embedded grammar (the test suite
	// compiles it on every run); a failure here is a broken build.
	if err := j.UseDefaults(Semver, Defaults); err != nil {
		panic(err)
	}
	return j
}

// Cached instance behind Parse, built once. Engines are not safe for
// concurrent use, so Parse serialises callers through a mutex; the cost
// is far below rebuilding the grammar per call (the perf test pins it).
var (
	defaultOnce sync.Once
	defaultInst *tabnas.Tabnas
	defaultMu   sync.Mutex
)

// Parse parses one version string. On success the value is a
// map[string]any with the keys "major", "minor", "patch" (float64, or
// *big.Int above MaxSafeInteger), "prerelease" ([]any of string, float64
// or *big.Int — numeric identifiers are numbers) and "build" ([]any of
// string). On failure the error is the engine's *tabnas.TabnasError with
// Code "unexpected".
//
// Parse is safe for concurrent use.
func Parse(src string) (any, error) {
	defaultOnce.Do(func() { defaultInst = Make() })
	defaultMu.Lock()
	defer defaultMu.Unlock()
	return defaultInst.Parse(src)
}

// fromText builds the value from accepted text. The grammar has already
// proven the text well-formed, so every split is total and unambiguous:
// version-core contains no `-`, so the first `-` (before any `+`) opens
// the pre-release; no identifier contains `+`, so the first `+` opens the
// build metadata; and `.` separates identifiers, which never contain it.
func fromText(text string) map[string]any {
	core := text
	build := []any{}
	if plus := strings.IndexByte(core, '+'); plus >= 0 {
		for _, b := range strings.Split(core[plus+1:], ".") {
			build = append(build, b)
		}
		core = core[:plus]
	}
	prerelease := []any{}
	if dash := strings.IndexByte(core, '-'); dash >= 0 {
		for _, p := range strings.Split(core[dash+1:], ".") {
			prerelease = append(prerelease, identifier(p))
		}
		core = core[:dash]
	}
	parts := strings.Split(core, ".")
	return map[string]any{
		"major":      integer(parts[0]),
		"minor":      integer(parts[1]),
		"patch":      integer(parts[2]),
		"prerelease": prerelease,
		"build":      build,
	}
}

// identifier: a pre-release identifier is numeric when it is all digits
// (the grammar has already excluded a leading zero there), a string
// otherwise.
func identifier(text string) any {
	if isDigits(text) {
		return integer(text)
	}
	return text
}

func isDigits(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || '9' < text[i] {
			return false
		}
	}
	return len(text) > 0
}

// integer: a digit string as an exact integer — a float64 up to
// MaxSafeInteger, a *big.Int beyond.
func integer(digits string) any {
	// Sixteen decimal digits can exceed 2^53; fifteen cannot.
	if len(digits) <= 15 {
		n, _ := strconv.ParseUint(digits, 10, 64)
		return float64(n)
	}
	b, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		// Unreachable for grammar-accepted text; a zero is still an
		// honest failure mode compared to a panic.
		return float64(0)
	}
	if b.IsUint64() && b.Uint64() <= MaxSafeInteger {
		return float64(b.Uint64())
	}
	return b
}

// Format renders a parsed value back to its version string. For a value
// that came out of Parse this is the exact input text.
func Format(v any) (string, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return "", errors.New("semver: not a parsed version (want map[string]any)")
	}
	var sb strings.Builder
	for i, key := range []string{"major", "minor", "patch"} {
		s, err := numberText(m[key])
		if err != nil {
			return "", fmt.Errorf("semver: %s: %w", key, err)
		}
		if i > 0 {
			sb.WriteByte('.')
		}
		sb.WriteString(s)
	}
	pre, ok := m["prerelease"].([]any)
	if !ok {
		return "", errors.New("semver: prerelease is not a list")
	}
	for i, p := range pre {
		sb.WriteByte(map[bool]byte{true: '-', false: '.'}[i == 0])
		if s, isStr := p.(string); isStr {
			sb.WriteString(s)
		} else {
			s, err := numberText(p)
			if err != nil {
				return "", fmt.Errorf("semver: prerelease: %w", err)
			}
			sb.WriteString(s)
		}
	}
	bld, ok := m["build"].([]any)
	if !ok {
		return "", errors.New("semver: build is not a list")
	}
	for i, b := range bld {
		sb.WriteByte(map[bool]byte{true: '+', false: '.'}[i == 0])
		s, isStr := b.(string)
		if !isStr {
			return "", errors.New("semver: build identifier is not a string")
		}
		sb.WriteString(s)
	}
	return sb.String(), nil
}

// stripTreeActions removes the compiler's tree-building actions from a
// converted spec, in place: every alt action that resolves through
// spec.Ref is the emitter's own AST builder (this grammar has no other
// kind — no `bo`/`bc` hooks and no probe dispatcher), and dropping them
// leaves the rules, the tokens and therefore the accepted language
// exactly as they were. It is the typed-spec equivalent of
// bnf.ToRecognitionSpec, which returns pure data instead of a spec and
// so cannot be installed without a serialise/reload round trip.
//
// Call it BEFORE AttachActions, which adds the one action that must
// survive.
func stripTreeActions(spec *tabnas.GrammarSpec) {
	if spec == nil || len(spec.Ref) == 0 {
		return
	}
	// The same drop set bnf.ToRecognitionSpec uses: an action that
	// resolves through spec.Ref (closure conversion, which is what this
	// plugin does) or names one of the engine's tree-building builtins
	// (a `Builtins: true` conversion, which this plugin does not do —
	// listed so the strip stays correct if it ever starts to).
	treeBuiltin := map[string]bool{
		"@node$": true, "@capture$": true, "@bubble$": true, "@fold$": true,
	}
	isRef := func(v any) bool {
		s, ok := v.(string)
		if !ok {
			return false
		}
		if treeBuiltin[s] {
			return true
		}
		_, found := spec.Ref[tabnas.FuncRef(s)]
		return found
	}
	keep := func(v any) any {
		switch a := v.(type) {
		case nil:
			return nil
		case string:
			if isRef(a) {
				return nil
			}
		case []any:
			out := make([]any, 0, len(a))
			for _, e := range a {
				if !isRef(e) {
					out = append(out, e)
				}
			}
			if len(out) == 0 {
				return nil
			}
			return out
		}
		return v
	}
	for _, rule := range spec.Rule {
		if rule == nil {
			continue
		}
		for _, alts := range [][]*tabnas.GrammarAltSpec{
			specAlts(rule.Open), specAlts(rule.Close),
		} {
			for _, alt := range alts {
				if alt == nil {
					continue
				}
				alt.A = keep(alt.A)
				// The dropped builtins' per-alt configuration goes with
				// them; anything else under K is the grammar's own.
				for _, k := range []string{"node$", "capture$", "fold$"} {
					delete(alt.K, k)
				}
			}
		}
	}
	spec.Ref = nil
}

// specAlts reads a rule-spec phase in either shape the engine accepts.
func specAlts(state any) []*tabnas.GrammarAltSpec {
	switch v := state.(type) {
	case []*tabnas.GrammarAltSpec:
		return v
	case *tabnas.GrammarAltListSpec:
		if v != nil {
			return v.Alts
		}
	}
	return nil
}

func numberText(n any) (string, error) {
	switch x := n.(type) {
	case float64:
		// The finite test comes first: +Inf survives both of the others
		// (Trunc(+Inf) is +Inf, and +Inf < 0 is false) and would render
		// as "+Inf".
		if err := finite(x); err != nil {
			return "", err
		}
		if x != math.Trunc(x) || x < 0 {
			return "", fmt.Errorf("not a non-negative integer: %v", x)
		}
		return strconv.FormatFloat(x, 'f', -1, 64), nil
	case *big.Int:
		if x == nil || x.Sign() < 0 {
			return "", errors.New("not a non-negative integer")
		}
		return x.String(), nil
	}
	return "", fmt.Errorf("not a number: %T", n)
}

// Compare orders two parsed values by precedence, as the specification
// defines it (§11): -1 when a ranks below b, 1 when above, 0 when they
// are the same version. Build metadata is ignored (§10, §11.1):
// 1.0.0+a and 1.0.0+b compare equal.
func Compare(a, b any) (int, error) {
	am, ok := a.(map[string]any)
	if !ok {
		return 0, errors.New("semver: not a parsed version (want map[string]any)")
	}
	bm, ok := b.(map[string]any)
	if !ok {
		return 0, errors.New("semver: not a parsed version (want map[string]any)")
	}
	for _, key := range []string{"major", "minor", "patch"} {
		c, err := compareNumber(am[key], bm[key])
		if err != nil {
			return 0, fmt.Errorf("semver: %s: %w", key, err)
		}
		if c != 0 {
			return c, nil
		}
	}
	ap, ok := am["prerelease"].([]any)
	if !ok {
		return 0, errors.New("semver: prerelease is not a list")
	}
	bp, ok := bm["prerelease"].([]any)
	if !ok {
		return 0, errors.New("semver: prerelease is not a list")
	}
	return comparePrerelease(ap, bp)
}

// §11.2: numerically. Every value is an exact integer, so a float64 and a
// *big.Int compare exactly once both are big.
func compareNumber(x, y any) (int, error) {
	xf, xOK := x.(float64)
	yf, yOK := y.(float64)
	if xOK && yOK {
		// The fast path still has to reject what a parse cannot produce:
		// every comparison with a NaN is false, so without this it would
		// report two versions equal, and an infinity would order against
		// a real version instead of failing as it does on the *big.Int
		// path below.
		if err := finite(xf); err != nil {
			return 0, err
		}
		if err := finite(yf); err != nil {
			return 0, err
		}
		switch {
		case xf < yf:
			return -1, nil
		case xf > yf:
			return 1, nil
		}
		return 0, nil
	}
	xb, err := toBig(x)
	if err != nil {
		return 0, err
	}
	yb, err := toBig(y)
	if err != nil {
		return 0, err
	}
	return xb.Cmp(yb), nil
}

// finite rejects the float64 values no parse produces: an infinity
// (which passes an integer test, since Trunc(+Inf) is +Inf) and a NaN
// (which compares false against everything, itself included).
func finite(x float64) error {
	if math.IsInf(x, 0) || math.IsNaN(x) {
		return fmt.Errorf("not a finite number: %v", x)
	}
	return nil
}

func toBig(n any) (*big.Int, error) {
	switch x := n.(type) {
	case float64:
		// Without the finite test an infinity reached big.Float.Int,
		// which returns nil for it, and the caller's Cmp dereferenced
		// that nil.
		if err := finite(x); err != nil {
			return nil, err
		}
		if x != math.Trunc(x) {
			return nil, fmt.Errorf("not an integer: %v", x)
		}
		b, _ := new(big.Float).SetFloat64(x).Int(nil)
		return b, nil
	case *big.Int:
		if x == nil {
			return nil, errors.New("nil *big.Int")
		}
		return x, nil
	}
	return nil, fmt.Errorf("not a number: %T", n)
}

// §11.3: a pre-release version ranks below the associated normal version.
// §11.4: otherwise identifier by identifier, left to right, and a larger
// set of identifiers ranks above a smaller one when every preceding
// identifier is equal.
func comparePrerelease(x, y []any) (int, error) {
	if len(x) == 0 && len(y) == 0 {
		return 0, nil
	}
	if len(x) == 0 {
		return 1, nil
	}
	if len(y) == 0 {
		return -1, nil
	}
	n := len(x)
	if len(y) < n {
		n = len(y)
	}
	for i := 0; i < n; i++ {
		c, err := compareIdentifier(x[i], y[i])
		if err != nil {
			return 0, fmt.Errorf("semver: prerelease: %w", err)
		}
		if c != 0 {
			return c, nil
		}
	}
	switch {
	case len(x) < len(y):
		return -1, nil
	case len(x) > len(y):
		return 1, nil
	}
	return 0, nil
}

// §11.4.1: numeric identifiers numerically. §11.4.2: alphanumeric
// identifiers lexically in ASCII sort order (a Go string comparison,
// since identifiers are ASCII). §11.4.3: a numeric identifier always
// ranks below an alphanumeric one.
func compareIdentifier(p, q any) (int, error) {
	ps, pStr := p.(string)
	qs, qStr := q.(string)
	switch {
	case !pStr && !qStr:
		return compareNumber(p, q)
	case !pStr:
		return -1, nil
	case !qStr:
		return 1, nil
	}
	return strings.Compare(ps, qs), nil
}
