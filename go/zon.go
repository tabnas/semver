/* Copyright (c) 2025 Richard Rodger, MIT License */

// Package zon is a jsonic plugin that parses Zig Object Notation (ZON)
// syntax. ZON is a data format based on Zig anonymous struct literals.
//
// Example:
//
//	.{
//	    .name = "example",
//	    .version = "0.0.1",
//	    .deps = .{ .foo = .{ .url = "https://..." } },
//	    .paths = .{ "build.zig", "src" },
//	}
package tabnaszon

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	jsonic "github.com/tabnas/jsonic/go"
)

// VERSION is this module's version. It MUST equal ts/package.json
// "version": the release orchestrator rewrites both, and
// TestVersionMatchesPackageJSON fails the build if they drift.
const VERSION = "0.5.6"

// --- BEGIN EMBEDDED zon-grammar.jsonic ---
const grammarText = `
# ZON Grammar Definition
# Parses Zig Object Notation (ZON) - a data format based on Zig anonymous
# struct literals.
#
# Example:
#   .{
#       .name = "example",
#       .version = "0.0.1",
#       .deps = .{ .foo = .{ .url = "https://..." } },
#       .paths = .{ "build.zig", "src" },
#   }
#
# The custom zon-dot lex matcher distinguishes struct (map) and tuple (list)
# openings at lex time by peeking ahead:
#   .{  followed by  .ident =    -> emits #OB  (struct / map)
#   .{  otherwise                -> emits #OS  (tuple / list)
# Both close on } which lexes as #CB. The list rules below use #CB (not
# the default #CS) as the list terminator so that the single } character
# closes both struct and tuple literals.
#
# A bare .identifier emits #TX with val = identifier (the leading dot is
# stripped). This token is both a valid KEY (when followed by =) and a
# valid VAL (when used as an enum literal).
#
# The grammar is applied with { rule: { alt: { g: 'zon' } } } so every
# alt below is automatically tagged with the 'zon' group.

{
  rule: val: open: [
    # Empty .{} -> empty list.
    { s: '#OS #CB' b: 2 p: list g: 'list,empty' }
  ]

  rule: list: open: [
    # @array$ allocates the list node (an empty array). Under the new core
    # the list node is no longer auto-seeded: @tabnas/json's @array$ only
    # runs on its own #OS open alts, which ZON replaces here. Without this
    # the elem rule's @elem-bc/replace would push onto an undefined node.
    { s: '#OS #CB' b: 1 a: '@array$' g: 'list,empty' }
    { s: '#OS' p: elem a: '@array$' g: 'list,open' }
  ]
  rule: list: close: [
    { s: '#CB' g: 'list,close' }
  ]

  rule: elem: close: [
    { s: '#CA #CB' b: 1 g: 'elem,trailing,comma' }
    { s: '#CA' r: elem g: 'elem,next,comma' }
    { s: '#CB' b: 1 g: 'elem,end' }
  ]

  rule: pair: close: [
    { s: '#CA #CB' b: 1 g: 'pair,trailing,comma' }
    { s: '#CA' r: pair g: 'pair,next,comma' }
    { s: '#CB' b: 1 g: 'pair,end' }
  ]
}
`

// --- END EMBEDDED zon-grammar.jsonic ---

