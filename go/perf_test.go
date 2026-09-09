/* Copyright (c) 2026 Richard Rodger, MIT License */

package tabnassemver

import (
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
