#!/usr/bin/env node

// Embed semver-grammar.abnf into the TypeScript and Go source files.
// Run via: npm run embed  (or:  node embed-grammar.js)
//
// The grammar file at the repo root is the single source of truth; the
// two copies between the BEGIN/END markers are generated and must never
// be edited by hand.

const fs = require('fs')
const path = require('path')

const GRAMMAR_FILE = path.join(__dirname, '..', 'semver-grammar.abnf')
const TS_FILE = path.join(__dirname, 'src', 'semver.ts')
const GO_FILE = path.join(__dirname, '..', 'go', 'semver.go')

const BEGIN = '// --- BEGIN EMBEDDED semver-grammar.abnf ---'
const END = '// --- END EMBEDDED semver-grammar.abnf ---'

const grammar = fs.readFileSync(GRAMMAR_FILE, 'utf8')

// --- TypeScript embedding ---
function embedTS() {
  let src = fs.readFileSync(TS_FILE, 'utf8')
  const startIdx = src.indexOf(BEGIN)
  const endIdx = src.indexOf(END)
  if (startIdx === -1 || endIdx === -1) {
    console.error('TS markers not found in', TS_FILE)
    process.exit(1)
  }

  // Escape backticks and template expressions for a JS template literal.
  const escaped = grammar
    .replace(/\\/g, '\\\\')
    .replace(/`/g, '\\`')
    .replace(/\$\{/g, '\\${')

  const replacement =
    BEGIN +
    '\nconst grammarText = `\n' +
    escaped +
    '`\n' +
    END

  src = src.substring(0, startIdx) + replacement + src.substring(endIdx + END.length)
  fs.writeFileSync(TS_FILE, src)
  console.log('Embedded grammar into', TS_FILE)
}

// --- Go embedding ---
function embedGo() {
  let src = fs.readFileSync(GO_FILE, 'utf8')
  const startIdx = src.indexOf(BEGIN)
  const endIdx = src.indexOf(END)
  if (startIdx === -1 || endIdx === -1) {
    console.error('Go markers not found in', GO_FILE)
    process.exit(1)
  }

  if (grammar.includes('`')) {
    console.error('Grammar contains backticks, incompatible with Go raw strings')
    process.exit(1)
  }

  // The blank line before END keeps the result gofmt-clean (gofmt wants
  // a blank line between the const declaration and the trailing comment).
  const replacement =
    BEGIN +
    '\nconst grammarText = `\n' +
    grammar +
    '`\n\n' +
    END

  src = src.substring(0, startIdx) + replacement + src.substring(endIdx + END.length)
  fs.writeFileSync(GO_FILE, src)
  console.log('Embedded grammar into', GO_FILE)
}

embedTS()
embedGo()
