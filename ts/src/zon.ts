/* Copyright (c) 2025 Richard Rodger, MIT License */

// The engine is the tabnas parser; jsonic supplies the relaxed-JSON
// grammar that the embedded grammar text is authored in.
import {
  Tabnas,
  Rule,
  Plugin,
  Context,
  Config,
  TabnasOptions,
  Lex,
} from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'

// Plugin options.
type ZonOptions = {
  // When true, parse Zig char literals ('x') as numeric code points.
  // When false, parse them as single-character strings.
  charAsNumber: boolean
  // When set, wrap enum literals (.foo used as value) in `{ [enumTag]: 'foo' }`
  // instead of producing the bare string 'foo'.
  enumTag: null | string
}

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

// Plugin implementation.
const Zon: Plugin = (tn: Tabnas, options: ZonOptions) => {
  const charAsNumber = !!options.charAsNumber
  const enumTag = options.enumTag || null

  // Human descriptions for the ZON tokens, surfaced in railroad diagram
  // legends (read off the live config by @tabnas/railroad). ZON uses Zig
  // anonymous-struct syntax: both maps and tuples open with `.{` and close
  // with `}`; bare { [ ] are disabled and keys are `.identifier` field names.
  tn.options({
    config: {
      modify: {
        'zon-tokendesc': (cfg: any) => {
          cfg.tokenDesc = Object.assign(cfg.tokenDesc || {}, {
            '#OB': '.{ — start of a struct (map)',
            '#OS': '.{ — start of a tuple (list)',
            '#CS': '} — end of a tuple (list)',
            'KEY': 'field name: .identifier or .@"..." (dot stripped)',
            'VAL': 'value: number, string, true/false/null, or .enum',
          })
        },
      },
    },
  })

  // If enumTag is set, wrap enum-literal values (produced by zonDot) into
  // `{ [enumTag]: name }` objects. The relaxed-JSON grammar takes ownership
  // of the `@val-bc` (val close) phase via `@val-bc/replace`, which sets
  // r.node from the matched token; once a phase is "replaced" the engine
  // suppresses any `/prepend` on it. So run in the `@val-ac` (after-close)
  // phase and rewrap the node the close handler produced from the enum token.
  const refs: Record<string, Function> = {
    '@val-ac': (r: Rule, _ctx: Context) => {
      if (!enumTag) return
      if (undefined !== r.child.node) return
      if (0 === r.os) return
      const tkn: any = r.o0
      if (tkn && tkn.use && tkn.use.zonEnum) {
        r.node = { [enumTag]: tkn.val }
      }
    },

    // Zig rejects a struct literal that repeats a field name. jsonic's own
    // `@pair-bc` performs the assignment (last one wins), so this guard has
    // to run *before* it — hence `/prepend`, which is available because
    // jsonic declares a plain `@pair-bc` rather than taking the phase with
    // `/replace` (contrast `@val-bc`, see the note above).
    '@pair-bc/prepend': (r: Rule, ctx: Context) => {
      if (!r.u.pair) return
      const node: any = r.node
      if (null == node || 'object' !== typeof node) return
      const key = r.u.key
      if (key in node) {
        const tkn: any = r.o0 || ctx.t0
        tkn.err = 'zon_dup_field'
        tkn.use = Object.assign(tkn.use || {}, { key })
        return tkn
      }
    },
  }

  const grammarDef = new Tabnas().use(jsonic).parse(grammarText)
  grammarDef.ref = refs
  // All jsonic option overrides live on the grammar object so the plugin
  // applies them atomically alongside its rule alts.
  grammarDef.options = {
    rule: {
      // Remove jsonic extensions (implicit maps/lists, top-level commas,
      // path dives). ZON uses explicit struct literals only.
      exclude: 'jsonic,imp',
      start: 'val',
    },
    fixed: {
      token: {
        // Bare `{`, `[`, `]` are not valid in ZON. Struct opening is `.{`
        // which is handled by the custom zonDot lex matcher below.
        '#OB': null,
        '#OS': null,
        '#CS': null,
        // `=` replaces `:` as the key/value separator.
        '#CL': '=',
      },
    },
    tokenSet: {
      // ZON field names are identifiers only.
      KEY: ['#TX'],
    },
    string: {
      chars: '"',
      multiChars: '',
      // Zig-flavoured escape sequences.
      escape: {
        n: '\n',
        r: '\r',
        t: '\t',
        '\\': '\\',
        '"': '"',
        '\'': '\'',
      },
      allowUnknown: false,
    },
    // jsonic's relaxed number lexer accepts `+1`, `.5`, `5.`, `0123`,
    // `1__0` and friends, none of which are ZON. The zonNumber matcher
    // below implements Zig's numeric literal grammar exactly instead.
    number: {
      lex: false,
    },
    error: {
      zon_number: 'invalid ZON number literal: {src}',
      zon_ident: 'invalid ZON identifier: {src}',
      zon_char: 'invalid ZON character literal: {src}',
      zon_doc_comment: 'doc comments are not allowed in ZON: {src}',
      zon_dup_field: 'duplicate struct field name: {src}',
    },
    // Only `//` line comments in ZON.
    comment: {
      lex: true,
      def: {
        hash: { lex: false },
        slash: { line: true, start: '//', lex: true, eatline: false },
        multi: { lex: false },
      },
    },
    value: {
      lex: true,
      def: {
        true: { val: true },
        false: { val: false },
        null: { val: null },
      },
    },
    // The default jsonic text matcher is disabled; identifiers are only
    // produced by the custom zonDot matcher below.
    text: {
      lex: false,
    },
    lex: {
      match: {
        zonDot: { order: 1e5, make: buildZonDotMatcher() },
        zonMultiString: { order: 1.1e5, make: buildZonMultiStringMatcher() },
        zonChar: { order: 1.2e5, make: buildZonCharMatcher(charAsNumber) },
        zonNumber: { order: 1.3e5, make: buildZonNumberMatcher() },
        // Must out-order jsonic's comment matcher (6e6) so `//!` and `///`
        // are rejected instead of being eaten as ordinary line comments.
        zonDocComment: { order: 1.4e5, make: buildZonDocCommentMatcher() },
      },
    },
  }

  // Tag every alt in this grammar with the 'zon' group so callers can
  // selectively exclude zon alts via `rule.exclude: 'zon'`.
  tn.grammar(grammarDef, { rule: { alt: { g: 'zon' } } })
}