// Zon is a jsonic plugin that adds ZON parsing support.
// Options are pre-merged with Defaults by jsonic.UseDefaults.
func Zon(j *jsonic.Jsonic, options map[string]any) error {
	// Guard against re-invocation: SetOptions triggers plugin re-application.
	if j.Decoration("zon-init") != nil {
		return nil
	}
	j.Decorate("zon-init", true)

	charAsNumber := toBool(options["charAsNumber"])
	enumTag := toString(options["enumTag"])

	// If enumTag is set, wrap enum-literal values (produced by zonDot) into
	// `{ [enumTag]: name }` objects. jsonic's relaxed grammar takes ownership
	// of the `@val-bc` (val close) phase via `@val-bc/replace`, which resolves
	// r.Node from the matched token; once a phase is "replaced" the engine
	// SUPPRESSES any `/prepend` on it. So run in the `@val-ac` (after-close)
	// phase — mirroring the TS side — and rewrap the node the close handler
	// produced from the enum token. The val rule is declared in the grammar
	// text, so wireStateActions binds this plain `@val-ac` name as an append
	// to the val rule's AC phase (after jsonic's openval-restore @val-ac).
	refs := map[jsonic.FuncRef]any{
		// Zig rejects a struct literal that repeats a field name. jsonic's
		// own `@pair-bc-jsonic` performs the assignment (last one wins), so
		// this guard has to run *before* it — hence `/prepend`. Go state
		// actions cannot return an error token, so signal via ctx.ParseErr
		// (the engine's own error channel) instead.
		"@pair-bc/prepend": jsonic.StateAction(func(r *jsonic.Rule, ctx *jsonic.Context) {
			if r.U["pair"] != true {
				return
			}
			key, ok := r.U["key"].(string)
			if !ok || r.Node == nil {
				return
			}
			if !nodeMapHasKey(r.Node, key) {
				return
			}
			tkn := r.O0
			if tkn == nil {
				tkn = ctx.T0
			}
			if tkn == nil {
				return
			}
			tkn.Bad("zon_dup_field", map[string]any{"key": key})
			ctx.ParseErr = tkn
		}),
	}
	if enumTag != "" {
		refs["@val-ac"] = jsonic.StateAction(func(r *jsonic.Rule, _ *jsonic.Context) {
			if r.Child != nil && !jsonic.IsUndefined(r.Child.Node) {
				return
			}
			if r.OS == 0 || r.O0 == nil {
				return
			}
			tkn := r.O0
			if tkn.Use == nil {
				return
			}
			if _, ok := tkn.Use["zonEnum"]; !ok {
				return
			}
			if name, ok := tkn.Val.(string); ok {
				r.Node = map[string]any{enumTag: name}
			}
		})
	}

	gs, err := parseGrammarText(grammarText, refs)
	if err != nil {
		return err
	}
	// All jsonic option overrides live on the grammar object so the plugin
	// applies them atomically alongside its rule alts.
	eqSrc := "="
	gs.Options = &jsonic.Options{
		Rule: &jsonic.RuleOptions{
			// Remove jsonic extensions (implicit maps/lists, top-level commas,
			// path dives). ZON uses explicit struct literals only.
			Exclude: "jsonic,imp",
			Start:   "val",
		},
		Fixed: &jsonic.FixedOptions{
			Token: map[string]*string{
				// Bare `{`, `[`, `]` are not valid in ZON. `.{` is handled by
				// the custom zonDot lex matcher below.
				"#OB": nil,
				"#OS": nil,
				"#CS": nil,
				// `=` replaces `:` as the key/value separator.
				"#CL": &eqSrc,
			},
		},
		TokenSet: map[string][]string{
			// ZON field names are identifiers (.ident or .@"...") only.
			"KEY": {"#TX"},
		},
		String: &jsonic.StringOptions{
			Chars:        "\"",
			MultiChars:   "",
			Escape:       map[string]string{"n": "\n", "r": "\r", "t": "\t", "\\": "\\", "\"": "\"", "'": "'"},
			AllowUnknown: boolPtr(false),
		},
		// jsonic's relaxed number lexer accepts `+1`, `.5`, `5.`, `0123`,
		// `1__0` and friends, none of which are ZON. The zonNumber matcher
		// below implements Zig's numeric literal grammar exactly instead.
		Number: &jsonic.NumberOptions{
			Lex: boolPtr(false),
		},
		Error: map[string]string{
			"zon_number":      "invalid ZON number literal: {src}",
			"zon_ident":       "invalid ZON identifier: {src}",
			"zon_char":        "invalid ZON character literal: {src}",
			"zon_doc_comment": "doc comments are not allowed in ZON: {src}",
			"zon_dup_field":   "duplicate struct field name: {src}",
		},
		Comment: &jsonic.CommentOptions{
			Lex: boolPtr(true),
			Def: map[string]*jsonic.CommentDef{
				"hash":  {Line: true, Start: "#", Lex: boolPtr(false)},
				"slash": {Line: true, Start: "//", Lex: boolPtr(true)},
				"multi": {Line: false, Start: "/*", End: "*/", Lex: boolPtr(false)},
			},
		},
		Value: &jsonic.ValueOptions{
			Lex: boolPtr(true),
			Def: map[string]*jsonic.ValueDef{
				"true":  {Val: true},
				"false": {Val: false},
				"null":  {Val: nil},
			},
		},
		Text: &jsonic.TextOptions{
			// Disabled: the default text matcher would consume identifiers,
			// but in ZON identifiers only appear as `.ident` and are handled
			// by the custom zonDot matcher.
			Lex: boolPtr(false),
		},
		Lex: &jsonic.LexOptions{
			Match: map[string]*jsonic.MatchSpec{
				"zonDot":         {Order: 100000, Make: buildZonDotMatcher()},
				"zonMultiString": {Order: 110000, Make: buildZonMultiStringMatcher()},
				"zonChar":        {Order: 120000, Make: buildZonCharMatcher(charAsNumber)},
				"zonNumber":      {Order: 130000, Make: buildZonNumberMatcher()},
				// Must out-order jsonic's comment matcher so `//!` and `///`
				// are rejected instead of eaten as ordinary line comments.
				"zonDocComment": {Order: 140000, Make: buildZonDocCommentMatcher()},
			},
		},
	}
	// Tag every alt in this grammar with the 'zon' group so callers can
	// selectively exclude zon alts via rule.exclude.
	setting := &jsonic.GrammarSetting{
		Rule: &jsonic.GrammarSettingRule{
			Alt: &jsonic.GrammarSettingAlt{G: "zon"},
		},
	}
	if err := j.Grammar(gs, setting); err != nil {
		return fmt.Errorf("zon: failed to apply grammar: %w", err)
	}

	return nil
}

