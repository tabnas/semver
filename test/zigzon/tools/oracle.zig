//! oracle.zig — the ZON reference implementation, as a batch judge.
//!
//! This program decides what ZON means; @tabnas/zon never does. It runs the
//! pinned Zig toolchain's own `std.zig.Ast` (in `.zon` mode) and
//! `std.zig.ZonGen` over each input and prints the verdict.
//!
//! Protocol (stdin, repeated until EOF), so arbitrary bytes — newlines,
//! NULs, invalid UTF-8 — survive the trip:
//!
//!     <decimal byte length>\n<that many bytes of ZON source>
//!
//! Output (stdout, one line per input):
//!
//!     {"ok":true,"value":<canonical JSON>}
//!     {"ok":false,"error":"<the compiler's own message>"}
//!
//! The canonical JSON encoding keeps the distinctions plain JSON cannot:
//!
//!     .foo                  -> {"$enum":"foo"}
//!     'x'                    -> 120                (a codepoint)
//!     inf / -inf / nan       -> "@inf" / "@-inf" / "@nan"
//!     an integer that is not exactly representable as an f64
//!                            -> {"$big":"<exact decimal>"}
//!
//! Everything else maps to the obvious JSON: struct literals to objects,
//! tuple literals to arrays, strings to strings, true/false/null as-is.

const std = @import("std");
const Ast = std.zig.Ast;
const Zoir = std.zig.Zoir;
const ZonGen = std.zig.ZonGen;

pub fn main(init: std.process.Init) !void {
    const io = init.io;
    const arena = init.arena.allocator();

    var in_buf: [64 * 1024]u8 = undefined;
    var stdin_reader = std.Io.File.stdin().reader(io, &in_buf);
    const stdin = try stdin_reader.interface.allocRemaining(arena, .unlimited);

    var out_buf: [64 * 1024]u8 = undefined;
    var stdout_writer = std.Io.File.stdout().writer(io, &out_buf);
    const stdout = &stdout_writer.interface;

    var out = std.ArrayList(u8).empty;

    var pos: usize = 0;
    while (pos < stdin.len) {
        const nl = std.mem.indexOfScalarPos(u8, stdin, pos, '\n') orelse break;
        const len = try std.fmt.parseInt(usize, stdin[pos..nl], 10);
        pos = nl + 1;
        if (pos + len > stdin.len) break;
        const src = stdin[pos .. pos + len];
        pos += len;

        out.clearRetainingCapacity();
        try judge(arena, src, &out);
        try out.append(arena, '\n');
        try stdout.writeAll(out.items);
    }
    try stdout.flush();
}

fn judge(gpa: std.mem.Allocator, src: []const u8, out: *std.ArrayList(u8)) !void {
    const src0 = try gpa.dupeZ(u8, src);
    defer gpa.free(src0);

    var ast = try Ast.parse(gpa, src0, .zon);
    defer ast.deinit(gpa);

    if (ast.errors.len > 0) {
        var msg: std.Io.Writer.Allocating = .init(gpa);
        defer msg.deinit();
        try ast.renderError(ast.errors[0], &msg.writer);
        try out.appendSlice(gpa, "{\"ok\":false,\"error\":");
        try writeJsonString(gpa, out, msg.written());
        try out.append(gpa, '}');
        return;
    }

    var zoir = try ZonGen.generate(gpa, ast, .{});
    defer zoir.deinit(gpa);

    if (zoir.hasCompileErrors()) {
        const err = zoir.compile_errors[0];
        try out.appendSlice(gpa, "{\"ok\":false,\"error\":");
        try writeJsonString(gpa, out, err.msg.get(zoir));
        try out.append(gpa, '}');
        return;
    }

    try out.appendSlice(gpa, "{\"ok\":true,\"value\":");
    try emit(gpa, out, zoir, .root);
    try out.append(gpa, '}');
}

