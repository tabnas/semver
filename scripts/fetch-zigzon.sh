#!/usr/bin/env bash
#
# fetch-zigzon.sh — build the ZON conformance corpora from the ZIG REFERENCE
# IMPLEMENTATION. Idempotent; nothing it writes is committed.
#
#     bash scripts/fetch-zigzon.sh
#
# What it does:
#
#   1. downloads the pinned zig SOURCE tarball and the pinned prebuilt zig
#      TOOLCHAIN for this host, into test/zigzon/vendor/ (~80 MB, cached)
#   2. builds test/zigzon/tools/oracle.zig with that toolchain — a batch judge
#      wrapping the compiler's own std.zig.Ast + std.zig.ZonGen
#   3. harvests every ZON document in the zig tree (test/zigzon/tools/harvest.py)
#      and asks the oracle for each verdict + value
#        -> test/zigzon/cases.json
#   4. runs the locally authored probes in test/strictness/inputs.txt through
#      the same oracle
#        -> test/strictness/cases.json
#
# Both runtimes run this themselves before grading — `pretest` in ts/ and
# TestMain in go/ — so the corpora are present wherever the suites run,
# including CI. Without them the conformance suites FAIL; they never skip.
# This repo never decides what ZON means: every verdict in both corpora comes
# from the pinned compiler.
#
# Requires: bash, curl, tar, python3, and sha256sum or shasum. Network access
# on first run only.
#
# HOST SUPPORT. The oracle is a zig program built with the pinned zig
# toolchain, so this script runs where that toolchain is wired up: linux and
# macos, x86_64 and aarch64. On any other host it exits 0 with a notice and
# writes nothing; the conformance runners then report one explicit,
# platform-named skip rather than pretending to measure. See test/AGENTS.md.

set -euo pipefail

ZIG_VERSION="0.16.0"
# The ziglang/zig commit the release was cut from, recorded in the corpora so
# a result can always be traced back to an exact tree.
ZIG_COMMIT="24fdd5b7a4c1c8b5deb5b56756b9dbc8e08c86a8"
ZIG_REPO="https://codeberg.org/ziglang/zig"

# --- the pins --------------------------------------------------------------
# Every download is content-addressed. A SHA-256 over the exact bytes is a
# stronger pin than a version in a URL: it survives the upstream repo moving
# host (which ziglang/zig just did, GitHub -> Codeberg) and it makes a swapped
# artifact a hard failure instead of a silently different corpus.
#
# Values are the `shasum` fields published in https://ziglang.org/download/index.json
# for 0.16.0. Re-check them there if a pin is ever bumped.
ZIG_SRC_SHA256="43186959edc87d5c7a1be7b7d2a25efffd22ce5807c7af99067f86f99641bfdf"

zig_toolchain_sha256() { # triple
  case "$1" in
    x86_64-linux)   echo "70e49664a74374b48b51e6f3fdfbf437f6395d42509050588bd49abe52ba3d00" ;;
    aarch64-linux)  echo "ea4b09bfb22ec6f6c6ceac57ab63efb6b46e17ab08d21f69f3a48b38e1534f17" ;;
    x86_64-macos)   echo "0387557ed1877bc6a2e1802c8391953baddba76081876301c522f52977b52ba7" ;;
    aarch64-macos)  echo "b23d70deaa879b5c2d486ed3316f7eaa53e84acf6fc9cc747de152450d401489" ;;
    *)              echo "" ;;
  esac
}

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VENDOR="$REPO_ROOT/test/zigzon/vendor"
TOOLS="$REPO_ROOT/test/zigzon/tools"
SRC_DIR="$VENDOR/zig-$ZIG_VERSION"
TOOLCHAIN_DIR="$VENDOR/toolchain"
BUILD_DIR="$VENDOR/oracle-build"

mkdir -p "$VENDOR" "$BUILD_DIR"

# --- host triple for the prebuilt toolchain --------------------------------
# An unsupported host is NOT an error: it exits 0 having written nothing, and
# the runners report a named skip for that platform. An unsupported host on a
# platform we do claim (a missing pin, a broken download) is a hard failure.
case "$(uname -s)" in
  Linux)  os="linux" ;;
  Darwin) os="macos" ;;
  *)
    echo "fetch-zigzon: no pinned zig $ZIG_VERSION oracle toolchain for" \
         "$(uname -s); skipping the corpus build on this host." >&2
    exit 0 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch="x86_64" ;;
  arm64|aarch64) arch="aarch64" ;;
  *)
    echo "fetch-zigzon: no pinned zig $ZIG_VERSION oracle toolchain for" \
         "$(uname -m); skipping the corpus build on this host." >&2
    exit 0 ;;
esac
TRIPLE="$arch-$os"
TOOLCHAIN="$TOOLCHAIN_DIR/zig-$TRIPLE-$ZIG_VERSION"

sha256_of() { # file
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1
  elif command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" | cut -d' ' -f1
  else
    echo "fetch-zigzon: need sha256sum or shasum to verify a pinned download" >&2
    exit 1
  fi
}