// Defaults matches the TS Zon.defaults. Used with jsonic.UseDefaults.
var Defaults = map[string]any{
	"charAsNumber": false,
	"enumTag":      "",
}

// ZonOptions is a typed wrapper for common plugin options.
// Fields are pointers so callers can express "omit" (nil) vs "set".
type ZonOptions struct {
	// CharAsNumber, when true, parses Zig char literals ('x') as numeric
	// code points. When false (default), they are parsed as one-char strings.
	CharAsNumber *bool
	// EnumTag, when non-empty, wraps enum literals (.foo used as value) in
	// map[string]any{<EnumTag>: name} instead of producing the bare string.
	EnumTag string
}

func (o ZonOptions) toMap() map[string]any {
	m := map[string]any{}
	if o.CharAsNumber != nil {
		m["charAsNumber"] = *o.CharAsNumber
	}
	if o.EnumTag != "" {
		m["enumTag"] = o.EnumTag
	}
	return m
}

// MakeJsonic returns a reusable Jsonic instance configured for ZON parsing.
// Use this when parsing multiple ZON strings with the same options.
func MakeJsonic(opts ...ZonOptions) *jsonic.Jsonic {
	j := jsonic.Make()
	var m map[string]any
	if len(opts) > 0 {
		m = opts[0].toMap()
	}
	if err := j.UseDefaults(Zon, Defaults, m); err != nil {
		// Plugin registration errors are programming errors with static
		// inputs; surface them via panic rather than silent misbehavior.
		panic(fmt.Sprintf("zon: plugin initialisation failed: %v", err))
	}
	return j
}

// defaultParser is a lazily-created instance reused by the default (no-option)
// Parse path, so repeated calls don't rebuild the engine and grammar each time
// (building the ZON grammar dominates a parse — see perf_test.go). Parsing
// builds a fresh context per call and only reads instance state, so the shared
// instance is safe for concurrent use. Mirrors @tabnas/json's Parse.
var (
	defaultOnce   sync.Once
	defaultParser *jsonic.Jsonic
)

// Parse parses a ZON string and returns the resulting value. Convenience
// wrapper around MakeJsonic(opts...).Parse(src).
//
// The default (no-options) path reuses a single cached instance, so repeated
// calls don't rebuild the engine + grammar. Option-taking calls still build a
// dedicated instance, since their configuration differs per call.
func Parse(src string, opts ...ZonOptions) (any, error) {
	if len(opts) == 0 {
		defaultOnce.Do(func() { defaultParser = MakeJsonic() })
		return defaultParser.Parse(src)
	}
	return MakeJsonic(opts...).Parse(src)
}

// Custom lex matcher for `.`-prefixed tokens:
//
//	`.{`           -> #OB if followed by `.ident =`, else #OS
//	`.identifier`  -> #TX (Val = identifier, Use["zonEnum"] = true)
//	`.@"any name"` -> #TX (Val = the decoded string, Use["zonEnum"] = true)
//
// Runs ahead of the fixed-token matcher so it reliably owns the `.` prefix.
func buildZonDotMatcher() jsonic.MakeLexMatcher {
	return func(cfg *jsonic.LexConfig, _ *jsonic.Options) jsonic.LexMatcher {
		return func(lex *jsonic.Lex, _ *jsonic.Rule) *jsonic.Token {
			pnt := lex.Cursor()
			src := lex.Src
			sI := pnt.SI
			if sI >= len(src) || src[sI] != '.' {
				return nil
			}

			// Zig's tokenizer emits `.` and what follows as separate tokens,
			// so whitespace and comments may sit between them (`. foo`).
			dI, dRI, dCI := skipInsigPos(cfg, src, sI+1, pnt.RI, pnt.CI+1)

			advance := func(end int) {
				pnt.SI = end
				pnt.RI = dRI
				pnt.CI = dCI + (end - dI)
			}

			// `.{` opens a struct literal. Decide map vs list by peeking.
			if dI < len(src) && src[dI] == '{' {
				var tkn *jsonic.Token
				if peekIsMapOpen(cfg, src, dI+1) {
					tkn = lex.Token("#OB", jsonic.TinOB, nil, src[sI:dI+1])
				} else {
					tkn = lex.Token("#OS", jsonic.TinOS, nil, src[sI:dI+1])
				}
				advance(dI + 1)
				return tkn
			}

			// `.@"..."` - escaped identifier: any string may name a field or
			// an enum literal. Zig rejects an empty one and one with a NUL.
			if dI+1 < len(src) && src[dI] == '@' && src[dI+1] == '"' {
				val, end, ok := decodeZigString(src, dI+2)
				if !ok || val == "" || strings.IndexByte(val, 0) >= 0 {
					bad := dI + 2
					if ok {
						bad = end
					}
					return zonBad(lex, "zon_ident", src, sI, bad)
				}
				tkn := lex.Token("#TX", jsonic.TinTX, val, src[sI:end])
				tkn.Use = map[string]any{"zonEnum": true}
				advance(end)
				return tkn
			}

			// `.identifier` - field name or enum literal.
			if dI >= len(src) || !isIdStart(src[dI]) {
				return nil
			}
			eI := dI
			for eI < len(src) && isIdCont(src[eI]) {
				eI++
			}

			tkn := lex.Token("#TX", jsonic.TinTX, src[dI:eI], src[sI:eI])
			tkn.Use = map[string]any{"zonEnum": true}
			advance(eI)
			return tkn
		}
	}
}

