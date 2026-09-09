// Copyright (c) 2026 Richard Rodger and other contributors, MIT License

package tabnassemver

// The plugin's own surface: the value shape, the float64/*big.Int
// boundary, Format, Compare, and the error contract. Everything expressible
// as `input → JSON` lives in the shared fixtures (test/spec/*.tsv, run by
// both runtimes); what is here is what a fixture cannot say — big
// integers, function results, error details — mirrored case for case with
// ts/test/semver.test.ts.

import (
	"encoding/json"
	"math/big"
	"reflect"
	"strings"
	"testing"

	tabnas "github.com/tabnas/parser/go"
)

var testInst = Make()

func mustParse(t *testing.T, src string) map[string]any {
	t.Helper()
	v, err := testInst.Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("Parse(%q) = %T, want map[string]any", src, v)
	}
	return m
}

func bigOf(s string) *big.Int {
	b, _ := new(big.Int).SetString(s, 10)
	return b
}

// value builds the expected shape the way the parser does: float64 for
// safe integers, []any lists.
func value(major, minor, patch any, pre []any, build []any) map[string]any {
	if pre == nil {
		pre = []any{}
	}
	if build == nil {
		build = []any{}
	}
	return map[string]any{
		"major": major, "minor": minor, "patch": patch,
		"prerelease": pre, "build": build,
	}
}

func assertValue(t *testing.T, src string, want map[string]any) {
	t.Helper()
	got := mustParse(t, src)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse(%q)\n got %#v\nwant %#v", src, got, want)
	}
}

func TestValueShape(t *testing.T) {
	assertValue(t, "1.2.3-alpha.1+build.5",
		value(1.0, 2.0, 3.0, []any{"alpha", 1.0}, []any{"build", "5"}))
	assertValue(t, "0.0.0", value(0.0, 0.0, 0.0, nil, nil))
}

func TestPrereleaseIdentifierTypes(t *testing.T) {
	got := mustParse(t, "1.0.0-0.10.a1.1a.01a.-1")
	want := []any{0.0, 10.0, "a1", "1a", "01a", "-1"}
	if !reflect.DeepEqual(got["prerelease"], want) {
		t.Errorf("prerelease = %#v, want %#v", got["prerelease"], want)
	}
}

func TestBuildIdentifiersAreStrings(t *testing.T) {
	got := mustParse(t, "1.0.0+001.0.10")
	want := []any{"001", "0", "10"}
	if !reflect.DeepEqual(got["build"], want) {
		t.Errorf("build = %#v, want %#v", got["build"], want)
	}
}

func TestBigIntegerBoundary(t *testing.T) {
	// MAX_SAFE_INTEGER is a float64, one more is a *big.Int.
	got := mustParse(t, "9007199254740991.9007199254740992.0")
	if got["major"] != float64(9007199254740991) {
		t.Errorf("major = %#v, want float64 MAX_SAFE_INTEGER", got["major"])
	}
	if b, ok := got["minor"].(*big.Int); !ok || b.Cmp(bigOf("9007199254740992")) != 0 {
		t.Errorf("minor = %#v, want *big.Int 9007199254740992", got["minor"])
	}
	if got["patch"] != float64(0) {
		t.Errorf("patch = %#v, want 0", got["patch"])
	}
}

func TestHugeComponentsAreExact(t *testing.T) {
	cases := map[string]struct {
		key  string
		want string
	}{
		"99999999999999999999.0.0":                    {"major", "99999999999999999999"},
		"0.340282366920938463463374607431768211456.0": {"minor", "340282366920938463463374607431768211456"},
		"0.0.18446744073709551616":                    {"patch", "18446744073709551616"},
		"1.0.0-alpha.9007199254740993":                {"prerelease", "9007199254740993"},
	}
	for src, c := range cases {
		got := mustParse(t, src)
		var n any = got[c.key]
		if c.key == "prerelease" {
			n = got["prerelease"].([]any)[1]
		}
		b, ok := n.(*big.Int)
		if !ok || b.String() != c.want {
			t.Errorf("Parse(%q)[%s] = %#v, want *big.Int %s", src, c.key, n, c.want)
		}
	}
	// A huge build identifier stays a string.
	got := mustParse(t, "1.0.0+9007199254740993")
	if !reflect.DeepEqual(got["build"], []any{"9007199254740993"}) {
		t.Errorf("build = %#v, want the digits as a string", got["build"])
	}
}

