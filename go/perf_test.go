/* Copyright (c) 2026 Richard Rodger, MIT License */

package tabnassemver

import (
	"strings"
	"testing"
	"time"
)

// TestParseReusesInstance guards against a performance regression where the
// convenience Parse() rebuilds the grammar on every call instead of reusing
// a cached instance. Compiling the ABNF and installing the rule set
// dominates a parse by orders of magnitude, so a rebuild-per-call Parse()
// is many times slower than reusing one Make() instance.
//
// The check is machine-INDEPENDENT: it compares Parse() against instance
// reuse on the SAME machine in the SAME run, so a slow CI box cannot make it
// flaky (both sides scale together). There is deliberately NO wall-clock
// budget.
func TestParseReusesInstance(t *testing.T) {
	const src = "1.2.3-alpha.1+build.5"
	const n = 3000

	// Warm both paths so the comparison is steady-state.
	for i := 0; i < 100; i++ {
		_, _ = Parse(src)
	}
	j := Make()
	for i := 0; i < 100; i++ {
		_, _ = j.Parse(src)
	}

	t0 := time.Now()
	for i := 0; i < n; i++ {
		if _, err := Parse(src); err != nil {
			t.Fatalf("Parse error: %v", err)
		}
	}
	conv := time.Since(t0)

	t1 := time.Now()
	for i := 0; i < n; i++ {
		if _, err := j.Parse(src); err != nil {
			t.Fatalf("reuse parse error: %v", err)
		}
	}
	reuse := time.Since(t1)

	// A cached Parse() is ~= instance reuse; allow 4x for scheduling noise
	// and the mutex. A rebuild-per-call Parse() is many times slower here,
	// so this catches the regression without depending on absolute
	// wall-clock speed.
	if conv > 4*reuse {
		t.Errorf("Parse() appears to rebuild the grammar on every call: "+
			"%d Parse() calls took %v vs %v reusing one instance (ratio %.1fx, limit 4x). "+
			"Cache a lazy default instance (see Parse / sync.Once).",
			n, conv, reuse, float64(conv)/float64(reuse))
	}
	t.Logf("Parse()=%v  reuse=%v  ratio=%.2fx", conv, reuse, float64(conv)/float64(reuse))
}

// TestLongIdentifierSurvives: a long identifier is VALID. The
// specification bounds neither the length of a pre-release or build
// identifier nor the number of them, and the regular expression
// semver.org publishes accepts every string below. They arrive from lock
// files, tags and HTTP headers, so they are attacker-chosen text (see
// AGENTS.md, "Untrusted input").
//
// While the plugin asked the compiler for the {rule, src, kids} tree it
// never read, each of these was QUADRATIC in the identifier's length —
// the per-character helper chain re-appended its child's src and
// re-copied its kids at every level. "1.0.0-" + 16,000 letters took 7.2 s
// and 9.1 GB; 32,000 was killed by the kernel OOM killer, which no
// error return can catch. This test is that crash: it does not measure
// time, it measures survival.
func TestLongIdentifierSurvives(t *testing.T) {
	j := Make()
	const L = 32000
	list := make([]string, L/2)
	for i := range list {
		list[i] = "a"
	}
	for _, tc := range []struct{ what, src string }{
		{"pre-release identifier", "1.0.0-" + strings.Repeat("a", L)},
		{"build identifier", "1.0.0+" + strings.Repeat("a", L)},
		{"major", strings.Repeat("1", L) + ".0.0"},
		{"identifier list", "1.0.0-" + strings.Join(list, ".")},
	} {
		v, err := j.Parse(tc.src)
		if err != nil {
			t.Fatalf("%s: %v", tc.what, err)
		}
		if m, ok := v.(map[string]any); !ok || m["major"] == nil {
			t.Fatalf("%s: no value", tc.what)
		}
	}
}

// TestIdentifierCostIsLinear: ... and the cost of doing it grows with the
// input, not with its square. Machine-INDEPENDENT like the test above:
// the same instance, the same shape, one length against eight times that
// length in the same run. Linear is ~8x, quadratic ~64x; the bound is
// 24x, three times linear, so only a return to quadratic trips it.
func TestIdentifierCostIsLinear(t *testing.T) {
	j := Make()
	measure := func(src string) time.Duration {
		if _, err := j.Parse(src); err != nil { // warm
			t.Fatalf("parse: %v", err)
		}
		t0 := time.Now()
		if _, err := j.Parse(src); err != nil {
			t.Fatalf("parse: %v", err)
		}
		return time.Since(t0)
	}
	small := measure("1.0.0-" + strings.Repeat("a", 2000))
	large := measure("1.0.0-" + strings.Repeat("a", 16000))
	if large > 24*small {
		t.Errorf("identifier cost is superlinear: 2000 chars %v, 16000 chars %v "+
			"(%.1fx for 8x the input)", small, large, float64(large)/float64(small))
	}
	t.Logf("2000=%v 16000=%v ratio=%.1fx", small, large, float64(large)/float64(small))
}
