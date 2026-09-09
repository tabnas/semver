// Copyright (c) 2026 Richard Rodger and other contributors, MIT License

package tabnassemver

// Precedence (specification §11), driven by the shared fixtures in
// `test/precedence/` at the repo root — the same files
// ts/test/precedence.test.ts runs, so Compare cannot drift between the
// runtimes.
//
//	order.tsv  one version per row, in strictly ascending precedence
//	equal.tsv  pairs that compare equal (build metadata is ignored)
//
// These sit beside, not in, `test/spec/`, whose runner treats every row as
// a parse case.

import (
	"path/filepath"
	"testing"

	support "github.com/tabnas/support/go"
)

func precedenceDir(t *testing.T) string {
	t.Helper()
	dir, err := support.FindSpecDir("")
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "..", "precedence")
}

func TestPrecedenceOrder(t *testing.T) {
	spec, err := support.LoadSpec(filepath.Join(precedenceDir(t), "order.tsv"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Rows) <= 20 {
		t.Fatalf("order.tsv has %d rows; expected a real chain", len(spec.Rows))
	}
	j := Make()
	versions := make([]string, len(spec.Rows))
	parsed := make([]any, len(spec.Rows))
	for i, row := range spec.Rows {
		versions[i] = row.Named("version")
		v, err := j.Parse(versions[i])
		if err != nil {
			t.Fatalf("%s: %v", row.Where(), err)
		}
		parsed[i] = v
		if out, err := Format(v); err != nil || out != versions[i] {
			t.Errorf("%s: Format = %q, %v", row.Where(), out, err)
		}
		if c, err := Compare(v, v); err != nil || c != 0 {
			t.Errorf("%s: %s should equal itself: %d, %v", row.Where(), versions[i], c, err)
		}
	}
	for i := range parsed {
		for k := i + 1; k < len(parsed); k++ {
			if c, err := Compare(parsed[i], parsed[k]); err != nil || c != -1 {
				t.Errorf("%s: Compare(%s, %s) = %d, %v; want -1", spec.Rows[i].Where(), versions[i], versions[k], c, err)
			}
			if c, err := Compare(parsed[k], parsed[i]); err != nil || c != 1 {
				t.Errorf("%s: Compare(%s, %s) = %d, %v; want 1", spec.Rows[k].Where(), versions[k], versions[i], c, err)
			}
		}
	}
}

func TestPrecedenceEqual(t *testing.T) {
	spec, err := support.LoadSpec(filepath.Join(precedenceDir(t), "equal.tsv"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Rows) <= 5 {
		t.Fatalf("equal.tsv has %d rows", len(spec.Rows))
	}
	j := Make()
	for _, row := range spec.Rows {
		a, b := row.Named("a"), row.Named("b")
		va, err := j.Parse(a)
		if err != nil {
			t.Fatalf("%s: %v", row.Where(), err)
		}
		vb, err := j.Parse(b)
		if err != nil {
			t.Fatalf("%s: %v", row.Where(), err)
		}
		if c, err := Compare(va, vb); err != nil || c != 0 {
			t.Errorf("%s: Compare(%s, %s) = %d, %v; want 0", row.Where(), a, b, c, err)
		}
		if c, err := Compare(vb, va); err != nil || c != 0 {
			t.Errorf("%s: Compare(%s, %s) = %d, %v; want 0", row.Where(), b, a, c, err)
		}
	}
}