// Custom lex matcher for `.`-prefixed tokens.
//   `.{`            -> #OB if followed by `.ident =`, otherwise #OS
//   `.identifier`   -> #TX (val = identifier, use.zonEnum = true)
//   `.@"any name"`  -> #TX (val = the decoded string, use.zonEnum = true)
// Runs ahead of the fixed-token matcher so it reliably owns the `.` prefix.
function buildZonDotMatcher() {
  return function makeZonDotMatcher(cfg: Config, _opts: TabnasOptions) {
    return function zonDotMatcher(lex: Lex) {
      const { pnt } = lex
      const src: string = lex.src as unknown as string
      const { sI, cI, rI } = pnt
      if ('.' !== src[sI]) return undefined

      // Zig's tokenizer emits `.` and what follows as separate tokens, so
      // whitespace and comments may sit between them (`. foo`, `. {}`).
      const pos = skipInsigPos(cfg, src, sI + 1, rI, cI + 1)
      const dI = pos.i

      // Advance pnt past a token running from sI to `end`, given that the
      // insignificant run up to dI may have crossed rows.
      const advance = (end: number) => {
        pnt.sI = end
        pnt.rI = pos.rI
        pnt.cI = pos.cI + (end - dI)
      }

      // `.{` opens a struct literal. Decide map vs list by peeking ahead.
      if ('{' === src[dI]) {
        const isMap = peekIsMapOpen(cfg, src, dI + 1)
        const tin = isMap ? '#OB' : '#OS'
        const tkn = lex.token(tin, undefined, src.substring(sI, dI + 1), pnt)
        advance(dI + 1)
        return tkn
      }

      // `.@"..."` - escaped identifier: any string may name a field or an
      // enum literal. Zig rejects an empty one (`.@""`).
      if ('@' === src[dI] && '"' === src[dI + 1]) {
        const dec = decodeZigString(src, dI + 2)
        // Zig rejects an empty identifier and one containing a null byte.
        if (null === dec || '' === dec.val || -1 !== dec.val.indexOf('\u0000')) {
          return lex.bad('zon_ident', sI, null === dec ? dI + 2 : dec.end)
        }
        const srcText = src.substring(sI, dec.end)
        const tkn = lex.token('#TX', dec.val, srcText, pnt, { zonEnum: true })
        advance(dec.end)
        return tkn
      }

      // `.identifier` - field name or enum literal.
      if (!isIdStart(src.charCodeAt(dI))) return undefined
      let eI = dI
      while (eI < src.length && isIdCont(src.charCodeAt(eI))) eI++

      const srcText = src.substring(sI, eI)
      const name = src.substring(dI, eI)
      const tkn = lex.token('#TX', name, srcText, pnt, { zonEnum: true })
      advance(eI)
      return tkn
    }
  }
}