// zonBad builds a #BD error token spanning src[start:end], so the error
// message can quote the whole offending literal ({src}).
func zonBad(lex *jsonic.Lex, code, src string, start, end int) *jsonic.Token {
	if end > len(src) {
		end = len(src)
	}
	if end <= start {
		end = start + 1
		if end > len(src) {
			end = len(src)
		}
	}
	tkn := lex.Token("#BD", jsonic.TinBD, nil, src[start:end])
	tkn.Err = code
	tkn.Why = code
	return tkn
}

// decodeZigString decodes a Zig double-quoted string body starting at i (just
// past the opening quote), returning the value, the index just past the
// closing quote, and whether the literal was well formed. Used for `.@"..."`
// escaped identifiers; ordinary string literals are lexed by jsonic.
func decodeZigString(src string, i int) (string, int, bool) {
	var b strings.Builder
	for i < len(src) {
		c := src[i]
		if c == '"' {
			return b.String(), i + 1, true
		}
		if c == '\n' || c == '\r' {
			return "", i, false
		}
		if c != '\\' {
			b.WriteByte(c)
			i++
			continue
		}
		if i+1 >= len(src) {
			return "", i, false
		}
		switch e := src[i+1]; e {
		case 'n':
			b.WriteByte('\n')
			i += 2
		case 'r':
			b.WriteByte('\r')
			i += 2
		case 't':
			b.WriteByte('\t')
			i += 2
		case '\\', '\'', '"':
			b.WriteByte(e)
			i += 2
		case 'x':
			if i+4 > len(src) || !isHex(src[i+2:i+4]) {
				return "", i, false
			}
			n, err := strconv.ParseInt(src[i+2:i+4], 16, 32)
			if err != nil {
				return "", i, false
			}
			b.WriteByte(byte(n))
			i += 4
		case 'u':
			if i+2 >= len(src) || src[i+2] != '{' {
				return "", i, false
			}
			end := strings.IndexByte(src[i+3:], '}')
			if end < 0 {
				return "", i, false
			}
			end += i + 3
			hex := src[i+3 : end]
			if !isHex(hex) {
				return "", i, false
			}
			n, err := strconv.ParseInt(hex, 16, 64)
			if err != nil || n > 0x10ffff {
				return "", i, false
			}
			if n == 0 {
				b.WriteByte(0)
			} else {
				b.WriteRune(rune(n))
			}
			i = end + 1
		default:
			return "", i, false
		}
	}
	return "", i, false
}

// peekIsMapOpen returns true if the source position inside `.{ ... }` begins
// with a field name (`.ident` or `.@"..."`) followed by `=`, meaning a
// struct/map literal rather than a tuple.
func peekIsMapOpen(cfg *jsonic.LexConfig, src string, start int) bool {
	i := skipInsig(cfg, src, start)
	if i >= len(src) || src[i] != '.' {
		return false
	}
	i = skipInsig(cfg, src, i+1)
	if i+1 < len(src) && src[i] == '@' && src[i+1] == '"' {
		_, end, ok := decodeZigString(src, i+2)
		if !ok {
			return false
		}
		i = end
	} else {
		if i >= len(src) || !isIdStart(src[i]) {
			return false
		}
		i++
		for i < len(src) && isIdCont(src[i]) {
			i++
		}
	}
	i = skipInsig(cfg, src, i)
	return i < len(src) && src[i] == '='
}

// skipInsig advances past whitespace, newlines, and `//` line comments.
func skipInsig(cfg *jsonic.LexConfig, src string, i int) int {
	out, _, _ := skipInsigPos(cfg, src, i, 1, 1)
	return out
}

