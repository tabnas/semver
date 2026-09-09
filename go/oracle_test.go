// Copyright (c) 2026 Richard Rodger and other contributors, MIT License

package tabnassemver

// Conformance against the specification's own oracle.
//
// semver.org publishes a regular expression that recognises exactly the
// language of its grammar (FAQ: "Is there a suggested regular expression
// (RegEx) to check a SemVer string?"). It is a different formalism from
// the ABNF this plugin compiles, written by different people, and its
// captures name the same five parts the plugin's value carries — which
// makes it the judge here. On every string of the corpus below the
// plugin's VERDICT must equal the expression's, and on every accepted
// string the plugin's VALUE must match the expression's captures and
// Format must give the input back.
//
// The corpus is generated, not committed, and is IDENTICAL to the one in
// ts/test/oracle.test.ts: the same alphabets, the same enumeration order,
// the same xorshift32 stream from the same seed, the same mutation
// operators in the same order. A pinned census (how many strings each
// section accepts) and a pinned FNV-1a hash over the whole corpus make
// sure both runtimes graded the same strings. Changing the corpus means
// changing both constants, in both runtimes, in the same commit.

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"testing"

	tabnas "github.com/tabnas/parser/go"
)

// semver.org, FAQ — the numbered-capture-group form. Group 1..3 are the
// version core, 4 the pre-release (without the `-`), 5 the build metadata
// (without the `+`). RE2 and JavaScript agree on every construct in it.
var oracle = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

// ---- the corpus (keep byte-identical with ts/test/oracle.test.ts) ------

const (
	oracleShort    = "019aZ-.+"
	oracleShortMax = 5
	oraclePre      = "01a."
	oracleBld      = "0a."
	oracleTailMax  = 3
	oracleWide     = "019aZ-.+v _xB250\t/:"
	oracleMutation = 3000
	oracleRandoms  = 1000
	oracleSeed     = 0x9e3779b9
)

var oracleCores = []string{"0.0.0", "1.2.3", "01.0.0", "1.0", "1"}

var oracleSeeds = []string{
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
	"10.20.30-rc.1+build.2",
}

// The pinned census and hash. A change here is a change to the corpus,
// and needs the same change in ts/test/oracle.test.ts.
type oracleCount struct{ total, accepted int }

var oracleCensus = []struct {
	name string
	oracleCount
}{
	{"exhaustive", oracleCount{37449, 27}},
	{"structured", oracleCount{17000, 1634}},
	{"mutation", oracleCount{3000, 838}},
	{"random", oracleCount{1000, 0}},
}

const oracleHash = 0x97bd27cb

// Every string of alphabet of exactly n characters, in base-n counting
// order (most significant position first), appended to out.
func oracleStrings(out []string, alphabet string, n int) []string {
	total := 1
	for i := 0; i < n; i++ {
		total *= len(alphabet)
	}
	buf := make([]byte, n)
	for idx := 0; idx < total; idx++ {
		rest := idx
		for k := n - 1; k >= 0; k-- {
			buf[k] = alphabet[rest%len(alphabet)]
			rest /= len(alphabet)
		}
		out = append(out, string(buf))
	}
	return out
}

func oracleUpTo(alphabet string, min, max int) []string {
	var out []string
	for n := min; n <= max; n++ {
		out = oracleStrings(out, alphabet, n)
	}
	return out
}

// xorshift32, the same stream in both runtimes.
type oracleRng struct{ x uint32 }

func (r *oracleRng) next() uint32 {
	x := r.x
	x ^= x << 13
	x ^= x >> 17
	x ^= x << 5
	r.x = x
	return x
}

func (r *oracleRng) intn(n int) int { return int(r.next() % uint32(n)) }

func (r *oracleRng) pick(alphabet string) byte { return alphabet[r.intn(len(alphabet))] }

// One random edit. Every branch consumes the stream in the same order as
// its TS twin, including the branches that do nothing to an empty string.
func oracleMutate(s string, r *oracleRng) string {
	op := r.intn(5)
	n := len(s)
	switch op {
	case 0:
		pos := r.intn(n + 1)
		return s[:pos] + string(r.pick(oracleWide)) + s[pos:]
	case 1:
		if n == 0 {
			return s
		}
		pos := r.intn(n)
		return s[:pos] + s[pos+1:]
	case 2:
		if n == 0 {
			return s
		}
		pos := r.intn(n)
		return s[:pos] + string(r.pick(oracleWide)) + s[pos+1:]
	case 3:
		if n == 0 {
			return s
		}
		i := r.intn(n)
		j := i + 1 + r.intn(n-i)
		k := r.intn(n + 1)
		return s[:k] + s[i:j] + s[k:]
	}
	sep := ".-+"[r.intn(3)]
	count := 1 + r.intn(3)
	var seg strings.Builder
	for c := 0; c < count; c++ {
		seg.WriteByte(r.pick(oracleShort))
	}
	return s + string(sep) + seg.String()
}