func TestFormatRoundTrip(t *testing.T) {
	for _, s := range []string{
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
		// Big integers render as plain digits.
		"99999999254740993.0.0",
		"0.0.9007199254740992",
		"1.0.0-9007199254740993.x",
		"9007199254740991.9007199254740991.9007199254740991-9007199254740991+9007199254740991",
	} {
		out, err := Format(mustParse(t, s))
		if err != nil {
			t.Errorf("Format(%q): %v", s, err)
		} else if out != s {
			t.Errorf("Format(Parse(%q)) = %q", s, out)
		}
	}
}

func TestFormatHandBuilt(t *testing.T) {
	out, err := Format(value(1.0, 2.0, 3.0, []any{"rc", 1.0}, []any{"sha", "abc"}))
	if err != nil || out != "1.2.3-rc.1+sha.abc" {
		t.Errorf("Format = %q, %v", out, err)
	}
	out, err = Format(value(bigOf("1"), 0.0, 0.0, nil, nil))
	if err != nil || out != "1.0.0" {
		t.Errorf("Format(big 1) = %q, %v", out, err)
	}
	if _, err := Format("1.2.3"); err == nil {
		t.Error("Format of a string should fail")
	}
	if _, err := Format(value(1.5, 0.0, 0.0, nil, nil)); err == nil {
		t.Error("Format of a fractional component should fail")
	}
}

func cmp(t *testing.T, a, b string) int {
	t.Helper()
	c, err := Compare(mustParse(t, a), mustParse(t, b))
	if err != nil {
		t.Fatalf("Compare(%q, %q): %v", a, b, err)
	}
	return c
}

func lt(t *testing.T, a, b string) {
	t.Helper()
	if c := cmp(t, a, b); c != -1 {
		t.Errorf("Compare(%q, %q) = %d, want -1", a, b, c)
	}
	if c := cmp(t, b, a); c != 1 {
		t.Errorf("Compare(%q, %q) = %d, want 1", b, a, c)
	}
}

func eq(t *testing.T, a, b string) {
	t.Helper()
	if c := cmp(t, a, b); c != 0 {
		t.Errorf("Compare(%q, %q) = %d, want 0", a, b, c)
	}
	if c := cmp(t, b, a); c != 0 {
		t.Errorf("Compare(%q, %q) = %d, want 0", b, a, c)
	}
}

func TestCompareSpecification(t *testing.T) {
	// §11.2
	lt(t, "1.0.0", "2.0.0")
	lt(t, "2.0.0", "2.1.0")
	lt(t, "2.1.0", "2.1.1")
	lt(t, "1.9.0", "1.10.0")
	lt(t, "1.10.0", "1.11.0")
	lt(t, "9.0.0", "10.0.0")
	// §11.3
	lt(t, "1.0.0-alpha", "1.0.0")
	lt(t, "1.0.0-0", "1.0.0")
	lt(t, "1.0.0", "1.0.1-0")
	// §11.4, the specification's own chain, every pair.
	chain := []string{
		"1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta",
		"1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0",
	}
	for i := range chain {
		for j := i + 1; j < len(chain); j++ {
			lt(t, chain[i], chain[j])
		}
	}
	// §11.4.1
	lt(t, "1.0.0-2", "1.0.0-10")
	lt(t, "1.0.0-rc.9", "1.0.0-rc.10")
	// §11.4.2
	lt(t, "1.0.0-A", "1.0.0-a")
	lt(t, "1.0.0-Z", "1.0.0-a")
	lt(t, "1.0.0--", "1.0.0-0a")
	lt(t, "1.0.0-a", "1.0.0-b")
	lt(t, "1.0.0-a", "1.0.0-aa")
	lt(t, "1.0.0-alpha", "1.0.0-alpha0")
	// §11.4.3
	lt(t, "1.0.0-1", "1.0.0-a")
	lt(t, "1.0.0-9007199254740993", "1.0.0--")
	lt(t, "1.0.0-1", "1.0.0-1a")
	// §11.4.4
	lt(t, "1.0.0-alpha", "1.0.0-alpha.1")
	lt(t, "1.0.0-alpha.1", "1.0.0-alpha.1.0")
	// §10 / §11.1
	eq(t, "1.0.0", "1.0.0+build")
	eq(t, "1.0.0+a", "1.0.0+b")
	eq(t, "1.0.0-alpha+1", "1.0.0-alpha+2")
	for _, s := range []string{"0.0.0", "1.2.3-rc.1", "1.2.3+x"} {
		eq(t, s, s)
	}
}

func TestCompareBigIntegers(t *testing.T) {
	lt(t, "9007199254740991.0.0", "9007199254740992.0.0")
	lt(t, "9007199254740992.0.0", "9007199254740993.0.0")
	eq(t, "9007199254740993.0.0", "9007199254740993.0.0")
	lt(t, "1.0.0-9007199254740992", "1.0.0-9007199254740993")
	lt(t, "1.0.0-9007199254740993", "1.0.0-a")
}