// Skip whitespace, newlines and `//` line comments, tracking the row/column
// so a token that follows still reports an accurate source position.
function skipInsigPos(
  cfg: Config,
  src: string,
  i: number,
  rI: number,
  cI: number,
): { i: number; rI: number; cI: number } {
  while (i < src.length) {
    const c = src[i]
    if (cfg.line.chars[c]) {
      if (cfg.line.rowChars[c]) rI++
      cI = 1
      i++
    } else if (' ' === c || '\t' === c) {
      i++
      cI++
    } else if ('/' === c && '/' === src[i + 1]) {
      while (i < src.length && !cfg.line.chars[src[i]]) {
        i++
        cI++
      }
    } else {
      break
    }
  }
  return { i, rI, cI }
}

// Returns true if the position inside `.{ ... }` starts with a field name
// (`.ident` or `.@"..."`) followed by `=`, indicating a struct/map literal
// rather than a tuple/list.
function peekIsMapOpen(cfg: Config, src: string, start: number): boolean {
  let i = skipInsig(cfg, src, start)
  if ('.' !== src[i]) return false
  i = skipInsig(cfg, src, i + 1)
  if ('@' === src[i] && '"' === src[i + 1]) {
    const dec = decodeZigString(src, i + 2)
    if (null === dec) return false
    i = dec.end
  } else {
    if (!isIdStart(src.charCodeAt(i))) return false
    i++
    while (i < src.length && isIdCont(src.charCodeAt(i))) i++
  }
  i = skipInsig(cfg, src, i)
  return '=' === src[i]
}

// Decode a Zig double-quoted string body starting at `i` (just past the
// opening quote). Returns the decoded value and the index just past the
// closing quote, or null when the literal is malformed. Used for `.@"..."`
// escaped identifiers; ordinary string literals are lexed by jsonic.
function decodeZigString(
  src: string,
  i: number,
): { val: string; end: number } | null {
  let out = ''
  while (i < src.length) {
    const c = src[i]
    if ('"' === c) return { val: out, end: i + 1 }
    if ('\n' === c || '\r' === c) return null
    if ('\\' === c) {
      const e = src[i + 1]
      if ('x' === e) {
        const hex = src.substring(i + 2, i + 4)
        if (!/^[0-9a-fA-F]{2}$/.test(hex)) return null
        out += String.fromCharCode(parseInt(hex, 16))
        i += 4
      } else if ('u' === e) {
        if ('{' !== src[i + 2]) return null
        const endI = src.indexOf('}', i + 3)
        if (-1 === endI) return null
        const hex = src.substring(i + 3, endI)
        if (!/^[0-9a-fA-F]+$/.test(hex)) return null
        const cp = parseInt(hex, 16)
        if (0x10ffff < cp) return null
        out += String.fromCodePoint(cp)
        i = endI + 1
      } else if (undefined !== ZIG_ESCAPE[e]) {
        out += ZIG_ESCAPE[e]
        i += 2
      } else {
        return null
      }
    } else {
      out += c
      i++
    }
  }
  return null
}

// The single-character Zig escapes (the `\xNN` and `\u{...}` forms are
// handled positionally by decodeZigString).
const ZIG_ESCAPE: Record<string, string> = {
  n: '\n',
  r: '\r',
  t: '\t',
  '\\': '\\',
  '\'': '\'',
  '"': '"',
}

// Skip whitespace, newlines, and `//` line comments (position only).
function skipInsig(cfg: Config, src: string, i: number): number {
  return skipInsigPos(cfg, src, i, 1, 1).i
}

function isIdStart(c: number): boolean {
  return (
    (65 <= c && c <= 90) || // A-Z
    (97 <= c && c <= 122) || // a-z
    95 === c // _
  )
}

function isIdCont(c: number): boolean {
  return (
    (48 <= c && c <= 57) || // 0-9
    (65 <= c && c <= 90) ||
    (97 <= c && c <= 122) ||
    95 === c
  )
}