// skipInsigPos is skipInsig with row/column tracking, so a token that follows
// an insignificant run still reports an accurate source position.
func skipInsigPos(cfg *jsonic.LexConfig, src string, i, rI, cI int) (int, int, int) {
	for i < len(src) {
		c := src[i]
		if cfg.LineChars[rune(c)] {
			if cfg.RowChars[rune(c)] {
				rI++
			}
			cI = 1
			i++
		} else if c == ' ' || c == '\t' {
			i++
			cI++
		} else if c == '/' && i+1 < len(src) && src[i+1] == '/' {
			for i < len(src) && !cfg.LineChars[rune(src[i])] {
				i++
				cI++
			}
		} else {
			break
		}
	}
	return i, rI, cI
}

// nodeMapHasKey reports whether the map node already carries `key`. The
// engine's default object node is an *OrderedMap (insertion order), but a
// plain map may be configured instead.
func nodeMapHasKey(node any, key string) bool {
	switch n := node.(type) {
	case *jsonic.OrderedMap:
		return n.Has(key)
	case map[string]any:
		_, ok := n[key]
		return ok
	}
	return false
}

func isIdStart(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_'
}

func isIdCont(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_'
}

// Multi-line Zig strings: consecutive lines starting with `\\`. Each `\\`
// line contributes its content verbatim (after the `\\`); lines join with `\n`.
func buildZonMultiStringMatcher() jsonic.MakeLexMatcher {
	return func(cfg *jsonic.LexConfig, _ *jsonic.Options) jsonic.LexMatcher {
		return func(lex *jsonic.Lex, _ *jsonic.Rule) *jsonic.Token {
			pnt := lex.Cursor()
			src := lex.Src
			if pnt.SI+1 >= len(src) || src[pnt.SI] != '\\' || src[pnt.SI+1] != '\\' {
				return nil
			}

			startI := pnt.SI
			startCI := pnt.CI
			sI := pnt.SI
			rI := pnt.RI
			var parts []string

			for sI+1 < len(src) && src[sI] == '\\' && src[sI+1] == '\\' {
				sI += 2
				lineStart := sI
				for sI < len(src) && !cfg.LineChars[rune(src[sI])] {
					sI++
				}
				parts = append(parts, src[lineStart:sI])

				// Consume line terminator (handle \r\n as one).
				if sI < len(src) && cfg.LineChars[rune(src[sI])] {
					ch := src[sI]
					if cfg.RowChars[rune(ch)] {
						rI++
					}
					sI++
					if sI < len(src) && ch == '\r' && src[sI] == '\n' {
						sI++
					}
				}

				// Look for another `\\` continuation. Zig's tokenizer treats
				// the whole run of `\\` lines as ONE token and skips the
				// whitespace between them, so blank lines in the middle
				// continue the literal (contributing nothing) rather than
				// ending it.
				peek := sI
				peekR := 0
				for peek < len(src) {
					pc := src[peek]
					if pc == ' ' || pc == '\t' {
						peek++
					} else if cfg.LineChars[rune(pc)] {
						if cfg.RowChars[rune(pc)] {
							peekR++
						}
						peek++
					} else {
						break
					}
				}
				if peek+1 >= len(src) || src[peek] != '\\' || src[peek+1] != '\\' {
					break
				}
				sI = peek
				rI += peekR
			}

			val := strings.Join(parts, "\n")
			tsrc := src[startI:sI]
			tkn := lex.Token("#ST", jsonic.TinST, val, tsrc)
			pnt.SI = sI
			pnt.RI = rI
			pnt.CI = startCI + (sI - startI)
			return tkn
		}
	}
}

