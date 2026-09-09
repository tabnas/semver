// Copyright (c) 2026 Richard Rodger and other contributors, MIT License

package tabnassemver

// parity_test.go — cross-runtime conformance, driven by the shared
// `test/spec/*.tsv` fixtures at the repo root (see ../test/AGENTS.md).
//
// The fixture loader, the escape codec, the ERROR:<code> contract and the
// row loop all come from github.com/tabnas/support/go, whose TypeScript
// half ts/test/parity.test.ts uses to run the SAME files — so the two
// implementations cannot drift without one of them going red, and neither
// can the two loaders.
//
// What is left here is only what is specific to semver: how to build the
// parser, and how to flatten a result for comparison.

import (
	"encoding/json"
	"testing"

	support "github.com/tabnas/support/go"
)

// TestSpec runs every fixture in the spec directory. FindSpecDir walks up
// from the package directory, and Dir discovers the files by listing, so
// adding a .tsv runs it in both runtimes without touching either runner.
func TestSpec(t *testing.T) {
	dir, err := support.FindSpecDir("")
	if err != nil {
		t.Fatal(err)
	}

	// One instance for every row: the plugin has no options, so nothing
	// can leak between rows, and rebuilding the grammar per row would
	// only make the suite slow.
	j := Make()

	support.Runner{
		Parse: func(input string) (any, error) {
			return j.Parse(input)
		},

		// Flatten through JSON so the parser's own containers and numeric
		// types compare against the fixture's decoded shape. Normalize runs
		// outermost first, so the top node is enough — the plain values it
		// yields pass through unchanged.
		Normalize: jsonFlatten,
	}.Dir(t, dir)
}

// jsonFlatten renders a value as JSON and reads it back as plain
// map/slice/float64/string/bool/nil. A value that will not marshal is
// returned as it is: the comparison then fails and prints it, which says
// more than a panic here would.
func jsonFlatten(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return v
	}
	return out
}