# fetch <url> <dest> <sha256> — idempotent AND verified. A cached file with the
# wrong hash is refetched, not trusted; a fresh download with the wrong hash is
# a hard failure. Downloading a tarball and untarring it unverified is how a
# conformance corpus quietly becomes something else.
fetch() {
  local url="$1" dest="$2" want="$3" got
  if [ -f "$dest" ]; then
    got="$(sha256_of "$dest")"
    if [ "$got" = "$want" ]; then return 0; fi
    echo "fetch-zigzon: cached $(basename "$dest") has the wrong hash, refetching" >&2
    rm -f "$dest"
  fi
  echo "fetch-zigzon: downloading $url"
  if ! curl --fail --location --retry 3 --progress-bar --output "$dest.part" "$url"; then
    rm -f "$dest.part"
    echo "fetch-zigzon: download failed: $url" >&2
    exit 1
  fi
  got="$(sha256_of "$dest.part")"
  if [ "$got" != "$want" ]; then
    rm -f "$dest.part"
    echo "fetch-zigzon: SHA-256 MISMATCH for $url" >&2
    echo "  expected $want" >&2
    echo "  actual   $got" >&2
    echo "This is a pinned artifact. A mismatch means the pin is wrong or the" >&2
    echo "download was tampered with. It is not something to work around." >&2
    exit 1
  fi
  mv "$dest.part" "$dest"
}

# GNU tar needs --wildcards to expand the member patterns below; bsdtar (the
# macOS default) does not have the flag and globs by default. Without this the
# source extraction fails outright on macOS.
# (A plain string, not an array: macOS still ships bash 3.2, where expanding an
# empty array under `set -u` is itself an error.)
TAR_WILDCARDS=""
if tar --version 2>/dev/null | grep -qi 'gnu tar'; then
  TAR_WILDCARDS="--wildcards"
fi

# --- 1. the prebuilt toolchain ---------------------------------------------
TOOLCHAIN_TAR="$VENDOR/zig-$TRIPLE-$ZIG_VERSION.tar.xz"
TOOLCHAIN_SHA256="$(zig_toolchain_sha256 "$TRIPLE")"
if [ -z "$TOOLCHAIN_SHA256" ]; then
  echo "fetch-zigzon: no pinned zig $ZIG_VERSION toolchain hash for $TRIPLE" >&2
  exit 1
fi
if [ ! -x "$TOOLCHAIN/zig" ]; then
  fetch "https://ziglang.org/download/$ZIG_VERSION/zig-$TRIPLE-$ZIG_VERSION.tar.xz" \
    "$TOOLCHAIN_TAR" "$TOOLCHAIN_SHA256"
  mkdir -p "$TOOLCHAIN_DIR"
  tar -xJf "$TOOLCHAIN_TAR" -C "$TOOLCHAIN_DIR"
fi
ZIG="$TOOLCHAIN/zig"
HAVE_VERSION="$("$ZIG" version)"
# The corpus and the stdlib that adjudicates it must be the same release.
if [ "$HAVE_VERSION" != "$ZIG_VERSION" ]; then
  echo "fetch-zigzon: oracle toolchain is zig '$HAVE_VERSION', expected '$ZIG_VERSION'" >&2
  exit 1
fi
echo "fetch-zigzon: toolchain $HAVE_VERSION"

# --- 2. the source tree (only the parts that hold ZON) ---------------------
# The extracted tree is stamped with a hash of THIS SCRIPT. A plain "already
# extracted" marker made a stale tree survive a change to the member patterns
# below, silently, forever: a working copy kept harvesting the OLD set of
# documents while a fresh checkout harvested the new one, and the difference
# only showed up as a corpus census mismatch in CI. Re-extract whenever the
# recipe changes.
SRC_TAR="$VENDOR/zig-$ZIG_VERSION.tar.xz"
SRC_STAMP="$SRC_DIR/.fetched"
WANT_STAMP="$(sha256_of "${BASH_SOURCE[0]}")"
if [ ! -f "$SRC_STAMP" ] || [ "$(cat "$SRC_STAMP")" != "$WANT_STAMP" ]; then
  fetch "https://ziglang.org/download/$ZIG_VERSION/zig-$ZIG_VERSION.tar.xz" \
    "$SRC_TAR" "$ZIG_SRC_SHA256"
  rm -rf "$SRC_DIR"
  mkdir -p "$SRC_DIR"
  # Everything that can contain a ZON document: the manifests, the codegen
  # tables, the behaviour/compile-error fixtures, and the std zon parser
  # (whose tests embed ~150 more snippets).
  tar -xJf "$SRC_TAR" -C "$SRC_DIR" --strip-components=1 $TAR_WILDCARDS \
    '*/build.zig.zon' \
    '*/lib/init/*' \
    '*/lib/std/zon/*' \
    '*/src/codegen/*' \
    '*/test/behavior/zon/*' \
    '*/test/cases/compile_errors/zon/*'
  printf '%s' "$WANT_STAMP" > "$SRC_STAMP"
fi

# --- 3. the oracle ---------------------------------------------------------
ORACLE="$BUILD_DIR/oracle"
echo "fetch-zigzon: building the oracle"
"$ZIG" build-exe "$TOOLS/oracle.zig" \
  -O ReleaseSafe \
  --cache-dir "$BUILD_DIR/cache" \
  -femit-bin="$ORACLE"

# --- 4. the corpora --------------------------------------------------------
echo "fetch-zigzon: harvesting ZON documents from the zig tree"
python3 "$TOOLS/harvest.py" "$SRC_DIR" > "$BUILD_DIR/harvest.json"

echo "fetch-zigzon: judging with zig $ZIG_VERSION"
ZIG_VERSION="$ZIG_VERSION" ZIG_COMMIT="$ZIG_COMMIT" ZIG_REPO="$ZIG_REPO" \
python3 "$TOOLS/judge.py" \
  --oracle "$ORACLE" \
  --harvest "$BUILD_DIR/harvest.json" \
  --inputs "$REPO_ROOT/test/strictness/inputs.txt" \
  --zigzon-out "$REPO_ROOT/test/zigzon/cases.json" \
  --strictness-out "$REPO_ROOT/test/strictness/cases.json"

echo "fetch-zigzon: done"
echo "  test/zigzon/cases.json"
echo "  test/strictness/cases.json"