// Zig character literal: `'x'`, `'\n'`, `'\x41'`, `'\u{1F600}'`.
// Produces a numeric code point (if charAsNumber) or a one-char string.
func buildZonCharMatcher(charAsNumber bool) jsonic.MakeLexMatcher {
	return func(_ *jsonic.LexConfig, _ *jsonic.Options) jsonic.LexMatcher {
		return func(lex *jsonic.Lex, _ *jsonic.Rule) *jsonic.Token {
			pnt := lex.Cursor()
			src := lex.Src
			sI := pnt.SI
			if sI >= len(src) || src[sI] != '\'' {
				return nil
			}

			i := sI + 1
			if i >= len(src) {
				return nil
			}

			var codepoint int

			if src[i] == '\\' {
				i++
				if i >= len(src) {
					return nil
				}
				switch src[i] {
				case 'n':
					codepoint = '\n'
					i++
				case 'r':
					codepoint = '\r'
					i++
				case 't':
					codepoint = '\t'
					i++
				case '\\':
					codepoint = '\\'
					i++
				case '\'':
					codepoint = '\''
					i++
				case '"':
					codepoint = '"'
					i++
				case '0':
					codepoint = 0
					i++
				case 'x':
					i++
					if i+2 > len(src) {
						return nil
					}
					hex := src[i : i+2]
					if !isHex(hex) {
						return nil
					}
					n, err := strconv.ParseInt(hex, 16, 32)
					if err != nil {
						return nil
					}
					codepoint = int(n)
					i += 2
				case 'u':
					i++
					if i >= len(src) || src[i] != '{' {
						return nil
					}
					i++
					end := strings.IndexByte(src[i:], '}')
					if end < 0 {
						return nil
					}
					end += i
					hex := src[i:end]
					if !isHex(hex) {
						return nil
					}
					n, err := strconv.ParseInt(hex, 16, 32)
					if err != nil {
						return nil
					}
					codepoint = int(n)
					// Zig: the escape must name a valid unicode scalar value.
					if codepoint > 0x10ffff {
						return zonBad(lex, "zon_char", src, sI, end+2)
					}
					i = end + 1
				default:
					return nil
				}
			} else if src[i] != '\'' {
				r, size := utf8.DecodeRuneInString(src[i:])
				if r == utf8.RuneError && size <= 1 {
					return nil
				}
				// A raw control character (notably a literal newline) is not
				// a character literal in Zig — it is an invalid token.
				if r < 0x20 || r == 0x7f {
					return zonBad(lex, "zon_char", src, sI, i+1)
				}
				codepoint = int(r)
				i += size
			} else {
				return nil
			}

			if i >= len(src) || src[i] != '\'' {
				return nil
			}
			i++

			var val any
			if charAsNumber {
				val = float64(codepoint)
			} else {
				val = string(rune(codepoint))
			}
			tsrc := src[sI:i]
			tkn := lex.Token("#NR", jsonic.TinNR, val, tsrc)
			pnt.SI = i
			pnt.CI += i - sI
			return tkn
		}
	}
}

// `//!` and `///` are Zig DOC comments, which ZON rejects outright. `////`
// and longer runs are plain line comments. This matcher only ever fails the
// lex: an ordinary `//` comment falls through to jsonic's comment matcher.
func buildZonDocCommentMatcher() jsonic.MakeLexMatcher {
	return func(_ *jsonic.LexConfig, _ *jsonic.Options) jsonic.LexMatcher {
		return func(lex *jsonic.Lex, _ *jsonic.Rule) *jsonic.Token {
			pnt := lex.Cursor()
			src := lex.Src
			sI := pnt.SI
			if sI+2 >= len(src) || src[sI] != '/' || src[sI+1] != '/' {
				return nil
			}
			c := src[sI+2]
			if c == '!' || (c == '/' && (sI+3 >= len(src) || src[sI+3] != '/')) {
				return zonBad(lex, "zon_doc_comment", src, sI, sI+3)
			}
			return nil
		}
	}
}

// Zig numeric literals, as ZON defines them. This replaces jsonic's relaxed
// number lexer (Number.Lex false) because that one happily accepts `+1`,
// `.5`, `5.`, `0123`, `00`, `1__0` and `0x_2A`, none of which are ZON.
//
// Accepted: decimal / `0x` / `0o` / `0b` integers with `_` separators between
// digits, decimal and hexadecimal floats (`1.5e3`, `0x1.8p1`, `0x103.70`),
// the `inf` and `nan` keywords, and a leading `-` on any of those except
// `nan`. Integers whose exact value is not representable as a float64 are
// returned as a *big.Int rather than silently rounded (TS: bigint).
func buildZonNumberMatcher() jsonic.MakeLexMatcher {
	return func(_ *jsonic.LexConfig, _ *jsonic.Options) jsonic.LexMatcher {
		return func(lex *jsonic.Lex, _ *jsonic.Rule) *jsonic.Token {
			pnt := lex.Cursor()
			src := lex.Src
			sI := pnt.SI
			if sI >= len(src) {
				return nil
			}

			i := sI
			neg := false
			if src[i] == '-' {
				neg = true
				i++
				for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
					i++
				}
			}
			if i >= len(src) {
				return nil
			}

			// The `inf` / `nan` keywords. Zig allows `-inf` but not `-nan`.
			if c := src[i]; c == 'i' || c == 'n' {
				if i+3 <= len(src) {
					kw := src[i : i+3]
					if (kw == "inf" || kw == "nan") &&
						(i+3 >= len(src) || !isIdCont(src[i+3])) {
						if kw == "nan" && neg {
							return zonBad(lex, "zon_number", src, sI, i+3)
						}
						var val float64
						if kw == "inf" {
							if neg {
								val = math.Inf(-1)
							} else {
								val = math.Inf(1)
							}
						} else {
							val = math.NaN()
						}
						end := i + 3
						tkn := lex.Token("#NR", jsonic.TinNR, val, src[sI:end])
						pnt.SI = end
						pnt.CI += end - sI
						return tkn
					}
				}
				return nil
			}

			if !isDigit(src[i]) {
				return nil
			}

			num, ok := scanZonNumber(src, i)
			if !ok {
				return zonBad(lex, "zon_number", src, sI, num.end)
			}

			var val any
			if num.isInt {
				// Zig: `-0` is an ambiguous integer literal (`-0.0` is fine).
				if neg && num.big.Sign() == 0 {
					return zonBad(lex, "zon_number", src, sI, num.end)
				}
				signed := new(big.Int).Set(num.big)
				if neg {
					signed.Neg(signed)
				}
				if f, acc := new(big.Float).SetInt(signed).Float64(); acc == big.Exact &&
					!math.IsInf(f, 0) {
					val = f
				} else {
					val = signed
				}
			} else if neg {
				val = -num.num
			} else {
				val = num.num
			}

			end := num.end
			tkn := lex.Token("#NR", jsonic.TinNR, val, src[sI:end])
			pnt.SI = end
			pnt.CI += end - sI
			return tkn
		}
	}
}