// Multi-line Zig strings: consecutive lines starting with `\\`.
// Each `\\` line contributes its content verbatim (after the `\\`); lines
// are joined with `\n`.
function buildZonMultiStringMatcher() {
  return function makeZonMultiStringMatcher(cfg: Config, _opts: TabnasOptions) {
    return function zonMultiStringMatcher(lex: Lex) {
      const { pnt } = lex; const src: string = lex.src as unknown as string
      if ('\\' !== src[pnt.sI] || '\\' !== src[pnt.sI + 1]) return undefined

      const startI = pnt.sI
      const startCI = pnt.cI
      let sI = pnt.sI
      let rI = pnt.rI
      const parts: string[] = []

      while ('\\' === src[sI] && '\\' === src[sI + 1]) {
        sI += 2
        const lineStart = sI
        while (sI < src.length && !cfg.line.chars[src[sI]]) sI++
        parts.push(src.substring(lineStart, sI))

        // Consume line terminator (handle \r\n as one).
        if (sI < src.length && cfg.line.chars[src[sI]]) {
          const ch = src[sI]
          if (cfg.line.rowChars[ch]) rI++
          sI++
          if (sI < src.length && '\r' === ch && '\n' === src[sI]) sI++
        }

        // Look for another `\\` continuation. Zig's tokenizer treats the
        // whole run of `\\` lines as ONE token and skips the whitespace
        // between them, so blank lines in the middle continue the literal
        // (and contribute nothing) rather than ending it.
        let peek = sI
        let peekR = 0
        while (peek < src.length) {
          const pc = src[peek]
          if (' ' === pc || '\t' === pc) {
            peek++
          } else if (cfg.line.chars[pc]) {
            if (cfg.line.rowChars[pc]) peekR++
            peek++
          } else {
            break
          }
        }
        if ('\\' !== src[peek] || '\\' !== src[peek + 1]) break
        sI = peek
        rI += peekR
      }

      const val = parts.join('\n')
      const tsrc = src.substring(startI, sI)
      const tkn = lex.token('#ST', val, tsrc, pnt)
      pnt.sI = sI
      pnt.rI = rI
      pnt.cI = startCI + (sI - startI)
      return tkn
    }
  }
}

// Zig character literal: `'x'`, `'\n'`, `'\x41'`, `'\u{1F600}'`.
// Produces a numeric code point (if charAsNumber) or a one-char string.
function buildZonCharMatcher(charAsNumber: boolean) {
  return function makeZonCharMatcher(_cfg: Config, _opts: TabnasOptions) {
    return function zonCharMatcher(lex: Lex) {
      const { pnt } = lex; const src: string = lex.src as unknown as string
      const { sI, cI } = pnt
      if ('\'' !== src[sI]) return undefined

      let i = sI + 1
      let codepoint: number | null = null

      if ('\\' === src[i]) {
        i++
        const esc = src[i]
        switch (esc) {
          case 'n': codepoint = 10; i++; break
          case 'r': codepoint = 13; i++; break
          case 't': codepoint = 9; i++; break
          case '\\': codepoint = 92; i++; break
          case '\'': codepoint = 39; i++; break
          case '"': codepoint = 34; i++; break
          case '0': codepoint = 0; i++; break
          case 'x': {
            i++
            const hex = src.substring(i, i + 2)
            if (!/^[0-9a-fA-F]{2}$/.test(hex)) return undefined
            codepoint = parseInt(hex, 16)
            i += 2
            break
          }
          case 'u': {
            i++
            if ('{' !== src[i]) return undefined
            i++
            const endI = src.indexOf('}', i)
            if (-1 === endI) return undefined
            const hex = src.substring(i, endI)
            if (!/^[0-9a-fA-F]+$/.test(hex)) return undefined
            codepoint = parseInt(hex, 16)
            // Zig: the escape must name a valid unicode scalar value.
            if (0x10ffff < codepoint) return lex.bad('zon_char', sI, endI + 2)
            i = endI + 1
            break
          }
          default:
            return undefined
        }
      } else if (src[i] && '\'' !== src[i]) {
        codepoint = src.codePointAt(i) as number
        // A raw control character (notably a literal newline) is not a
        // character literal in Zig — it is an invalid token.
        if (codepoint < 0x20 || 0x7f === codepoint) {
          return lex.bad('zon_char', sI, i + 1)
        }
        i += codepoint > 0xffff ? 2 : 1
      } else {
        return undefined
      }

      if ('\'' !== src[i]) return undefined
      i++

      const tsrc = src.substring(sI, i)
      const val = charAsNumber ? codepoint : String.fromCodePoint(codepoint!)
      const tkn = lex.token('#NR', val, tsrc, pnt)
      pnt.sI = i
      pnt.cI = cI + (i - sI)
      return tkn
    }
  }
}

