/* Copyright (c) 2025 Richard Rodger and other contributors, MIT License */

import { describe, test } from 'node:test'
import assert from 'node:assert'

import { Tabnas } from '@tabnas/parser'
import { jsonic } from '@tabnas/jsonic'
import { Zon } from '../dist/zon'

// Jsonic builds maps with Object.create(null); normalise to plain objects so
// assert.deepStrictEqual can compare against JSON-style literals.
function plain(v: any): any {
  if (v === null || typeof v !== 'object') return v
  if (Array.isArray(v)) return v.map(plain)
  const out: Record<string, any> = {}
  for (const k of Object.keys(v)) out[k] = plain((v as any)[k])
  return out
}

function parse(src: string, opts?: any) {
  const j = new Tabnas().use(jsonic).use(Zon, opts || {})
  return plain(j.parse(src))
}

describe('zon', () => {
  test('scalar values', () => {
    assert.strictEqual(parse('42'), 42)
    assert.strictEqual(parse('3.14'), 3.14)
    assert.strictEqual(parse('true'), true)
    assert.strictEqual(parse('false'), false)
    assert.strictEqual(parse('null'), null)
    assert.strictEqual(parse('"hello"'), 'hello')
  })

  test('numeric bases', () => {
    assert.strictEqual(parse('0x2a'), 42)
    assert.strictEqual(parse('0o52'), 42)
    assert.strictEqual(parse('0b101010'), 42)
    assert.strictEqual(parse('1_000_000'), 1000000)
  })

  test('enum literal as bare string', () => {
    assert.strictEqual(parse('.foo'), 'foo')
    assert.strictEqual(parse('.bar_baz'), 'bar_baz')
  })

  test('empty struct', () => {
    assert.deepStrictEqual(parse('.{}'), [])
  })

  test('simple struct', () => {
    assert.deepStrictEqual(parse('.{ .a = 1 }'), { a: 1 })
    assert.deepStrictEqual(parse('.{ .a = 1, .b = 2 }'), { a: 1, b: 2 })
  })

  test('trailing comma in struct', () => {
    assert.deepStrictEqual(parse('.{ .a = 1, }'), { a: 1 })
    assert.deepStrictEqual(parse('.{ .a = 1, .b = 2, }'), { a: 1, b: 2 })
  })

  test('tuple literal', () => {
    assert.deepStrictEqual(parse('.{ 1, 2, 3 }'), [1, 2, 3])
    assert.deepStrictEqual(parse('.{ "a", "b" }'), ['a', 'b'])
  })

  test('trailing comma in tuple', () => {
    assert.deepStrictEqual(parse('.{ 1, 2, 3, }'), [1, 2, 3])
  })

  test('nested struct', () => {
    assert.deepStrictEqual(parse('.{ .a = .{ .b = 1 } }'), { a: { b: 1 } })
  })

  test('nested tuple', () => {
    assert.deepStrictEqual(parse('.{ .{ 1, 2 }, .{ 3, 4 } }'), [
      [1, 2],
      [3, 4],
    ])
  })

  test('mixed nesting', () => {
    assert.deepStrictEqual(
      parse('.{ .xs = .{ 1, 2, 3 }, .y = .{ .z = true } }'),
      { xs: [1, 2, 3], y: { z: true } },
    )
  })

  test('string escapes', () => {
    assert.strictEqual(parse('"a\\nb"'), 'a\nb')
    assert.strictEqual(parse('"a\\tb"'), 'a\tb')
    assert.strictEqual(parse('"a\\\\b"'), 'a\\b')
  })

  test('enum literal as value', () => {
    assert.deepStrictEqual(parse('.{ .kind = .red }'), { kind: 'red' })
  })

  test('comments', () => {
    const src = `.{
      // a comment
      .name = "x", // trailing comment
      .version = "1.0", // version
    }`
    assert.deepStrictEqual(parse(src), { name: 'x', version: '1.0' })
  })

  test('realistic build.zig.zon', () => {
    const src = `.{
      .name = "example",
      .version = "0.0.1",
      .minimum_zig_version = "0.14.0",
      .dependencies = .{
        .foo = .{
          .url = "https://example.com/foo.tar.gz",
          .hash = "1220deadbeef",
        },
      },
      .paths = .{
        "build.zig",
        "src",
        "",
      },
    }`
    assert.deepStrictEqual(parse(src), {
      name: 'example',
      version: '0.0.1',
      minimum_zig_version: '0.14.0',
      dependencies: {
        foo: {
          url: 'https://example.com/foo.tar.gz',
          hash: '1220deadbeef',
        },
      },
      paths: ['build.zig', 'src', ''],
    })
  })

  test('char literal as number', () => {
    assert.strictEqual(parse("'A'", { charAsNumber: true }), 65)
    assert.strictEqual(parse("'\\n'", { charAsNumber: true }), 10)
    assert.strictEqual(parse("'\\u{1F600}'", { charAsNumber: true }), 0x1f600)
  })

  test('char literal as string', () => {
    assert.strictEqual(parse("'A'"), 'A')
  })

  test('multi-line string', () => {
    const src = `.{
      .text = \\\\hello
              \\\\world
      ,
    }`
    assert.deepStrictEqual(parse(src), { text: 'hello\nworld' })
  })

  test('enumTag option', () => {
    const opts = { enumTag: '$enum' }
    assert.deepStrictEqual(parse('.{ .kind = .red }', opts), {
      kind: { $enum: 'red' },
    })
  })

  // The cases below cannot live in `test/spec/*.tsv`: the shared fixtures
  // compare after a JSON round-trip, and bigint / Infinity / NaN / -0 have no
  // JSON spelling. See ../../test/AGENTS.md.

  test('integers beyond double precision keep their exact value', () => {
    // 2^65 - 1 is not representable as an IEEE-754 double, so the plugin
    // returns a bigint rather than silently rounding (zig 0.16.0 reports the
    // same exact value).
    assert.strictEqual(parse('36893488147419103231'), 36893488147419103231n)
    assert.strictEqual(parse('368934_881_474191032_31'), 36893488147419103231n)
    assert.strictEqual(parse('0x1ffffffffffffffff'), 36893488147419103231n)
    assert.strictEqual(parse('0o3777777777777777777777'), 36893488147419103231n)
    assert.strictEqual(
      parse('0b' + '1'.repeat(65)), 36893488147419103231n)
    assert.strictEqual(parse('-36893488147419103231'), -36893488147419103231n)
    assert.strictEqual(parse('9007199254740993'), 9007199254740993n)
  })

  test('integers that fit a double exactly stay numbers', () => {
    assert.strictEqual(parse('9007199254740992'), 9007199254740992)
    assert.strictEqual(parse('18446744073709551616'), 18446744073709551616)
    assert.strictEqual(parse('-36893488147419103232'), -36893488147419103232)
    assert.strictEqual(typeof parse('18446744073709551616'), 'number')
  })

  test('inf and nan literals', () => {
    assert.strictEqual(parse('inf'), Infinity)
    assert.strictEqual(parse('-inf'), -Infinity)
    assert.strictEqual(parse('- inf'), -Infinity)
    assert.ok(Number.isNaN(parse('nan')))
    assert.deepStrictEqual(parse('.{ nan, inf, -inf }').map(Number.isFinite),
      [false, false, false])
    // `-nan` is not a Zig numeric literal.
    assert.throws(() => parse('-nan'))
  })

  test('negative zero', () => {
    assert.ok(Object.is(parse('-0.0'), -0))
    assert.ok(Object.is(parse('-0e0'), -0))
    // But an integer `-0` is ambiguous in Zig and rejected.
    assert.throws(() => parse('-0'))
  })

  test('duplicate struct field names are rejected', () => {
    assert.throws(() => parse('.{ .a = 1, .a = 2 }'), /duplicate struct field/)
    assert.throws(() => parse('.{ .@"a" = 1, .a = 2 }'), /duplicate struct field/)
    // Sibling structs may of course repeat a name.
    assert.deepStrictEqual(parse('.{ .x = .{ .a = 1 }, .y = .{ .a = 2 } }'),
      { x: { a: 1 }, y: { a: 2 } })
  })

  test('doc comments are rejected, ordinary comments are not', () => {
    assert.throws(() => parse('//! doc\n1'), /doc comments/)
    assert.throws(() => parse('/// doc\n1'), /doc comments/)
    assert.strictEqual(parse('//// four\n1'), 1)
    assert.strictEqual(parse('// two\n1'), 1)
  })
})
