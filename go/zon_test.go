/* Copyright (c) 2025 Richard Rodger, MIT License */

package tabnaszon

import (
	"math"
	"math/big"
	"reflect"
	"strings"
	"testing"

	jsonic "github.com/tabnas/jsonic/go"
)

// mustZon builds a jsonic instance with the Zon plugin installed, failing the
// test if the plugin does not install.
//
// UseDefaults returns an error and both helpers below used to DISCARD it. That
// hid a whole class of failure: with the plugin not installed, `j` is left as
// bare jsonic, and bare jsonic already parses `42`, `3.14`, `true`, `null` and
// `"hello"` — so a chunk of this suite would still pass while exercising none
// of this plugin's behaviour.
func mustZon(t *testing.T, opts ...map[string]any) *jsonic.Jsonic {
	t.Helper()
	j := jsonic.Make()
	if err := j.UseDefaults(Zon, Defaults, opts...); err != nil {
		t.Fatalf("Zon plugin failed to install: %v", err)
	}
	return j
}

// parse creates a jsonic instance with the Zon plugin and parses src.
func parse(t *testing.T, src string, opts ...map[string]any) any {
	t.Helper()
	result, err := mustZon(t, opts...).Parse(src)
	if err != nil {
		t.Fatalf("parse(%q) unexpected error: %v", src, err)
	}
	return result
}

func parseErr(t *testing.T, src string, opts ...map[string]any) error {
	t.Helper()
	_, err := mustZon(t, opts...).Parse(src)
	return err
}

// normalize recursively converts parsed *OrderedMap nodes into plain
// map[string]any so value comparisons are order-agnostic. The parser now
// returns insertion-ordered maps; these tests assert values/structure, not
// key order, so both sides are flattened before reflect.DeepEqual.
func normalize(v any) any {
	switch x := v.(type) {
	case *jsonic.OrderedMap:
		m := make(map[string]any, x.Len())
		for _, k := range x.Keys {
			m[k] = normalize(x.Vals[k])
		}
		return m
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, val := range x {
			m[k] = normalize(val)
		}
		return m
	case []any:
		s := make([]any, len(x))
		for i, val := range x {
			s[i] = normalize(val)
		}
		return s
	default:
		return v
	}
}

func assertEqual(t *testing.T, label string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(normalize(got), normalize(want)) {
		t.Errorf("%s: got %#v, want %#v", label, got, want)
	}
}

func TestScalars(t *testing.T) {
	assertEqual(t, "int", parse(t, "42"), float64(42))
	assertEqual(t, "float", parse(t, "3.14"), float64(3.14))
	assertEqual(t, "true", parse(t, "true"), true)
	assertEqual(t, "false", parse(t, "false"), false)
	assertEqual(t, "null", parse(t, "null"), nil)
	assertEqual(t, "string", parse(t, `"hello"`), "hello")
}

func TestNumericBases(t *testing.T) {
	assertEqual(t, "hex", parse(t, "0x2a"), float64(42))
	assertEqual(t, "oct", parse(t, "0o52"), float64(42))
	assertEqual(t, "bin", parse(t, "0b101010"), float64(42))
	assertEqual(t, "sep", parse(t, "1_000_000"), float64(1000000))
}

func TestEnumLiteralAsValue(t *testing.T) {
	assertEqual(t, "single", parse(t, ".foo"), "foo")
	assertEqual(t, "snake_case", parse(t, ".bar_baz"), "bar_baz")
}

func TestEmptyStruct(t *testing.T) {
	got := parse(t, ".{}")
	if !reflect.DeepEqual(got, []any{}) {
		t.Errorf("empty .{} got %#v, want []any{}", got)
	}
}

func TestSimpleStruct(t *testing.T) {
	assertEqual(t, "single", parse(t, ".{ .a = 1 }"),
		map[string]any{"a": float64(1)})
	assertEqual(t, "two", parse(t, ".{ .a = 1, .b = 2 }"),
		map[string]any{"a": float64(1), "b": float64(2)})
}

func TestTrailingCommaStruct(t *testing.T) {
	assertEqual(t, "single", parse(t, ".{ .a = 1, }"),
		map[string]any{"a": float64(1)})
	assertEqual(t, "two", parse(t, ".{ .a = 1, .b = 2, }"),
		map[string]any{"a": float64(1), "b": float64(2)})
}

func TestTuple(t *testing.T) {
	assertEqual(t, "numbers", parse(t, ".{ 1, 2, 3 }"),
		[]any{float64(1), float64(2), float64(3)})
	assertEqual(t, "strings", parse(t, `.{ "a", "b" }`),
		[]any{"a", "b"})
}

