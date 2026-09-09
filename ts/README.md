# @tabnas/semver

A [Tabnas](https://github.com/tabnas/parser) grammar plugin that parses
[Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html) version
strings into their five parts, exactly as the specification defines them.
The parser is the specification's own grammar, compiled from ABNF by
[`@tabnas/abnf`](https://github.com/tabnas/abnf) when the plugin is
installed.

## Install

```bash
npm install @tabnas/parser @tabnas/abnf @tabnas/semver
```

`@tabnas/parser` and `@tabnas/abnf` are peer dependencies.

## One example

The plugin installs on a bare Tabnas engine:

```js
import { Tabnas } from '@tabnas/parser'
import { Semver, compare, format } from '@tabnas/semver'

const tn = new Tabnas().use(Semver)

tn.parse('1.2.3-alpha.1+build.5')
// => { major: 1, minor: 2, patch: 3, prerelease: ['alpha', 1], build: ['build', '5'] }

compare(tn.parse('1.0.0-alpha'), tn.parse('1.0.0')) // => -1
format(tn.parse('1.2.3+sha.5114f85'))               // => '1.2.3+sha.5114f85'
```

Build the instance once and reuse it — compiling the grammar is the
expensive part.

## Documentation

Full documentation follows the [Diátaxis](https://diataxis.fr)
framework:

- [Tutorial](doc/tutorial.md) — a guided first parse, start to finish.
- [How-to guide](doc/guide.md) — short recipes for individual tasks.
- [Reference](doc/reference.md) — the public API, the value shape, and
  the complete syntax accepted.
- [Concepts](doc/concepts.md) — how the plugin turns the specification's
  grammar into a parser, and why.

For the Go port, see [`../go/README.md`](../go/README.md).

## Grammar

The grammar is defined in the top-level
[`semver-grammar.abnf`](../semver-grammar.abnf) and embedded into this
implementation (and the Go port) by [`embed-grammar.js`](embed-grammar.js)
during the build. It is also exported, as `grammar`, for tooling that
wants the text.

## License

Copyright (c) 2026 Richard Rodger and other contributors,
[MIT License](LICENSE).
