#!/usr/bin/env python3
"""Ask the oracle for a verdict on every document and write the corpora.

Driven by scripts/fetch-zigzon.sh. Two inputs:

  --harvest  the JSON array of {origin, name, source} produced by harvest.py
  --inputs   test/strictness/inputs.txt, one locally authored probe per line
             (escapes \\n \\r \\t \\\\, '#'-comments and blank lines skipped)

Both are fed to the oracle over its length-prefixed stdin protocol, and the
verdicts are written verbatim: this script never decides whether a document is
valid ZON, and never computes a value.
"""

import argparse
import json
import os
import subprocess
import sys


def unescape(line):
    out = []
    i = 0
    while i < len(line):
        c = line[i]
        if c == "\\" and i + 1 < len(line):
            n = line[i + 1]
            if n in "nrt\\":
                out.append({"n": "\n", "r": "\r", "t": "\t", "\\": "\\"}[n])
                i += 2
                continue
        out.append(c)
        i += 1
    return "".join(out)


def read_probes(path):
    probes = []
    with open(path, encoding="utf8") as fh:
        for line in fh.read().split("\n"):
            if line == "" or line.startswith("#"):
                continue
            probes.append(unescape(line))
    return probes


def judge(oracle, sources):
    payload = b"".join(
        str(len(s.encode("utf8"))).encode() + b"\n" + s.encode("utf8")
        for s in sources
    )
    proc = subprocess.run([oracle], input=payload, stdout=subprocess.PIPE, check=True)
    lines = proc.stdout.decode("utf8").splitlines()
    if len(lines) != len(sources):
        raise SystemExit(
            f"oracle returned {len(lines)} verdicts for {len(sources)} inputs"
        )
    return [json.loads(l) for l in lines]


def provenance(generator, note):
    return {
        "repo": os.environ.get("ZIG_REPO", ""),
        "commit": os.environ.get("ZIG_COMMIT", ""),
        "zig": os.environ.get("ZIG_VERSION", ""),
        "generator": generator,
        "note": note,
    }


def write(path, source, cases):
    with open(path, "w", encoding="utf8") as fh:
        json.dump({"source": source, "cases": cases}, fh, indent=1)
        fh.write("\n")
    valid = sum(1 for c in cases if c["valid"])
    print(f"  {path}: {len(cases)} cases ({valid} valid, {len(cases) - valid} invalid)")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--oracle", required=True)
    ap.add_argument("--harvest", required=True)
    ap.add_argument("--inputs", required=True)
    ap.add_argument("--zigzon-out", required=True)
    ap.add_argument("--strictness-out", required=True)
    args = ap.parse_args()

    harvested = json.load(open(args.harvest, encoding="utf8"))
    verdicts = judge(args.oracle, [c["source"] for c in harvested])
    cases = []
    for c, v in zip(harvested, verdicts):
        case = {"name": c["name"], "origin": c["origin"], "source": c["source"],
                "valid": v["ok"]}
        if v["ok"]:
            case["value"] = v["value"]
        else:
            case["error"] = v["error"]
        cases.append(case)
    write(args.zigzon_out,
          provenance("scripts/fetch-zigzon.sh + test/zigzon/tools/",
                     "GENERATED, NEVER COMMITTED. Regenerate with scripts/fetch-zigzon.sh."),
          cases)

    probes = read_probes(args.inputs)
    verdicts = judge(args.oracle, probes)
    cases = []
    for s, v in zip(probes, verdicts):
        case = {"source": s, "valid": v["ok"]}
        if v["ok"]:
            case["value"] = v["value"]
        else:
            case["error"] = v["error"]
        cases.append(case)
    src = provenance("scripts/fetch-zigzon.sh + test/zigzon/tools/",
                     "GENERATED, NEVER COMMITTED. Regenerate with scripts/fetch-zigzon.sh.")
    src["inputs"] = "test/strictness/inputs.txt (locally authored)"
    write(args.strictness_out, src, cases)


if __name__ == "__main__":
    main()