fn emit(
    gpa: std.mem.Allocator,
    out: *std.ArrayList(u8),
    zoir: Zoir,
    idx: Zoir.Node.Index,
) !void {
    switch (idx.get(zoir)) {
        .true => try out.appendSlice(gpa, "true"),
        .false => try out.appendSlice(gpa, "false"),
        .null => try out.appendSlice(gpa, "null"),
        .pos_inf => try out.appendSlice(gpa, "\"@inf\""),
        .neg_inf => try out.appendSlice(gpa, "\"@-inf\""),
        .nan => try out.appendSlice(gpa, "\"@nan\""),

        .int_literal => |int| switch (int) {
            .small => |v| try out.print(gpa, "{d}", .{v}),
            .big => |v| {
                // An integer is emitted as a plain JSON number only when the
                // exact value survives a round trip through an f64; otherwise
                // it is boxed so no precision is lost in the corpus.
                const f: f64 = v.toFloat(f64, .nearest_even)[0];
                var round_trips = false;
                if (std.math.isFinite(f) and @floor(f) == f) {
                    var limbs: [40]std.math.big.Limb = undefined;
                    var back: std.math.big.int.Mutable = .{
                        .limbs = &limbs,
                        .len = 0,
                        .positive = true,
                    };
                    if (back.setFloat(f, .trunc) == .exact and back.toConst().eql(v)) {
                        round_trips = true;
                    }
                }
                if (round_trips) {
                    try emitFloat(gpa, out, f);
                } else {
                    const text = try v.toStringAlloc(gpa, 10, .lower);
                    defer gpa.free(text);
                    try out.appendSlice(gpa, "{\"$big\":\"");
                    try out.appendSlice(gpa, text);
                    try out.appendSlice(gpa, "\"}");
                }
            },
        },

        .float_literal => |v| try emitFloat(gpa, out, @floatCast(v)),
        .char_literal => |v| try out.print(gpa, "{d}", .{v}),

        .enum_literal => |name| {
            try out.appendSlice(gpa, "{\"$enum\":");
            try writeJsonString(gpa, out, name.get(zoir));
            try out.append(gpa, '}');
        },

        .string_literal => |s| try writeJsonString(gpa, out, s),

        // `.{}` is ambiguous between an empty struct and an empty tuple; the
        // corpus follows @tabnas/zon and treats it as the empty list.
        .empty_literal => try out.appendSlice(gpa, "[]"),

        .array_literal => |range| {
            try out.append(gpa, '[');
            var i: u32 = 0;
            while (i < range.len) : (i += 1) {
                if (i > 0) try out.append(gpa, ',');
                try emit(gpa, out, zoir, range.at(i));
            }
            try out.append(gpa, ']');
        },

        .struct_literal => |lit| {
            try out.append(gpa, '{');
            var i: u32 = 0;
            while (i < lit.vals.len) : (i += 1) {
                if (i > 0) try out.append(gpa, ',');
                try writeJsonString(gpa, out, lit.names[i].get(zoir));
                try out.append(gpa, ':');
                try emit(gpa, out, zoir, lit.vals.at(i));
            }
            try out.append(gpa, '}');
        },
    }
}

fn emitFloat(gpa: std.mem.Allocator, out: *std.ArrayList(u8), v: f64) !void {
    if (std.math.isNan(v)) return out.appendSlice(gpa, "\"@nan\"");
    if (std.math.isPositiveInf(v)) return out.appendSlice(gpa, "\"@inf\"");
    if (std.math.isNegativeInf(v)) return out.appendSlice(gpa, "\"@-inf\"");
    // `{d}` drops the sign of negative zero, but `-0.0` and `0.0` are
    // distinct ZON documents, so spell it out.
    if (v == 0 and std.math.signbit(v)) return out.appendSlice(gpa, "-0.0");
    // Shortest decimal that round-trips. Printed straight into the growable
    // buffer: `{d}` spells large magnitudes out in full (1.23e79 becomes an
    // 80-digit integer), which no fixed stack buffer can hold.
    try out.print(gpa, "{d}", .{v});
}

fn writeJsonString(
    gpa: std.mem.Allocator,
    out: *std.ArrayList(u8),
    s: []const u8,
) !void {
    try out.append(gpa, '"');
    for (s) |c| switch (c) {
        '"' => try out.appendSlice(gpa, "\\\""),
        '\\' => try out.appendSlice(gpa, "\\\\"),
        '\n' => try out.appendSlice(gpa, "\\n"),
        '\r' => try out.appendSlice(gpa, "\\r"),
        '\t' => try out.appendSlice(gpa, "\\t"),
        else => {
            if (c < 0x20) {
                try out.print(gpa, "\\u{x:0>4}", .{c});
            } else {
                try out.append(gpa, c);
            }
        },
    };
    try out.append(gpa, '"');
}
