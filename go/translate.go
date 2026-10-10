package tabnassemver

import _ "embed"

// TranslationPart is one optional alchemy source and the entry point a host calls.
type TranslationPart struct {
	Entry  string
	Source string
}

// TranslationParts is the package-local structural translation interface.
type TranslationParts struct {
	Manifest string
	Lift     *TranslationPart
	// Embed is an optional embedding of a plain tree in the format's
	// schema, with its reverse.
	Embed  *TranslationPart
	Render *TranslationPart
}

// The copies ../ts/embed-translate.js writes (npm run embed), since a Go
// module embeds only files inside it.

//go:embed translate/manifest.json
var translationManifest string

//go:embed translate/embed.alc
var translationEmbed string

//go:embed translate/render.alc
var translationRender string

var translationParts = TranslationParts{
	Manifest: translationManifest,
	Embed:    &TranslationPart{Entry: "semver-embed", Source: translationEmbed},
	Render:   &TranslationPart{Entry: "semver-render", Source: translationRender},
}

// Translate returns Semantic Versioning's translation parts.
// Each call returns a copy of its own, so that what one caller changes
// is not what another reads.
func Translate() *TranslationParts {
	parts := translationParts
	parts.Lift = copyPart(parts.Lift)
	parts.Embed = copyPart(parts.Embed)
	parts.Render = copyPart(parts.Render)
	return &parts
}

// copyPart is a part of its own, so that no caller reaches another's.
func copyPart(part *TranslationPart) *TranslationPart {
	if part == nil {
		return nil
	}
	copied := *part
	return &copied
}