func oracleCorpus() map[string][]string {
	exhaustive := append([]string{""}, oracleUpTo(oracleShort, 1, oracleShortMax)...)

	var structured []string
	pres := oracleUpTo(oraclePre, 0, oracleTailMax)
	blds := oracleUpTo(oracleBld, 0, oracleTailMax)
	for _, core := range oracleCores {
		for _, x := range pres {
			for _, y := range blds {
				s := core
				if x != "" {
					s += "-" + x
				}
				if y != "" {
					s += "+" + y
				}
				structured = append(structured, s)
			}
		}
	}

	r := &oracleRng{x: oracleSeed}
	var mutation []string
	for i := 0; i < oracleMutation; i++ {
		s := oracleSeeds[r.intn(len(oracleSeeds))]
		edits := 1 + r.intn(3)
		for e := 0; e < edits; e++ {
			s = oracleMutate(s, r)
		}
		mutation = append(mutation, s)
	}

	var random []string
	for i := 0; i < oracleRandoms; i++ {
		n := 1 + r.intn(12)
		buf := make([]byte, n)
		for c := 0; c < n; c++ {
			buf[c] = r.pick(oracleWide)
		}
		random = append(random, string(buf))
	}

	return map[string][]string{
		"exhaustive": exhaustive,
		"structured": structured,
		"mutation":   mutation,
		"random":     random,
	}
}

// FNV-1a, 32-bit, over the bytes of every string in corpus order, each
// followed by a newline byte.
func oracleFnv1a(sections ...[]string) uint32 {
	h := uint32(0x811c9dc5)
	for _, section := range sections {
		for _, s := range section {
			for i := 0; i < len(s); i++ {
				h = (h ^ uint32(s[i])) * 0x01000193
			}
			h = (h ^ 0x0a) * 0x01000193
		}
	}
	return h
}

// ---- the judge ---------------------------------------------------------

func oracleIsDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || '9' < s[i] {
			return false
		}
	}
	return len(s) > 0
}

// The decimal text of a component, and whether it is a number (float64
// or *big.Int) rather than a string.
func oracleText(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, false
	case float64:
		return fmt.Sprintf("%.0f", x), true
	case *big.Int:
		return x.String(), true
	}
	return fmt.Sprintf("%v", v), false
}

// Grade one string; return a description of the disagreement, or "".
func oracleGrade(j *tabnas.Tabnas, s string) string {
	m := oracle.FindStringSubmatch(s)
	v, err := j.Parse(s)

	if m == nil {
		if err == nil {
			return fmt.Sprintf("accepted, oracle rejects: %q", s)
		}
		if te, ok := err.(*tabnas.TabnasError); !ok || te.Code != "unexpected" {
			return fmt.Sprintf("rejected with %v, not unexpected: %q", err, s)
		}
		return ""
	}

	if err != nil {
		return fmt.Sprintf("rejected (%v), oracle accepts: %q", err, s)
	}
	out, ferr := Format(v)
	if ferr != nil || out != s {
		return fmt.Sprintf("Format gave %q, %v for %q", out, ferr, s)
	}
	val := v.(map[string]any)
	for i, key := range []string{"major", "minor", "patch"} {
		text, isNum := oracleText(val[key])
		if !isNum || text != m[i+1] {
			return fmt.Sprintf("%s %q != %q in %q", key, text, m[i+1], s)
		}
	}
	var pre []string
	if m[4] != "" {
		pre = strings.Split(m[4], ".")
	}
	got := val["prerelease"].([]any)
	if len(got) != len(pre) {
		return fmt.Sprintf("prerelease length in %q", s)
	}
	for i := range pre {
		text, isNum := oracleText(got[i])
		if text != pre[i] {
			return fmt.Sprintf("prerelease[%d] %q != %q in %q", i, text, pre[i], s)
		}
		if isNum != oracleIsDigits(pre[i]) {
			return fmt.Sprintf("prerelease[%d] %q has the wrong type in %q", i, pre[i], s)
		}
	}
	var bld []string
	if m[5] != "" {
		bld = strings.Split(m[5], ".")
	}
	gotB := val["build"].([]any)
	if len(gotB) != len(bld) {
		return fmt.Sprintf("build length in %q", s)
	}
	for i := range bld {
		if gotB[i] != bld[i] {
			return fmt.Sprintf("build[%d] %v != %q in %q", i, gotB[i], bld[i], s)
		}
	}
	return ""
}

func TestOracleCorpusIsPinned(t *testing.T) {
	sections := oracleCorpus()
	for _, c := range oracleCensus {
		if got := len(sections[c.name]); got != c.total {
			t.Errorf("%s size = %d, want %d", c.name, got, c.total)
		}
	}
	h := oracleFnv1a(sections["exhaustive"], sections["structured"], sections["mutation"], sections["random"])
	if h != oracleHash {
		t.Errorf("corpus hash = %#x, want %#x — the generator changed; re-pin the hash and census here and in ts/test/oracle.test.ts", h, uint32(oracleHash))
	}
}

func TestOracleAgreement(t *testing.T) {
	sections := oracleCorpus()
	j := Make()
	for _, c := range oracleCensus {
		t.Run(c.name, func(t *testing.T) {
			var failures []string
			accepted := 0
			for _, s := range sections[c.name] {
				if why := oracleGrade(j, s); why != "" {
					if len(failures) < 20 {
						failures = append(failures, why)
					}
				} else if oracle.MatchString(s) {
					accepted++
				}
			}
			if len(failures) > 0 {
				t.Errorf("disagreements with the oracle in %s (first %d):\n  %s", c.name, len(failures), strings.Join(failures, "\n  "))
			}
			if accepted != c.accepted {
				t.Errorf("%s accepted %d; the census pins %d", c.name, accepted, c.accepted)
			}
		})
	}
}
