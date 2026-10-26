# speclint

Keeps a short, written statement of what each feature must and must not do, links it to the code
and tests that keep it, and checks that the links stay correct and the statements stay short.

- **`speclint`** checks every link: annotations that point at missing or withdrawn intents,
  implemented intents with no code or tests, malformed intent files, and ADRs whose Related
  section is wrong. It also warns when an intent file, a statement, or an entry point says more
  than an intent should.
- **`spectrace`** shows one intent, or a whole intent file, with everything linked to it: the
  statement and its notes, the code and tests that name it, layer by layer, and the ADRs behind it.
- **[`docs/intents.md`](docs/intents.md)** is the format both tools read: how intent files are
  written and how code and tests link to them. It's versioned with the tools, so the rules and the
  checker always agree.

Go 1.22 or later, standard library only. It reads annotations in any language with `//`, `#`, `--`,
or `<!-- -->` comments.

> **v2 is a breaking change.** v1 linked every step of Gherkin scenarios to tests. v2 drops
> Gherkin and steps for markdown intents. See [Moving from v1](#moving-from-v1).

## Why

An agent starts every session with nothing. It can read the code to learn what an entry point
*does*, but not what it's *for*: which behaviors are the point and which are incidental, and
what it must never do. That's the context it most often gets wrong, and it's the hardest to
recover from code alone.

So each feature gets an **intent file**: a title, a paragraph on what the feature is for, and a
short list of obligations under **Must** and **Must not**. Each obligation has a stable ID, and
the entry points that keep it say so in a comment. Grep an ID and you get the intent, the code,
the tests, and the ADRs in one search, with no index to keep up to date.

Intents are deliberately not a full specification. The code and tests already define behavior in
detail; repeating that in a spec gives two sources of truth that drift. An intent names what the
feature would be *wrong* without: the few things a reviewer would reject the change for. Edge
cases, validation rules, and error messages belong in tests. speclint enforces this with limits
(see [Configuration](#configuration)): a file with 15 intents, or an entry point naming 9 of them,
means the intents have turned back into a list of every behavior.

Written-down links only help while they're right, and without a check they drift: an intent gets
withdrawn, a test gets deleted. `speclint` is that check. It runs with the project's other checks
(in CI, or from a Claude Code Stop hook) and fails when a link is missing, broken, or stale, so the
agent fixes it in the same change that broke it.

## What it looks like

### Intent files

Intent files live in `features/*.md`:

```markdown
# Call reminders

Reminds people before a scheduled call so they don't miss it. Reminders are
local to the device; there's no server push.

## Must

- @id:default-lead-time @status:implemented
  When a call's reminder lead time arrives, the person is reminded of that call.

- @id:chosen-sound @status:not-implemented
  A reminder plays the sound chosen in settings.
  Replaces: @id:default-sound

- @id:default-sound @status:deprecated
  A reminder plays the default sound.

## Must not

- @id:cancelled-no-reminder @status:implemented
  A cancelled call never produces a reminder.

  Cancellation can arrive after the reminder is queued, so this is checked
  at send time, not schedule time.
```

Each item under `## Must` or `## Must not` is an intent: an `@id:` and a `@status:`, then one
sentence stating the obligation. Anything indented under it after a blank line is **notes**:
context, the reason behind it, tricky cases. Notes aren't checked, so they can be as long as they
need to be. Other `##` sections, like "Out of scope", are free-form.

| Status            | Meaning                                                    |
|-------------------|------------------------------------------------------------|
| `not-implemented` | Stated, not built yet.                                     |
| `implemented`     | Built. speclint requires links in both code and tests.     |
| `deprecated`      | Still in the app but scheduled for removal.                |
| `withdrawn`       | Removed. Kept as a record; links to it are an error.       |

IDs are unique across all intent files and never change or get reused, so code can keep pointing
at them after a statement is reworded.

### Code and tests

An `@intent` annotation goes on each entry point that keeps an intent (the function, handler,
endpoint, or component where the behavior starts) and on the tests that check it. It names
intents by ID, on one line:

```go
// @intent [#cancelled-no-reminder] [#default-lead-time]
func (s *Scheduler) onTick(now time.Time) {
	...
}

// @intent [#cancelled-no-reminder]
func TestNoReminderForCancelledCall(t *testing.T) {
	...
}
```

Other languages use their own comment style (`#` in Python, `--` in SQL, `{/* */}` in JSX).

### ADRs

ADRs in `docs/adr/` end with a Related section that names the intents or intent files they
affect, or `universal` for a decision that isn't tied to particular features. The text after the
link is a label for people and isn't checked:

```markdown
## Related

- [features/call-reminders.md#cancelled-no-reminder] No reminder for a cancelled call
- [features/call-scheduling.md] Call scheduling
- [docs/adr/0002-poll-scheduler-every-minute.md] Poll the scheduler every minute
```

### Finding and checking links

A plain grep finds everything for one intent. The closing bracket stops it matching longer IDs:

```sh
grep -rn "#cancelled-no-reminder\]" .
```

`spectrace` gives the same answer with the intent itself, grouped by layer:

```
Must not: A cancelled call never produces a reminder.
  features/call-reminders.md:21  @id:cancelled-no-reminder  @status:implemented

  Cancellation can arrive after the reminder is queued, so this is checked
  at send time, not schedule time.

Linked from (@intent)
  internal       internal/reminders/scheduler.go:40
  internal test  internal/reminders/scheduler_test.go:88

Decisions (ADR Related)
  docs/adr/0004-queue-reminders.md:31  this intent

Linked in code and tests.
```

Give it an intent file instead of an ID to see the feature's purpose and every intent in it.

`speclint` prints `file:line: problem` for each finding and exits 1 if there are any errors.
Limits print as `file:line: warning: ...` and don't fail the run unless `strictLimits` is set. The
full rules are in [`docs/intents.md`](docs/intents.md).

## Adding it to a project

Add it as a submodule at `tools/speclint`:

```sh
git submodule add https://github.com/SloanGwaltney/speclint tools/speclint
```

Run both tools from the project root:

```sh
go run -C tools/speclint ./cmd/speclint -root ../..
go run -C tools/speclint ./cmd/spectrace -root ../.. <intent-id | features/x.md#intent-id | features/x.md>
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
  "limits": { "intentsPerFile": 8, "intentsPerEntryPoint": 4, "statementWords": 40 },
  "strictLimits": false,
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
| `featuresDir` | `features` | Where the intent files are. |
| `adrDir` | `docs/adr` | Where the ADRs are. |
| `limits.intentsPerFile` | `8` | Most active (`not-implemented` or `implemented`) intents in one file. |
| `limits.intentsPerEntryPoint` | `4` | Most intents one `@intent` line in code may name. Tests aren't counted, since one test may check several. |
| `limits.statementWords` | `40` | Most words in an active intent's statement. Notes aren't counted. |
| `strictLimits` | `false` | Makes going over a limit an error instead of a warning. A limit of `0` turns it off. |
| `languages` | Most languages: see `defaultLanguages` in `internal/speclint/config.go` | Adds file extensions to read, each with its comment style: `c` (`//`, `/* */`, JSX `{/* */}`), `hash` (`#`), `dash` (`--`), `c-hash` (both `//` and `#`), or `html` (`<!-- -->`, and Thymeleaf's `<!--/* */-->`). `""` stops an extension being read. |
| `skipDirs` | Dot-directories, `node_modules`, `vendor`, `dist`, `build`, `out`, `target`, `bin`, `obj`, `coverage`, `testdata`, `venv`, `__pycache__` | Adds directories never read. A name matches anywhere; a path with a slash matches only that path. Add generated code here. |
| `testPatterns` | The usual conventions: `*_test.go`, `*.test.*`, `*.spec.*`, `test_*.py`, `*_test.py`, `*_spec.rb`, `*Test.*`, `*Tests.*`, and `__tests__/`, `test/`, `tests/`, `spec/` directories | Adds regular expressions, matched against repo-relative paths, for other test files. |
| `layers` | SQL files, then each top-level directory | Replaces the layers spectrace groups links by. List them bottom of the stack first; the first match wins. Files no layer matches are grouped by top-level directory. |

Unknown fields are an error, so a typo doesn't get silently ignored.

## Moving from v1

v1 read Gherkin `.feature` files and required every step of an implemented scenario to be linked
from a test. v2 reads markdown intent files and links whole intents only. speclint v2 reports
every leftover `.feature` file, `@scenario` line, and step annotation, so a project can work
through the list until it lints clean.

1. **Convert each `features/x.feature` to `features/x.md`.** Write a title and a purpose
   paragraph. For each scenario that isn't withdrawn, decide whether it's an obligation the
   feature would be wrong without. If it is, keep its `@id:` and `@status:` and write one sentence
   stating it under `## Must` or `## Must not`. If it's a detail (a validation rule, an edge case,
   a specific message), drop it from the file; its test stays as an ordinary test. Withdrawn
   scenarios can be kept as withdrawn intents for the record, or dropped.
2. **Replace `@scenario [#id]` with `@intent [#id]`** in code and tests, and remove IDs you
   dropped in step 1.
3. **Delete step annotations** (`@given`, `@when`, `@then`, `@and`, `@but`).
4. **Update ADR Related sections** to point at `features/x.md` instead of `features/x.feature`.
5. **Run speclint** and work through what it reports, including the limit warnings.

Keeping IDs unchanged in step 1 means annotations and ADRs only need their file extension or
keyword changed, not new IDs.

## Developing

```sh
go vet ./...
go test ./...
```

The test fixtures are in `internal/speclint/testdata/`. `good/` must lint clean, and `bad/` must
produce exactly `bad/want.txt`.