type zonNumScan struct {
	end   int
	isInt bool
	big   *big.Int
	num   float64
}

// scanZonNumber scans one unsigned Zig numeric literal starting at `start`
// (a digit). On failure the returned end spans the whole malformed literal.
func scanZonNumber(src string, start int) (zonNumScan, bool) {
	i := start
	base := 10
	fail := func() (zonNumScan, bool) {
		return zonNumScan{end: scanNumTokenEnd(src, start)}, false
	}

	if src[i] == '0' && i+1 < len(src) {
		switch c1 := src[i+1]; c1 {
		case 'x':
			base, i = 16, i+2
		case 'o':
			base, i = 8, i+2
		case 'b':
			base, i = 2, i+2
		case 'X', 'O', 'B':
			return fail() // base prefix must be lowercase
		default:
			if c1 == '_' || isDigit(c1) {
				return fail() // leading zero
			}
		}
	}

	intEnd, intCount, bad := scanDigitRun(src, i, base)
	if bad {
		return fail()
	}
	intText := strings.ReplaceAll(src[i:intEnd], "_", "")
	i = intEnd

	isFloat := false
	fracText := ""
	if i < len(src) && src[i] == '.' {
		startsFrac := false
		if i+1 < len(src) {
			after := src[i+1]
			dv := digitVal(after)
			// A `.` only starts a fraction when a digit of this base (or, for
			// hex, the `p` exponent) follows; otherwise the number ends here
			// and the stray `.` is a parse error (`1.`, `0.1.2`).
			startsFrac = (dv >= 0 && dv < base) ||
				(base == 16 && (after == 'p' || after == 'P'))
		}
		if startsFrac {
			if base != 10 && base != 16 {
				return fail() // invalid base for float literal
			}
			isFloat = true
			i++
			fracEnd, _, fbad := scanDigitRun(src, i, base)
			if fbad {
				return fail()
			}
			fracText = strings.ReplaceAll(src[i:fracEnd], "_", "")
			i = fracEnd
		}
	}

	expVal := 0
	hasExp := false
	expChars := ""
	if base == 16 {
		expChars = "pP"
	} else if base == 10 {
		expChars = "eE"
	}
	if expChars != "" && i < len(src) && strings.IndexByte(expChars, src[i]) >= 0 {
		hasExp = true
		isFloat = true
		i++
		expSign := 1
		if i < len(src) && (src[i] == '+' || src[i] == '-') {
			if src[i] == '-' {
				expSign = -1
			}
			i++
		}
		expEnd, expCount, ebad := scanDigitRun(src, i, 10)
		if ebad || expCount == 0 {
			return fail()
		}
		n, err := strconv.Atoi(strings.ReplaceAll(src[i:expEnd], "_", ""))
		if err != nil {
			return fail()
		}
		expVal = expSign * n
		i = expEnd
	}

	if intCount == 0 && len(fracText) == 0 {
		return fail()
	}

	// Anything alphanumeric still attached is a digit invalid for this base.
	if i < len(src) && isIdCont(src[i]) {
		return fail()
	}

	if isFloat {
		var text string
		if base == 16 {
			intPart, fracPart := intText, fracText
			if intPart == "" {
				intPart = "0"
			}
			if fracPart == "" {
				fracPart = "0"
			}
			text = "0x" + intPart + "." + fracPart + "p" + strconv.Itoa(expVal)
		} else {
			intPart := intText
			if intPart == "" {
				intPart = "0"
			}
			fracPart := fracText
			if fracPart == "" {
				fracPart = "0"
			}
			text = intPart + "." + fracPart + "e" + strconv.Itoa(expVal)
			_ = hasExp
		}
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			// Overflow to +/-Inf is what Zig does too; only a genuine syntax
			// failure is an error here.
			if ne, ok := err.(*strconv.NumError); !ok || ne.Err != strconv.ErrRange {
				return fail()
			}
		}
		return zonNumScan{end: i, num: f}, true
	}

	if intText == "" {
		intText = "0"
	}
	v, ok := new(big.Int).SetString(intText, base)
	if !ok {
		return fail()
	}
	return zonNumScan{end: i, isInt: true, big: v}, true
}

