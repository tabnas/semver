package tabnassemver

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"testing"
)

// What the package hands a host is the repository's manifest and the
// alchemy files it names: npm run embed, from ts/, copies them into
// go/translate/, and this fails until the copies are the files.
func TestTranslationParts(t *testing.T) {
	inCheckout(t)
	parts := Translate()
	if parts == nil {
		t.Fatal("Translate returned nil")
	}
	manifest, err := os.ReadFile("../tabnas.plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	if parts.Manifest != string(manifest) {
		t.Fatal("embedded manifest differs from tabnas.plugin.json")
	}
	if parts.Lift != nil {
		t.Fatal("Semantic Versioning has no lift")
	}
	if parts.Render == nil || parts.Render.Entry != "semver-render" {
		t.Fatalf("render entry is %#v", parts.Render)
	}
	render, err := os.ReadFile("../alchemy/render.alc")
	if err != nil {
		t.Fatal(err)
	}
	if parts.Render.Source != string(render) {
		t.Fatal("embedded render differs from alchemy/render.alc")
	}
	if parts.Embed == nil || parts.Embed.Entry != "semver-embed" {
		t.Fatalf("embed entry is %#v", parts.Embed)
	}
	embed, err := os.ReadFile("../alchemy/embed.alc")
	if err != nil {
		t.Fatal(err)
	}
	if parts.Embed.Source != string(embed) {
		t.Fatal("embedded embed differs from alchemy/embed.alc")
	}
}

// The manifest's translate object names the files the package carries,
// and says what the render takes: a tree whose root is object, of the
// schema semver, which a plain tree reaches through the embedding.
func TestTranslationManifest(t *testing.T) {
	var spec struct {
		LanguageID string `json:"languageId"`
		Translate  struct {
			Reads  string   `json:"reads"`
			Writes string   `json:"writes"`
			Root   string   `json:"root"`
			Schema string   `json:"schema"`
			Lift   *string  `json:"lift"`
			Embed  string   `json:"embed"`
			Render string   `json:"render"`
			Loss   []string `json:"loss"`
		} `json:"translate"`
	}
	if err := json.Unmarshal([]byte(Translate().Manifest), &spec); err != nil {
		t.Fatal(err)
	}
	tr := spec.Translate
	if spec.LanguageID != "semver" || tr.Reads != "tree" || tr.Writes != "tree" || tr.Root != "object" || tr.Schema != "semver" || tr.Lift != nil {
		t.Fatalf("the manifest says %+v", spec)
	}
	if tr.Embed != "alchemy/embed.alc" || tr.Render != "alchemy/render.alc" {
		t.Fatalf("the manifest names %q and %q", tr.Embed, tr.Render)
	}
	if len(tr.Loss) == 0 {
		t.Fatal("the manifest declares no loss")
	}
	for _, line := range tr.Loss {
		if line == "" || line[0] < 'A' || line[0] > 'Z' || line[len(line)-1] != '.' {
			t.Fatalf("%q is not a sentence", line)
		}
	}
}

// Every call hands back parts of its own: a caller that changes what it
// was given, the parts or a part they point to, changes nothing the next
// caller reads, and callers on several goroutines share nothing to race on.
func TestTranslateReturnsACopy(t *testing.T) {
	first := Translate()
	want := *Translate().Render
	first.Manifest = ""
	if first.Render != nil {
		first.Render.Entry = ""
		first.Render.Source = ""
	}
	first.Lift, first.Embed, first.Render = nil, nil, nil
	second := Translate()
	if second.Manifest == "" {
		t.Fatal("a change to one call's manifest reached the next call")
	}
	if second.Render == nil || *second.Render != want {
		t.Fatalf("a change to one call's render reached the next call: %#v", second.Render)
	}
}

// inCheckout skips a test that holds the embedded copies to the
// repository's own files when it runs where those files are not, as from
// the module cache, whose zip holds the go/ module alone. In a checkout,
// a missing file still fails the test that reads it.
func inCheckout(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("../ts/package.json"); errors.Is(err, fs.ErrNotExist) {
		t.Skip("not in a checkout of the repository: the module cache holds the go/ module alone")
	}
}