func TestTrailingCommaTuple(t *testing.T) {
	assertEqual(t, "trailing", parse(t, ".{ 1, 2, 3, }"),
		[]any{float64(1), float64(2), float64(3)})
}

func TestNestedStruct(t *testing.T) {
	assertEqual(t, "nested", parse(t, ".{ .a = .{ .b = 1 } }"),
		map[string]any{"a": map[string]any{"b": float64(1)}})
}

func TestNestedTuple(t *testing.T) {
	assertEqual(t, "nested",
		parse(t, ".{ .{ 1, 2 }, .{ 3, 4 } }"),
		[]any{
			[]any{float64(1), float64(2)},
			[]any{float64(3), float64(4)},
		})
}

func TestMixedNesting(t *testing.T) {
	got := parse(t, ".{ .xs = .{ 1, 2, 3 }, .y = .{ .z = true } }")
	want := map[string]any{
		"xs": []any{float64(1), float64(2), float64(3)},
		"y":  map[string]any{"z": true},
	}
	assertEqual(t, "mixed", got, want)
}

func TestStringEscapes(t *testing.T) {
	assertEqual(t, "newline", parse(t, `"a\nb"`), "a\nb")
	assertEqual(t, "tab", parse(t, `"a\tb"`), "a\tb")
	assertEqual(t, "backslash", parse(t, `"a\\b"`), `a\b`)
}

func TestEnumAsFieldValue(t *testing.T) {
	assertEqual(t, "field", parse(t, ".{ .kind = .red }"),
		map[string]any{"kind": "red"})
}

func TestComments(t *testing.T) {
	src := `.{
		// a comment
		.name = "x", // trailing comment
		.version = "1.0", // version
	}`
	got := parse(t, src)
	want := map[string]any{"name": "x", "version": "1.0"}
	assertEqual(t, "with comments", got, want)
}

func TestRealisticZon(t *testing.T) {
	src := `.{
		.name = "example",
		.version = "0.0.1",
		.minimum_zig_version = "0.14.0",
		.dependencies = .{
			.foo = .{
				.url = "https://example.com/foo.tar.gz",
				.hash = "1220deadbeef",
			},
		},
		.paths = .{
			"build.zig",
			"src",
			"",
		},
	}`
	got := parse(t, src)
	want := map[string]any{
		"name":                "example",
		"version":             "0.0.1",
		"minimum_zig_version": "0.14.0",
		"dependencies": map[string]any{
			"foo": map[string]any{
				"url":  "https://example.com/foo.tar.gz",
				"hash": "1220deadbeef",
			},
		},
		"paths": []any{"build.zig", "src", ""},
	}
	assertEqual(t, "build.zig.zon", got, want)
}

func TestCharLiteralAsNumber(t *testing.T) {
	assertEqual(t, "A", parse(t, "'A'", map[string]any{"charAsNumber": true}), float64(65))
	assertEqual(t, "n", parse(t, `'\n'`, map[string]any{"charAsNumber": true}), float64(10))
	assertEqual(t, "emoji",
		parse(t, `'\u{1F600}'`, map[string]any{"charAsNumber": true}),
		float64(0x1F600))
}

func TestCharLiteralAsString(t *testing.T) {
	assertEqual(t, "A", parse(t, "'A'"), "A")
}

func TestMultiLineString(t *testing.T) {
	src := ".{\n" +
		"\t.text = \\\\hello\n" +
		"\t\t\\\\world\n" +
		"\t,\n" +
		"}"
	got := parse(t, src)
	want := map[string]any{"text": "hello\nworld"}
	assertEqual(t, "multi-line", got, want)
}

func TestEnumTagOption(t *testing.T) {
	got := parse(t, ".{ .kind = .red }", map[string]any{"enumTag": "$enum"})
	want := map[string]any{"kind": map[string]any{"$enum": "red"}}
	assertEqual(t, "enum-tagged", got, want)
}

func TestSyntaxError(t *testing.T) {
	// `{` without `.` is not valid ZON.
	if err := parseErr(t, "{ a = 1 }"); err == nil {
		t.Error("expected error for bare { but got none")
	}
}

// The cases below cannot live in `test/spec/*.tsv`: the shared fixtures
// compare after a JSON round-trip, and big.Int / Inf / NaN / -0 have no JSON
// spelling. Mirrors the equivalent block in ts/test/zon.test.ts.

func bigOf(t *testing.T, s string) *big.Int {
	t.Helper()
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		t.Fatalf("bad big literal %q", s)
	}
	return v
}