// scanDigitRun scans a run of `base` digits with Zig's digit-separator rules:
// a `_` must sit directly between two digits (no leading, trailing, or
// repeated `_`). Returns the end index, the digit count, and a bad flag.
func scanDigitRun(src string, i, base int) (int, int, bool) {
	start := i
	count := 0
	prevDigit := false
	for ; i < len(src); i++ {
		ch := src[i]
		if ch == '_' {
			if !prevDigit {
				return i, count, true
			}
			prevDigit = false
			continue
		}
		dv := digitVal(ch)
		if dv >= 0 && dv < base {
			count++
			prevDigit = true
			continue
		}
		break
	}
	if start < i && !prevDigit {
		return i, count, true
	}
	return i, count, false
}

// scanNumTokenEnd returns the greedy extent of a malformed numeric token, so
// the error span covers the whole literal rather than its first character.
func scanNumTokenEnd(src string, i int) int {
	for i < len(src) {
		if isIdCont(src[i]) {
			i++
		} else if src[i] == '.' && i+1 < len(src) && isIdCont(src[i+1]) {
			i++
		} else {
			break
		}
	}
	return i
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func digitVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

func isHex(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// parseGrammarText parses grammar text into a GrammarSpec with refs attached.
func parseGrammarText(text string, refs map[jsonic.FuncRef]any) (*jsonic.GrammarSpec, error) {
	parsed, err := jsonic.Make().Parse(text)
	if err != nil {
		return nil, fmt.Errorf("zon: failed to parse grammar text: %w", err)
	}
	// The parser now returns insertion-ordered *OrderedMap for parsed objects.
	// A grammar spec is order-agnostic config, so flatten it to plain
	// map[string]any trees before the map assertions below.
	parsed = jsonic.Plainify(parsed)
	parsedMap, ok := parsed.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("zon: grammar text did not parse to a map")
	}
	gs := &jsonic.GrammarSpec{Ref: refs}
	ruleMap, ok := parsedMap["rule"].(map[string]any)
	if !ok {
		return gs, nil
	}
	gs.Rule = make(map[string]*jsonic.GrammarRuleSpec, len(ruleMap))
	for name, rDef := range ruleMap {
		rd, ok := rDef.(map[string]any)
		if !ok {
			continue
		}
		grs := &jsonic.GrammarRuleSpec{}
		if openDef, ok := rd["open"]; ok {
			grs.Open = buildGrammarAlts(openDef)
		}
		if closeDef, ok := rd["close"]; ok {
			grs.Close = buildGrammarAlts(closeDef)
		}
		gs.Rule[name] = grs
	}
	return gs, nil
}

// buildGrammarAlts converts a parsed-jsonic alt array into []*GrammarAltSpec.
func buildGrammarAlts(def any) []*jsonic.GrammarAltSpec {
	arr, ok := def.([]any)
	if !ok {
		return nil
	}
	alts := make([]*jsonic.GrammarAltSpec, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			alts = append(alts, &jsonic.GrammarAltSpec{})
			continue
		}
		ga := &jsonic.GrammarAltSpec{}
		if s, ok := m["s"]; ok {
			switch sv := s.(type) {
			case string:
				ga.S = sv
			case []any:
				strs := make([]string, len(sv))
				for i, v := range sv {
					strs[i], _ = v.(string)
				}
				ga.S = strs
			}
		}
		if b, ok := m["b"]; ok {
			switch bv := b.(type) {
			case float64:
				ga.B = int(bv)
			case int:
				ga.B = bv
			}
		}
		if p, ok := m["p"].(string); ok {
			ga.P = p
		}
		if r, ok := m["r"].(string); ok {
			ga.R = r
		}
		if a, ok := m["a"].(string); ok {
			ga.A = jsonic.FuncRef(a)
		}
		if c, ok := m["c"]; ok {
			switch cv := c.(type) {
			case string:
				ga.C = cv
			case map[string]any:
				ga.C = cv
			}
		}
		if u, ok := m["u"].(map[string]any); ok {
			ga.U = u
		}
		if g, ok := m["g"].(string); ok {
			ga.G = g
		}
		alts = append(alts, ga)
	}
	return alts
}

func toBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func toString(v any) string {
	s, _ := v.(string)
	return s
}

func boolPtr(b bool) *bool {
	return &b
}
