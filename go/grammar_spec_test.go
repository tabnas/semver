// Copyright (c) 2026 Richard Rodger and other contributors, MIT License

package tabnassemver

// The compiled grammar. semver-grammar.abnf is compiled at build time,
// by `npm run gen-grammar` (ts/gen-grammar.js), into semver-grammar.json
// at the repository root, and this module embeds a verbatim copy. The
// TypeScript suite holds the generated file to the compiler
// (ts/test/grammar-spec.test.ts); these tests hold this module to the
// generated file, and keep the compiler out of what the module ships.

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEmbeddedGrammarSpecIsTheGeneratedFile(t *testing.T) {
	generated, err := os.ReadFile(filepath.Join("..", "semver-grammar.json"))
	if err != nil {
		t.Fatalf("reading the generated file: %v", err)
	}
	if !bytes.Equal(grammarSpec, generated) {
		t.Fatal("go/semver-grammar.json differs from semver-grammar.json at " +
			"the repository root: run `npm run gen-grammar` (from ts/), which " +
			"writes every copy, and never edit one by hand")
	}
	// It is the engine's serialized form: a JSON object carrying options,
	// rules and the builtin schema version, and no function reference.
	var doc map[string]any
	if err := json.Unmarshal(grammarSpec, &doc); err != nil {
		t.Fatalf("the compiled grammar is not JSON: %v", err)
	}
	for _, key := range []string{"options", "rule", "v"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("the compiled grammar has no %q", key)
		}
	}
	if _, ok := doc["ref"]; ok {
		t.Error("the compiled grammar carries a ref map; it must be pure data")
	}
}

// The packages this module ships (the plugin and its C library) must not
// import the ABNF compiler, directly or through anything else: it is a
// build-time tool now. `go list -deps` reports the whole import graph of
// the non-test packages, which is what a consumer compiles.
func TestShippedPackagesImportNoCompiler(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		gobin = filepath.Join(runtime.GOROOT(), "bin", "go")
	}
	out, err := exec.Command(gobin, "list", "-deps", "-f", "{{.ImportPath}}", "./...").CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	deps := strings.Fields(string(out))
	sawEngine := false
	for _, dep := range deps {
		if dep == "github.com/tabnas/parser/go" {
			sawEngine = true
		}
		for _, compiler := range []string{"github.com/tabnas/abnf/", "github.com/tabnas/bnf/"} {
			if strings.HasPrefix(dep+"/", compiler) {
				t.Errorf("a shipped package depends on %s", dep)
			}
		}
	}
	// The listing is only evidence if it is the real graph.
	if !sawEngine {
		t.Fatalf("go list did not report the engine among %d dependencies", len(deps))
	}
}