// `//!` and `///` are Zig DOC comments, which ZON rejects outright ("expected
// expression, found 'a document comment'"). `////` and longer runs are plain
// line comments. This matcher only ever fails the lex: an ordinary `//`
// comment falls through to jsonic's comment matcher.
function buildZonDocCommentMatcher() {
  return function makeZonDocCommentMatcher(_cfg: Config, _opts: TabnasOptions) {
    return function zonDocCommentMatcher(lex: Lex) {
      const { pnt } = lex
      const src: string = lex.src as unknown as string
      const sI = pnt.sI
      if ('/' !== src[sI] || '/' !== src[sI + 1]) return undefined
      const c = src[sI + 2]
      if ('!' === c || ('/' === c && '/' !== src[sI + 3])) {
        return lex.bad('zon_doc_comment', sI, sI + 3)
      }
      return undefined
    }
  }
}

// Zig numeric literals, as ZON defines them. This replaces jsonic's relaxed
// number lexer (`number.lex: false`) because that one happily accepts `+1`,
// `.5`, `5.`, `0123`, `00`, `1__0` and `0x_2A`, none of which are ZON.
//
// Accepted: decimal / `0x` / `0o` / `0b` integers with `_` separators between
// digits, decimal and hexadecimal floats (`1.5e3`, `0x1.8p1`, `0x103.70`),
// the `inf` and `nan` keywords, and a leading `-` on any of those except
// `nan`. Integers whose exact value is not representable as an IEEE-754
// double are returned as a `bigint` rather than silently rounded.
function buildZonNumberMatcher() {
  return function makeZonNumberMatcher(_cfg: Config, _opts: TabnasOptions) {
    return function zonNumberMatcher(lex: Lex) {
      const { pnt } = lex
      const src: string = lex.src as unknown as string
      const { sI, cI } = pnt

      let i = sI
      let neg = false
      if ('-' === src[sI]) {
        neg = true
        i++
        while (' ' === src[i] || '\t' === src[i]) i++
      }

      const c = src[i]
      if (undefined === c) return undefined

      // The `inf` / `nan` keywords. Zig allows `-inf` but not `-nan`.
      if ('i' === c || 'n' === c) {
        const kw = src.substring(i, i + 3)
        if (('inf' === kw || 'nan' === kw) && !isIdCont(src.charCodeAt(i + 3))) {
          if ('nan' === kw && neg) return lex.bad('zon_number', sI, i + 3)
          const val = 'inf' === kw ? (neg ? -Infinity : Infinity) : NaN
          const end = i + 3
          const tkn = lex.token('#NR', val, src.substring(sI, end), pnt)
          pnt.sI = end
          pnt.cI = cI + (end - sI)
          return tkn
        }
        return undefined
      }

      if (!isDigit(c.charCodeAt(0))) return undefined

      const num = scanZonNumber(src, i)
      if (num.err) return lex.bad('zon_number', sI, num.end)

      let val: any
      if (num.isInt) {
        const big = num.big as bigint
        // Zig: `-0` is an ambiguous integer literal (`-0.0` is fine).
        if (neg && 0n === big) return lex.bad('zon_number', sI, num.end)
        const signed = neg ? -big : big
        const asNum = Number(signed)
        val =
          Number.isFinite(asNum) && BigInt(asNum) === signed ? asNum : signed
      } else {
        val = neg ? -(num.num as number) : (num.num as number)
      }

      const end = num.end
      const tkn = lex.token('#NR', val, src.substring(sI, end), pnt)
      pnt.sI = end
      pnt.cI = cI + (end - sI)
      return tkn
    }
  }
}

type ZonNumScan = {
  end: number
  err?: boolean
  isInt?: boolean
  big?: bigint
  num?: number
}