func TestBigIntegersKeepExactValue(t *testing.T) {
	// 2^65 - 1 is not representable as an IEEE-754 double, so the plugin
	// returns a *big.Int rather than silently rounding (zig 0.16.0 reports
	// the same exact value).
	want := bigOf(t, "36893488147419103231")
	for _, src := range []string{
		"36893488147419103231",
		"368934_881_474191032_31",
		"0x1ffffffffffffffff",
		"0o3777777777777777777777",
		"0b" + strings.Repeat("1", 65),
	} {
		got, ok := parse(t, src).(*big.Int)
		if !ok || got.Cmp(want) != 0 {
			t.Errorf("parse(%q) = %v, want *big.Int %v", src, parse(t, src), want)
		}
	}
	neg := bigOf(t, "-36893488147419103231")
	if got, ok := parse(t, "-36893488147419103231").(*big.Int); !ok || got.Cmp(neg) != 0 {
		t.Errorf("parse(-2^65+1) = %v, want %v", parse(t, "-36893488147419103231"), neg)
	}
	if got, ok := parse(t, "9007199254740993").(*big.Int); !ok ||
		got.Cmp(bigOf(t, "9007199254740993")) != 0 {
		t.Errorf("2^53+1 should be exact, got %v", parse(t, "9007199254740993"))
	}
}

func TestIntegersThatFitADoubleStayFloat64(t *testing.T) {
	cases := map[string]float64{
		"9007199254740992":      9007199254740992,
		"18446744073709551616":  18446744073709551616,
		"-36893488147419103232": -36893488147419103232,
	}
	for src, want := range cases {
		got, ok := parse(t, src).(float64)
		if !ok || got != want {
			t.Errorf("parse(%q) = %#v, want float64 %v", src, parse(t, src), want)
		}
	}
}

func TestInfAndNanLiterals(t *testing.T) {
	if got := parse(t, "inf").(float64); !math.IsInf(got, 1) {
		t.Errorf("inf = %v", got)
	}
	if got := parse(t, "-inf").(float64); !math.IsInf(got, -1) {
		t.Errorf("-inf = %v", got)
	}
	if got := parse(t, "- inf").(float64); !math.IsInf(got, -1) {
		t.Errorf("- inf = %v", got)
	}
	if got := parse(t, "nan").(float64); !math.IsNaN(got) {
		t.Errorf("nan = %v", got)
	}
	// `-nan` is not a Zig numeric literal.
	if err := parseErr(t, "-nan"); err == nil {
		t.Error("expected -nan to be rejected")
	}
}

func TestNegativeZero(t *testing.T) {
	for _, src := range []string{"-0.0", "-0e0"} {
		got := parse(t, src).(float64)
		if got != 0 || !math.Signbit(got) {
			t.Errorf("parse(%q) = %v, want -0", src, got)
		}
	}
	// But an integer `-0` is ambiguous in Zig and rejected.
	if err := parseErr(t, "-0"); err == nil {
		t.Error("expected -0 to be rejected")
	}
}

func TestDuplicateStructFieldRejected(t *testing.T) {
	for _, src := range []string{`.{ .a = 1, .a = 2 }`, `.{ .@"a" = 1, .a = 2 }`} {
		err := parseErr(t, src)
		if err == nil {
			t.Fatalf("expected %q to be rejected", src)
		}
		if !strings.Contains(err.Error(), "duplicate struct field") {
			t.Errorf("%q: unexpected message %q", src, err.Error())
		}
	}
	// Sibling structs may of course repeat a name.
	got := parse(t, `.{ .x = .{ .a = 1 }, .y = .{ .a = 2 } }`)
	want := map[string]any{
		"x": map[string]any{"a": 1.0},
		"y": map[string]any{"a": 2.0},
	}
	assertEqual(t, "sibling-dup", got, want)
}

func TestDocCommentsRejected(t *testing.T) {
	for _, src := range []string{"//! doc\n1", "/// doc\n1"} {
		err := parseErr(t, src)
		if err == nil {
			t.Fatalf("expected %q to be rejected", src)
		}
		if !strings.Contains(err.Error(), "doc comments") {
			t.Errorf("%q: unexpected message %q", src, err.Error())
		}
	}
	// Four or more slashes is an ordinary line comment again.
	for _, src := range []string{"//// four\n1", "// two\n1"} {
		if got := parse(t, src); got != 1.0 {
			t.Errorf("parse(%q) = %v, want 1", src, got)
		}
	}
}
