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

Go 1.22 or later, standard library only. It reads annotations in any language with `//`, `#`, `--`,
or `<!-- -->` comments.

## Why

The goal is a **grep-complete** codebase: one where a text search from any entry point finds
everything related to it, including the spec it implements, the tests that prove it, and the
decisions behind it.

That matters most when an agent writes the code. An agent starts every session with nothing. If
specs live in a wiki, decisions in an issue tracker, and the connections between them only in
someone's head, the agent has to search several sources and guess how they fit together. The more
hops that takes, the more tokens it spends and the more often it guesses wrong.

So the links are written down, in plain text, in the repository:

- Specs are `.feature` files. Every scenario has a stable `@id:` and a `@status:`.
- Code and tests point at scenarios and steps with comments like `@scenario [#move-card-forward]`.
- ADRs list the scenarios they affect in a Related section, using the same reference format.

Grep a scenario ID and you get its entry points, its tests, and its ADRs in one search, with no
index or embeddings to keep up to date.

Written-down links only help while they're right, and without a check they drift: a step gets
reworded, a scenario gets withdrawn, a test gets deleted. `speclint` is that check. It runs with
the project's other checks (in CI, or from a Claude Code Stop hook) and fails when any link is
missing, broken, or stale, so the agent fixes the links in the same change that broke them.
`spectrace` is the read side: give it a scenario and it prints everything linked to it.

## What it looks like

### Specs

Specs live in `features/*.feature`. They're Gherkin-like but never run by a BDD test runner; they
are the spec that code and tests link back to. Every `Scenario` and `Scenario Outline` gets exactly
one `@status:` tag and one kebab-case `@id:` tag:

```gherkin
Feature: Call reminders
  As a user
  I want to be reminded before a scheduled call
  So that I don't miss it

  @status:implemented @id:cancelled-no-reminder
  Scenario: No reminder for a cancelled call
    Given I had a call scheduled for 3:00 PM
    And I cancelled the call
    When the time reaches 2:45 PM
    Then I do not receive a reminder
```

| Status            | Meaning                                                    |
|-------------------|------------------------------------------------------------|
| `not-implemented` | Specified, not built yet.                                  |
| `implemented`     | Built. speclint requires links in both code and tests.     |
| `deprecated`      | Still in the app but scheduled for removal.                |
| `withdrawn`       | Removed. Kept as a record; links to it are an error.       |

IDs are unique across all feature files and never change or get reused, so code can keep pointing
at them after a scenario's title is reworded. `Background` isn't allowed: each scenario carries its
own `Given` steps so it reads on its own.

### Code

A `@scenario` annotation goes on each entry point: the function, handler, endpoint, or component
where the behavior starts. It names scenarios by ID, as many as the code implements, on one line:

```go
// @scenario [#cancelled-no-reminder]
func (s *Scheduler) onTick(now time.Time) {
	...
}
```

### Tests

Tests carry the `@scenario` annotation and one step annotation per step, each directly above the
code that does that step. A step annotation names the feature file and copies the step's text
exactly:

```go
// @scenario [#cancelled-no-reminder]
func TestNoReminderForCancelledCall(t *testing.T) {
	s, notifier := newTestScheduler(t)

	// @given [features/call-reminders.feature] I had a call scheduled for 3:00 PM
	call := s.Schedule("Standup", at(15, 0))
	// @and [features/call-reminders.feature] I cancelled the call
	s.Cancel(call.ID)

	// @when [features/call-reminders.feature] the time reaches 2:45 PM
	s.onTick(at(14, 45))

	// @then [features/call-reminders.feature] I do not receive a reminder
	if got := notifier.Sent(); len(got) != 0 {
		t.Fatalf("expected no reminders, got %v", got)
	}
}
```

When the same step text appears in several scenarios of one file but different code carries it
out, scope it with the ID: `@then [features/call-reminders.feature#default-lead-time] I receive a
reminder`. Other languages use their own comment style (`#` in Python, `--` in SQL,
`{/* */}` in JSX).

### ADRs

ADRs in `docs/adr/` end with a Related section using the same references, or `universal` for a
decision that isn't tied to particular features:

```markdown
## Related

- [features/call-reminders.feature#cancelled-no-reminder] No reminder for a cancelled call
- [features/call-scheduling.feature] Call scheduling
- [docs/adr/0002-poll-scheduler-every-minute.md] Poll the scheduler every minute
```

### Finding and checking links

A plain grep finds everything for one scenario. The closing bracket stops it matching longer IDs:

```sh
grep -rn "#cancelled-no-reminder\]" .
```

`spectrace` gives the same answer grouped by layer, and says whether the scenario is fully linked:

```
Scenario: Move a card to the next status
  features/job-board.feature:31  @id:move-card-forward  @status:implemented

Entry points (@scenario)
  service test   internal/jobs/move_test.go:30               @scenario
  Wails binding  app.go:101                                  @scenario
  frontend       frontend/src/components/Board.tsx:81        @scenario

Given I have a job for "Software Engineer" at "Acme" in "Saved"  (features/job-board.feature:32)
  service test   internal/jobs/move_test.go:33               @given
  frontend test  frontend/src/components/Board.test.tsx:209  @given
...
Decisions (ADR Related)
  docs/adr/0013-use-dnd-kit-for-moving-and-reordering-board-cards.md:60  this scenario

Fully linked: @scenario in code and tests, and all 4 steps in tests.
```

`speclint` prints `file:line: problem` for each finding and exits 1 if there are any. The full
rules are in [`docs/spec-annotations.md`](docs/spec-annotations.md).

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
| `languages` | Most languages: see `defaultLanguages` in `internal/speclint/config.go` | Adds file extensions to read, each with its comment style: `c` (`//`, `/* */`, JSX `{/* */}`), `hash` (`#`), `dash` (`--`), `c-hash` (both `//` and `#`), or `html` (`<!-- -->`, and Thymeleaf's `<!--/* */-->`). `""` stops an extension being read. |
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