// Scan one unsigned Zig numeric literal starting at `start` (a digit).
function scanZonNumber(src: string, start: number): ZonNumScan {
  let i = start
  let base = 10
  const fail = (): ZonNumScan => ({ end: scanNumTokenEnd(src, start), err: true })

  if ('0' === src[i]) {
    const c1 = src[i + 1]
    if ('x' === c1 || 'o' === c1 || 'b' === c1) {
      base = 'x' === c1 ? 16 : 'o' === c1 ? 8 : 2
      i += 2
    } else if ('X' === c1 || 'O' === c1 || 'B' === c1) {
      return fail() // base prefix must be lowercase
    } else if (undefined !== c1 && ('_' === c1 || isDigit(c1.charCodeAt(0)))) {
      return fail() // leading zero
    }
  }

  const intRun = scanDigitRun(src, i, base)
  if (intRun.bad) return fail()
  const intText = src.substring(i, intRun.end).replace(/_/g, '')
  i = intRun.end

  let isFloat = false
  let fracText = ''
  if ('.' === src[i]) {
    const after = src[i + 1]
    const afterDV = undefined === after ? -1 : digitVal(after)
    // A `.` only starts a fraction when a digit of this base (or, for hex,
    // the `p` exponent) follows; otherwise it is a stray token and the
    // number ends here — `1.` and `0.1.2` are rejected by the parser.
    const startsFrac =
      (0 <= afterDV && afterDV < base) ||
      (16 === base && ('p' === after || 'P' === after))
    if (startsFrac) {
      if (16 !== base && 10 !== base) return fail() // invalid base for float
      isFloat = true
      i++
      const fracRun = scanDigitRun(src, i, base)
      if (fracRun.bad) return fail()
      fracText = src.substring(i, fracRun.end).replace(/_/g, '')
      i = fracRun.end
    }
  }

  let expVal = 0
  let hasExp = false
  const expChar = 16 === base ? 'pP' : 10 === base ? 'eE' : ''
  if ('' !== expChar && undefined !== src[i] && -1 !== expChar.indexOf(src[i])) {
    hasExp = true
    isFloat = true
    i++
    let expSign = 1
    if ('+' === src[i] || '-' === src[i]) {
      if ('-' === src[i]) expSign = -1
      i++
    }
    const expRun = scanDigitRun(src, i, 10)
    if (expRun.bad || 0 === expRun.count) return fail()
    expVal = expSign * parseInt(src.substring(i, expRun.end).replace(/_/g, ''), 10)
    i = expRun.end
  }

  if (0 === intRun.count && 0 === fracText.length) return fail()

  // Anything alphanumeric still attached is a digit invalid for this base.
  if (undefined !== src[i] && isIdCont(src.charCodeAt(i))) return fail()

  if (isFloat) {
    let num: number
    if (16 === base) {
      const mant = Number(BigInt('0x' + ((intText || '0') + fracText)))
      num = mant * Math.pow(2, expVal - 4 * fracText.length)
    } else {
      num = parseFloat(
        (intText || '0') +
        (0 < fracText.length ? '.' + fracText : '') +
        (hasExp ? 'e' + expVal : ''),
      )
    }
    return { end: i, num }
  }

  const prefix = 16 === base ? '0x' : 8 === base ? '0o' : 2 === base ? '0b' : ''
  return { end: i, isInt: true, big: BigInt(prefix + (intText || '0')) }
}

// Scan a run of `base` digits with Zig's digit-separator rules: a `_` must
// sit directly between two digits (no leading, trailing, or repeated `_`).
function scanDigitRun(
  src: string,
  i: number,
  base: number,
): { end: number; count: number; bad: boolean } {
  const start = i
  let count = 0
  let prevDigit = false
  for (; i < src.length; i++) {
    const ch = src[i]
    if ('_' === ch) {
      if (!prevDigit) return { end: i, count, bad: true }
      prevDigit = false
      continue
    }
    const dv = digitVal(ch)
    if (0 <= dv && dv < base) {
      count++
      prevDigit = true
      continue
    }
    break
  }
  if (start < i && !prevDigit) return { end: i, count, bad: true }
  return { end: i, count, bad: false }
}

// The greedy extent of a malformed numeric token, so the error span covers
// the whole literal rather than its first character.
function scanNumTokenEnd(src: string, i: number): number {
  while (i < src.length) {
    const c = src.charCodeAt(i)
    if (isIdCont(c)) {
      i++
    } else if (46 === c && i + 1 < src.length && isIdCont(src.charCodeAt(i + 1))) {
      i++
    } else {
      break
    }
  }
  return i
}

function isDigit(c: number): boolean {
  return 48 <= c && c <= 57
}

function digitVal(ch: string): number {
  const c = ch.charCodeAt(0)
  if (48 <= c && c <= 57) return c - 48
  if (97 <= c && c <= 102) return c - 87
  if (65 <= c && c <= 70) return c - 55
  return -1
}

// Default option values.
Zon.defaults = {
  charAsNumber: false,
  enumTag: null,
} as ZonOptions

export { Zon, VERSION }
export type { ZonOptions }

// VERSION is this package's version. It MUST equal package.json "version":
// the release orchestrator rewrites both, and the version test fails the
// build if they drift. Mirrors `const VERSION` in go/zon.go.
const VERSION = '0.5.6'
