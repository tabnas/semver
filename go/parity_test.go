// Copyright (c) 2025 Richard Rodger and other contributors, MIT License

package tabnaszon

// parity_test.go — cross-runtime conformance, driven by the shared
// `test/spec/*.tsv` fixtures at the repo root (see ../test/AGENTS.md).
//
// The fixture loader, the escape codec, the ERROR:<code> contract and the
// row loop all come from github.com/tabnas/support/go, whose TypeScript
// half ts/test/parity.test.ts uses to run the SAME files — so the two
// implementations cannot drift without one of them going red, and neither
// can the two loaders.
//
// What is left here is only what is specific to zon: how to build the
// parser for a row's options, and how to flatten a result for comparison.

import (
	"encoding/json"
	"testing"

	jsonic "github.com/tabnas/jsonic/go"
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

	support.Runner{
		// A fresh parser per row: the `opts` column is per-case, and
		// plugin options must not leak from one row into the next.
		ParseRow: func(input string, row *support.Row) (any, error) {
			opts := map[string]any{}
			if raw := row.Named("opts"); "" != raw {
				if err := json.Unmarshal([]byte(raw), &opts); err != nil {
					return nil, err
				}
			}

			j := jsonic.Make()
			if err := j.UseDefaults(Zon, Defaults, opts); err != nil {
				return nil, err
			}
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
