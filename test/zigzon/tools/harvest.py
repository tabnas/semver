#!/usr/bin/env python3
"""Harvest every ZON document out of a checked-out zig source tree.

Two sources, both from the pinned release:

  * every `*.zon` file in the tree (build.zig.zon manifests, the
    `test/behavior/zon` and `test/cases/compile_errors/zon` fixtures, and the
    big `src/codegen/*/{instructions,encodings}.zon` tables)
  * every ZON snippet passed to `fromSlice` / `fromSliceAlloc` in
    `lib/std/zon/parse.zig` — the reference parser's own test corpus

Documents are de-duplicated by content, keeping the first in sorted order, and
written to stdout as a JSON array of {origin, source}. `fetch-zigzon.sh` feeds
them to the oracle, which supplies every verdict and value.
"""

import json
import os
import re
import sys

CALL = re.compile(r"\bfromSlice(?:Alloc)?\(")


def decode_zig_string(s, i):
    """Decode the zig string literal starting at s[i] == '"'."""
    out = []
    i += 1
    while i < len(s):
        c = s[i]
        if c == '"':
            return "".join(out), i + 1
        if c != "\\":
            out.append(c)
            i += 1
            continue
        e = s[i + 1]
        if e == "n":
            out.append("\n"); i += 2
        elif e == "r":
            out.append("\r"); i += 2
        elif e == "t":
            out.append("\t"); i += 2
        elif e in "\\'\"":
            out.append(e); i += 2
        elif e == "x":
            out.append(chr(int(s[i + 2:i + 4], 16))); i += 4
        elif e == "u":
            j = s.index("}", i)
            out.append(chr(int(s[i + 3:j], 16))); i = j + 1
        else:
            return None, i
    return None, i


def split_args(src, i):
    """Split the argument list that starts just after `(` at src[i].

    Returns the list of raw argument slices. String and character literals are
    atomic, and a `\\\\` multiline-string line runs to the end of its line.
    """
    depth = 0
    args = []
    start = i
    while i < len(src):
        c = src[i]
        if c == '"':
            _, i = decode_zig_string(src, i)
            continue
        if c == "'":
            j = i + 1
            while j < len(src) and src[j] != "'":
                j += 2 if src[j] == "\\" else 1
            i = j + 1
            continue
        if src.startswith("\\\\", i):
            i = src.find("\n", i)
            if i < 0:
                break
            continue
        if c in "([{":
            depth += 1
        elif c in ")]}":
            if depth == 0:
                args.append(src[start:i])
                return args
            depth -= 1
        elif c == "," and depth == 0:
            args.append(src[start:i])
            start = i + 1
        i += 1
    return args


def snippet_from_arg(arg):
    """Turn the raw third argument of a fromSlice call into ZON source."""
    stripped = arg.strip()
    if stripped.startswith('"'):
        val, _ = decode_zig_string(stripped, 0)
        return val
    lines = [l.strip() for l in arg.split("\n")]
    parts = [l[2:] for l in lines if l.startswith("\\\\")]
    return "\n".join(parts) if parts else None


def harvest_parse_zig(path, origin_prefix):
    src = open(path, encoding="utf8").read()
    out = []
    for m in CALL.finditer(src):
        args = split_args(src, m.end())
        if len(args) < 3:
            continue
        snippet = snippet_from_arg(args[2])
        if snippet is None:
            continue
        line = src.count("\n", 0, m.start()) + 1
        out.append({"origin": f"{origin_prefix}:{line}", "source": snippet})
    return out


def harvest_zon_files(root):
    out = []
    for dirpath, _dirnames, filenames in os.walk(root):
        for name in filenames:
            if not name.endswith(".zon"):
                continue
            full = os.path.join(dirpath, name)
            rel = os.path.relpath(full, root).replace(os.sep, "/")
            out.append({"origin": rel, "source": open(full, encoding="utf8").read()})
    out.sort(key=lambda c: c["origin"])
    return out


def main():
    root = sys.argv[1]
    cases = harvest_zon_files(root)
    parse_zig = os.path.join(root, "lib", "std", "zon", "parse.zig")
    if os.path.exists(parse_zig):
        cases += harvest_parse_zig(parse_zig, "lib/std/zon/parse.zig")

    seen = set()
    unique = []
    for c in cases:
        if c["source"] in seen:
            continue
        seen.add(c["source"])
        c["name"] = c["origin"]
        unique.append(c)
    json.dump(unique, sys.stdout)


if __name__ == "__main__":
    main()
