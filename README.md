# speclint

Links Gherkin-like specs to the code and tests that carry them out, and checks that the links stay
correct.

- **`speclint`** checks every link: annotations that point at missing scenarios or steps, step
  text that doesn't match, implemented scenarios with no code or tests, malformed specs, and ADRs
  whose Related section is wrong.
- **`spectrace`** shows everything linked to one scenario, layer by layer: its entry points, the
  tests for each step, and the ADRs behind it.
- **[`docs/spec-annotations.md`](docs/spec-annotations.md)** is the format both tools read: how
  specs are tagged and how code and tests link to them. It's versioned with the tools, so the
  rules and the checker always agree.

Go 1.22 or later, standard library only. It reads annotations in any language with `//`, `#`, or
`--` comments.

## Adding it to a project

Add it as a submodule at `tools/speclint`:

```sh
git submodule add https://github.com/SloanGwaltney/speclint tools/speclint
```

Run both tools from the project root:

```sh
go run -C tools/speclint ./cmd/speclint -root ../..
go run -C tools/speclint ./cmd/spectrace -root ../.. <scenario-id | features/x.feature#id | features/x.feature>
go run -C tools/speclint ./cmd/spectrace -root ../.. -missing features/x.feature
```

It's its own Go module, so in a Go project `go vet ./...` and `go test ./...` skip it.

To update it, pull a newer version and commit the new submodule pointer:

```sh
git -C tools/speclint pull
git add tools/speclint
```

Someone cloning the project needs `git clone --recurse-submodules`, or
`git submodule update --init` after cloning.

## Configuration

Everything has a default, so most projects need no configuration. To change something, add a
`.speclint.json` at the project root. Every field is optional:

```json
{
  "featuresDir": "features",
  "adrDir": "docs/adr",
  "languages": { ".nix": "hash", ".svelte": "" },
  "skipDirs": ["frontend/wailsjs"],
  "testPatterns": ["^e2e/"],
  "layers": [
    { "name": "SQL query", "match": "\\.sql$" },
    { "name": "storage", "match": "^internal/db/" },
    { "name": "backend", "match": "^internal/" },
    { "name": "frontend", "match": "^web/" }
  ]
}
```

| Field | Default | What it does |
|---|---|---|
| `featuresDir` | `features` | Where the `.feature` files are. |
| `adrDir` | `docs/adr` | Where the ADRs are. |
| `languages` | Most languages: see `defaultLanguages` in `internal/speclint/config.go` | Adds file extensions to read, each with its comment style: `c` (`//`, `/* */`, JSX `{/* */}`), `hash` (`#`), `dash` (`--`), or `c-hash` (both `//` and `#`). `""` stops an extension being read. |
| `skipDirs` | Dot-directories, `node_modules`, `vendor`, `dist`, `build`, `out`, `target`, `bin`, `obj`, `coverage`, `testdata`, `venv`, `__pycache__` | Adds directories never read. A name matches anywhere; a path with a slash matches only that path. Add generated code here. |
| `testPatterns` | The usual conventions: `*_test.go`, `*.test.*`, `*.spec.*`, `test_*.py`, `*_test.py`, `*_spec.rb`, `*Test.*`, `*Tests.*`, and `__tests__/`, `test/`, `tests/`, `spec/` directories | Adds regular expressions, matched against repo-relative paths, for other test files. |
| `layers` | SQL files, then each top-level directory | Replaces the layers spectrace groups links by. List them bottom of the stack first; the first match wins. Files no layer matches are grouped by top-level directory. |

Unknown fields are an error, so a typo doesn't get silently ignored.

## Developing

```sh
go vet ./...
go test ./...
```

The test fixtures are in `internal/speclint/testdata/`. `good/` must lint clean, and `bad/` must
produce exactly `bad/want.txt`.
