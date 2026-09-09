// Copyright (c) 2025 Richard Rodger and other contributors, MIT License

package tabnaszon

// zigzon_test.go — conformance against the ZIG REFERENCE IMPLEMENTATION.
//
// Two corpora, both GENERATED (never committed) by `scripts/fetch-zigzon.sh`
// from the pinned ziglang/zig 0.16.0 release:
//
//	test/zigzon/cases.json      every .zon file and every ZON snippet in the
//	                            zig tree, plus its verdict/value
//	test/strictness/cases.json  locally authored leniency probes, judged by
//	                            the same reference implementation
//
// Every verdict, and every valid document's expected VALUE, comes from
// `std.zig.Ast` + `std.zig.ZonGen` at that pinned version — this repo never
// decides what ZON means. Both halves are exercised: a valid document must
// parse AND produce the reference value (not merely "it did not throw"), and
// an invalid document must be rejected.
//
// The corpora are not bundled (generating them downloads a pinned zig
// toolchain), so TestMain below runs scripts/fetch-zigzon.sh before any test:
// `go test ./...` builds them itself, wherever it runs, CI included. A corpus
// that is still absent afterwards is a FAILURE, never a skip — a suite that
// quietly does not run reports a green tick that means nothing. The one
// exception is a host the pinned zig oracle toolchain is not wired for, where
// it reports a single explicit, platform-named skip.
//
// ts/test/zigzon.test.ts runs the identical corpora, so the two runtimes
// cannot drift.

import (
	"encoding/json"
	"math"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	jsonic "github.com/tabnas/jsonic/go"
)

// Corpus paths, relative to go/ (the working directory of `go test`).
var (
	zigZonCorpus     = filepath.Join("..", "test", "zigzon", "cases.json")
	strictnessCorpus = filepath.Join("..", "test", "strictness", "cases.json")
	fetchScript      = filepath.Join("..", "scripts", "fetch-zigzon.sh")
)

// oracleHost reports whether scripts/fetch-zigzon.sh has a pinned zig oracle
// toolchain for this platform. On anything else the corpus cannot be built at
// all, and that — not a missing file — is what the single permitted skip
// reports.
func oracleHost() bool {
	switch runtime.GOOS {
	case "linux", "darwin":
	default:
		return false
	}
	return "amd64" == runtime.GOARCH || "arm64" == runtime.GOARCH
}

// TestMain generates the conformance corpora before any test runs, so the
// conformance suites are never at the mercy of someone having remembered to
// run a script. This is the Go half of ts/package.json's `pretest` hook: the
// shared CI workflow runs `go test ./...` directly and has no repo-specific
// step to hang a fetch on, so without this the suites had no corpus in CI and
// silently graded nothing.
//
// The fetch is idempotent and cached, so this costs a few milliseconds once
// the pinned downloads are in place.
func TestMain(m *testing.M) {
	_, zErr := os.Stat(zigZonCorpus)
	_, sErr := os.Stat(strictnessCorpus)
	if (zErr != nil || sErr != nil) && oracleHost() {
		cmd := exec.Command("bash", fetchScript)
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			// Do not exit here: let the suites below report the missing
			// corpus as a test failure, with instructions, rather than
			// aborting the whole package before the other tests run.
			os.Stderr.WriteString("fetch-zigzon.sh failed: " + err.Error() + "\n")
		}
	}
	os.Exit(m.Run())
}

type zigCase struct {
	Name   string          `json:"name"`
	Origin string          `json:"origin"`
	Source string          `json:"source"`
	Valid  bool            `json:"valid"`
	Value  json.RawMessage `json:"value"`
	Error  string          `json:"error"`
}

type zigCorpus struct {
	Source struct {
		Zig    string `json:"zig"`
		Commit string `json:"commit"`
	} `json:"source"`
	Cases []zigCase `json:"cases"`
}

// The plugin options that put its output in the SAME shape as the oracle's
// canonical encoding, so values compare directly: `enumTag` distinguishes the
// enum literal `.foo` from the string "foo" (the default flat encoding
// cannot), and `charAsNumber` matches ZonGen's char_literal. This is
// representation alignment, not leniency — no input is excused by it.
func zigOpts() map[string]any {
	return map[string]any{"enumTag": "$enum", "charAsNumber": true}
}