func TestCompareRejectsNonVersions(t *testing.T) {
	if _, err := Compare("1.0.0", mustParse(t, "1.0.0")); err == nil {
		t.Error("Compare with a string should fail")
	}
	if _, err := Compare(mustParse(t, "1.0.0"), map[string]any{"major": "x"}); err == nil {
		t.Error("Compare with a malformed map should fail")
	}
}

func TestErrorContract(t *testing.T) {
	_, err := testInst.Parse("1.0.0-01")
	if err == nil {
		t.Fatal("1.0.0-01 should be rejected")
	}
	te, ok := err.(*tabnas.TabnasError)
	if !ok {
		t.Fatalf("error is %T, want *tabnas.TabnasError", err)
	}
	if te.Code != "unexpected" {
		t.Errorf("code = %q, want unexpected", te.Code)
	}
	if te.Row != 1 || te.Col != 9 {
		t.Errorf("position = %d:%d, want 1:9", te.Row, te.Col)
	}
	raw, jerr := json.Marshal(err)
	if jerr != nil {
		t.Fatal(jerr)
	}
	var doc map[string]any
	if jerr := json.Unmarshal(raw, &doc); jerr != nil {
		t.Fatal(jerr)
	}
	if doc["code"] != "unexpected" || doc["status"] != "failure" {
		t.Errorf("diagnostic = %v", doc)
	}
	hint, _ := doc["hint"].(string)
	if !strings.Contains(hint, "semver.org") || !strings.Contains(hint, "leading zero") {
		t.Errorf("hint should explain the format: %q", hint)
	}
}

func TestEmptyStringIsRejected(t *testing.T) {
	v, err := testInst.Parse("")
	if err == nil {
		t.Fatalf("empty string parsed to %#v", v)
	}
	if te, ok := err.(*tabnas.TabnasError); !ok || te.Code != "unexpected" {
		t.Errorf("error = %v, want unexpected", err)
	}
}

func TestErrorNamesTheCharacter(t *testing.T) {
	cases := map[string]struct {
		col int
		src string
	}{
		"v1.2.3":    {1, "v"},
		"1.2.3 ":    {6, " "},
		"1.2.3-a_b": {8, "_"},
	}
	for src, want := range cases {
		_, err := testInst.Parse(src)
		te, ok := err.(*tabnas.TabnasError)
		if !ok {
			t.Errorf("Parse(%q): %v", src, err)
			continue
		}
		if te.Col != want.col || te.Src != want.src {
			t.Errorf("Parse(%q) error at col %d src %q, want col %d src %q", src, te.Col, te.Src, want.col, want.src)
		}
	}
}

func TestInstanceIsReusable(t *testing.T) {
	a := mustParse(t, "1.0.0-a")
	if _, err := testInst.Parse("x"); err == nil {
		t.Fatal("x should be rejected")
	}
	if !reflect.DeepEqual(mustParse(t, "1.0.0-a"), a) {
		t.Error("a failed parse changed the next result")
	}
}

func TestPluginInstallsOnBareEngine(t *testing.T) {
	j := tabnas.Make()
	if err := j.Use(Semver); err != nil {
		t.Fatal(err)
	}
	v, err := j.Parse("1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if v.(map[string]any)["patch"] != float64(3) {
		t.Errorf("patch = %v", v.(map[string]any)["patch"])
	}
	// Installing twice is a no-op, not a second grammar.
	if err := j.Use(Semver); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Parse("1.2.3"); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultsAndGrammar(t *testing.T) {
	if len(Defaults) != 0 {
		t.Errorf("Defaults = %v, want none", Defaults)
	}
	for _, line := range []string{
		"semver = valid-semver",
		`valid-semver = version-core [ "-" pre-release ] [ "+" build ]`,
		"positive-digit = %x31-39",
	} {
		if !strings.Contains(Grammar, "\n"+line+"\n") {
			t.Errorf("Grammar should contain the line %q", line)
		}
	}
}

func TestConvenienceParse(t *testing.T) {
	v, err := Parse("1.2.3-rc.1+x")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v, value(1.0, 2.0, 3.0, []any{"rc", 1.0}, []any{"x"})) {
		t.Errorf("Parse = %#v", v)
	}
	if _, err := Parse("nope"); err == nil {
		t.Error("nope should be rejected")
	}
}

func TestVersionIsASemver(t *testing.T) {
	out, err := Format(mustParse(t, VERSION))
	if err != nil || out != VERSION {
		t.Errorf("VERSION %q is not a version string: %q %v", VERSION, out, err)
	}
}
