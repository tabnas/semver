# Agents Guide — shared spec fixtures

`spec/*.tsv` holds the cross-runtime conformance fixtures. Both runtimes
auto-discover and run **every** file in this directory, so a change here
affects TypeScript and Go together — edit with that in mind.

`zigzon/` and `strictness/` hold the generated zig-reference corpora. What is
tracked there is the instrument, never the corpus: `zigzon/tools/` (the
oracle and its harness) and `strictness/inputs.txt` (our own probe inputs).
Both `cases.json` files are `.gitignore`d and are rebuilt from the pinned
ziglang/zig 0.16.0 release by `scripts/fetch-zigzon.sh`.

## The instrument's own rules

- **The corpora are built automatically, not opt-in.** `pretest` in
  `ts/package.json` and `TestMain` in `go/zigzon_test.go` both run
  `scripts/fetch-zigzon.sh` before grading, so the suites run in CI as well
  as locally. Do not remove either hook.
- **A missing corpus is a FAILURE, not a skip.** The only skip permitted is
  the platform one: a host with no pinned zig oracle toolchain
  (anything but linux/macos on x86_64/aarch64) reports one explicit,
  platform-named skip. Never widen that carve-out to cover a missing file.
- **The census is pinned** — 184 valid / 44 invalid in `zigzon`, 45 / 72 in
  `strictness`. If it fails, find out what changed in the generator; do not
  edit the number to match.
- **Every download is SHA-256 pinned.** A mismatch is a hard failure, never
  something to work around.
- Do not shrink a corpus, add a skip list, narrow the option set, or loosen
  the value comparison to improve a number. A conformance figure that cannot
  fail is worth nothing.

## Format

Tab-separated, one case per line, with a header row naming the columns.
Blank lines are skipped, and so are comment lines — a line starting with
`#` that contains no tab. (A data row always has at least one tab, so a
`#`-leading source such as a C preprocessor directive still works.)

| Column | Meaning |
|---|---|
| `input` | ZON source. Escapes `\n` `\r` `\t` `\\` are decoded. |
| `expected` | A JSON value (the parse result), or `ERROR` / `ERROR:<code>` for inputs that must fail. The code is compared **exactly** — it is the error's code, not a substring of its message. |
| `opts` | Optional JSON object of plugin options (empty means defaults). |

`expected` and `opts` are **not** escape-decoded — they are raw JSON, so
JSON's own escape rules apply (`"a\nb"` is a string containing a newline).
To put a literal backslash in `input`, write `\\`.

Results are compared after a JSON round-trip, so key order and the
`OrderedMap` / null-prototype-object representations do not affect the
comparison.

## Who runs what

- TypeScript: `ts/test/parity.test.ts` — `makeRunner(...).dir(...)`.
- Go: `go/parity_test.go` — `support.Runner{...}.Dir(t, dir)`.

Both are a dozen lines holding only what is specific to zon: how to build
the parser for a row's options. Everything else — finding `test/spec`,
reading the file, decoding escapes, the `ERROR:` contract, the comparison,
the `<file>:<line>` in a failure message — comes from
[`@tabnas/support`](https://github.com/tabnas/support) and its Go half, so
the two loaders cannot drift from each other either.

Both discover files by directory listing: adding a `.tsv` here runs it in
both runtimes without touching either runner. An empty fixture, and a spec
directory with no fixtures in it, both **fail** — a runner that reports
green having run nothing is indistinguishable from coverage that was never
there.

## Rules

- Prefer adding a fixture here over a one-off in-language assertion when a
  case is expressible as input → output. That is what keeps the two
  runtimes honest against each other.
- What a fixture **cannot** express, because both runners compare after a
  JSON round-trip: `bigint` / `*big.Int` values, `Infinity`, `NaN`, and the
  `-0` / `0` distinction. Those live in `ts/test/zon.test.ts` and
  `go/zon_test.go`, mirrored case for case.
- [`strict.tsv`](spec/strict.tsv) collects the inputs the Zig reference
  implementation REJECTS. Every verdict there came from the oracle in
  `scripts/fetch-zigzon.sh`, not from a judgement call — if you add a row,
  get the verdict the same way.
- TypeScript is canonical. If the two runtimes disagree, the TS behaviour is
  the expected value — unless Go has exposed a genuine TS defect, in which
  case fix TS first and pin the corrected behaviour here.
- A new fixture must pass in BOTH runtimes: run `go test ./...` (from `go/`)
  and `npm test` (from `ts/`) before considering it done.