// zigCanon puts a parsed value in the oracle's canonical JSON shape. The
// oracle spells the non-JSON numbers "@inf"/"@-inf"/"@nan" and an integer too
// large for an exact float64 as {"$big": "<decimal>"}; do the same here so a
// CORRECT parse is comparable. It never excuses a wrong one.
func zigCanon(v any) any {
	switch t := v.(type) {
	case *big.Int:
		return map[string]any{"$big": t.String()}
	case float64:
		if math.IsNaN(t) {
			return "@nan"
		}
		if math.IsInf(t, 1) {
			return "@inf"
		}
		if math.IsInf(t, -1) {
			return "@-inf"
		}
		return t
	case *jsonic.OrderedMap:
		out := map[string]any{}
		for _, k := range t.Keys {
			out[k] = zigCanon(t.Vals[k])
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for k, val := range t {
			out[k] = zigCanon(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = zigCanon(val)
		}
		return out
	}
	return v
}

func loadZigCorpus(t *testing.T, path string) *zigCorpus {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc zigCorpus
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return &doc
}

func zigLabel(c zigCase) string {
	one := strings.Join(strings.Fields(c.Source), " ")
	if 60 < len(one) {
		one = one[:57] + "..."
	}
	return c.Origin + " | " + one
}

// runZigCorpus grades every document in a corpus. wantValid/wantInvalid are the
// pinned census: a floor of "more than zero" lets a broken generator quietly
// shrink the corpus and inflate the pass rate, so the exact counts are asserted.
func runZigCorpus(t *testing.T, path string, wantValid, wantInvalid int) {
	doc := loadZigCorpus(t, path)
	if doc == nil {
		if !oracleHost() {
			// Not a hidden skip: it names the platform, and it cannot mask a
			// missing corpus anywhere the oracle can actually be built.
			t.Skipf("no pinned zig oracle toolchain for %s/%s, so the corpus "+
				"cannot be generated on this host", runtime.GOOS, runtime.GOARCH)
		}
		t.Fatalf("corpus %s is missing.\nIt is generated by "+
			"`bash scripts/fetch-zigzon.sh`, which TestMain runs before this "+
			"test. This suite FAILS rather than skips when the corpus is "+
			"absent: a conformance suite that quietly does not run reports a "+
			"green tick while measuring nothing.", path)
	}
	if len(doc.Cases) == 0 {
		t.Fatalf("%s: corpus has no cases", path)
	}

	valid, invalid := 0, 0
	for _, c := range doc.Cases {
		if c.Valid {
			valid++
		} else {
			invalid++
		}
	}
	if valid != wantValid || invalid != wantInvalid {
		t.Fatalf("%s: corpus census is %d valid / %d invalid, pinned at %d / %d. "+
			"The generator changed, or the corpus was narrowed. Do not adjust "+
			"this number to match; find out what changed.",
			path, valid, invalid, wantValid, wantInvalid)
	}

	j := jsonic.Make()
	if err := j.UseDefaults(Zon, Defaults, zigOpts()); err != nil {
		t.Fatalf("plugin init: %v", err)
	}

	for _, c := range doc.Cases {
		t.Run(zigLabel(c), func(t *testing.T) {
			got, err := j.Parse(c.Source)

			if !c.Valid {
				if err == nil {
					t.Fatalf("accepted a document zig rejects (%s):\n%s", c.Error, c.Source)
				}
				return
			}
			if err != nil {
				t.Fatalf("rejected a document zig accepts:\n%s\n%v", c.Source, err)
			}

			raw, mErr := json.Marshal(zigCanon(got))
			if mErr != nil {
				t.Fatalf("marshal: %v", mErr)
			}
			var gotVal, wantVal any
			if err := json.Unmarshal(raw, &gotVal); err != nil {
				t.Fatalf("unmarshal got: %v", err)
			}
			if err := json.Unmarshal(c.Value, &wantVal); err != nil {
				t.Fatalf("unmarshal want: %v", err)
			}
			if !reflect.DeepEqual(gotVal, wantVal) {
				t.Errorf("value mismatch for %s\n  got  %s\n  want %s",
					c.Origin, raw, c.Value)
			}
		})
	}
}

// TestZigZon runs every ZON document harvested from the zig tree.
// Census re-measured 2026-08-09 against zig 0.16.0; the TS runner pins the
// same numbers.
func TestZigZon(t *testing.T) {
	runZigCorpus(t, zigZonCorpus, 184, 44)
}

// TestZigStrictness runs the leniency probes: inputs designed to catch
// relaxed-JSON behaviour leaking through the jsonic layer, with the verdicts
// supplied by the same reference implementation.
func TestZigStrictness(t *testing.T) {
	runZigCorpus(t, strictnessCorpus, 45, 72)
}
