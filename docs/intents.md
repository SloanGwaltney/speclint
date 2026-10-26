# Intents

How features state what they must and must not do, and how code and tests link back to those
statements.

Intents live in `features/*.md` (or the `featuresDir` set in `.speclint.json`), written with the
`intent` skill. Every intent has an `@id:` and a `@status:`. Code and tests point at intents with
`@intent [#id]` comments, and `speclint` checks the links.

## 1. What an intent is

An intent is an **obligation**: something the feature would be wrong without. It's not a
description of every behavior. The code and tests already define behavior in detail, and a spec
that repeats them becomes a second source of truth that drifts.

Write something as an intent when a reviewer would reject a change that broke it, and you could
explain why to someone who doesn't read code:

| Intent                                              | Not an intent (leave it to tests)            |
|-----------------------------------------------------|----------------------------------------------|
| A cancelled call never produces a reminder.         | The lead time can't be negative.             |
| A card can be moved to any status on the board.     | Dragging a card shows a drop shadow.         |
| An order is never charged twice.                    | The card number field rejects letters.       |
| Only the owner of a board can delete it.            | The delete button is red.                    |

Most features need two to six intents. The prohibitions under **Must not** are often the most
valuable, because code rarely says what it's careful never to do.

## 2. The file format

```markdown
# Call reminders

Reminds people before a scheduled call so they don't miss it. Reminders are
local to the device; there's no server push.

## Must

- @id:default-lead-time @status:implemented
  When a call's reminder lead time arrives, the person is reminded of that call.

## Must not

- @id:cancelled-no-reminder @status:implemented
  A cancelled call never produces a reminder.

  Cancellation can arrive after the reminder is queued, so this is checked
  at send time, not schedule time. See docs/adr/0004-queue-reminders.md.

## Out of scope

- Calendar sync is handled by the calendar feature.
```

Rules:

- **One feature per file**, named in kebab-case, e.g. `features/call-reminders.md`.
- **A `# Title` line first, then a purpose paragraph**: what the feature is for, and any context
  that shapes every intent in it. speclint reports a file with no purpose.
- **Intents go under `## Must` or `## Must not`.** Every list item in those sections is an intent.
  Other `##` sections are free-form and aren't checked, but they can't contain intents.
- **An intent starts with its tags:** `- @id:<id> @status:<status>`, in either order, nothing else.
- **Then the statement:** one sentence, on the same line as the tags or the indented lines after
  them. It ends at the first blank line.
- **Anything indented after that is notes:** why the intent exists, cases that are easy to get
  wrong, links to ADRs or code. Notes can be as long as they need to be and aren't checked.
- **IDs are kebab-case** (lowercase letters, digits, hyphens), unique across every intent file,
  **never change**, and **are never reused**. Code points at them, so keep the ID when a statement
  is reworded.

### Statuses

| Status            | Meaning                                                               |
|-------------------|-----------------------------------------------------------------------|
| `not-implemented` | Stated, not built yet. Can be freely edited or deleted.               |
| `implemented`     | Built. Linked from code and tests.                                    |
| `deprecated`      | Still in the app, but scheduled for removal. Don't build on it.       |
| `withdrawn`       | Removed from the app. Kept as a record; nothing may link to it.       |

Status only moves forward: `not-implemented → implemented → deprecated → withdrawn`. Don't change
a status to `implemented` yourself; that happens when the user says the intent is built.

### Changing an implemented intent

- **Notes can always change.** Adding context or a tricky case to an implemented intent's notes
  doesn't change what it promises.
- **The statement can be reworded but not changed in meaning.** Making it clearer is fine. If the
  feature should now do something different, that's a new intent.
- **Changing what's promised means replacing it.** Mark the old intent `@status:deprecated`, and
  write the new one as `@status:not-implemented` with a new ID and a `Replaces:` line in its notes:

  ```markdown
  - @id:chosen-sound @status:not-implemented
    A reminder plays the sound chosen in settings.
    Replaces: @id:default-sound
  ```

  speclint checks that a `Replaces:` line names an intent that exists and is no longer active.

## 3. Linking code and tests

Every implemented intent must be named by an `@intent` annotation in code, on the entry point
that keeps it, and in tests, on a test that checks it:

```
@intent [#<intent id>] [#<another intent id>] ...
```

- **`@intent` goes on the entry point:** the function, handler, endpoint, command, query, or
  component where the behavior starts. An intent kept by more than one entry point gets an
  `@intent` at each.
- **In tests, put it on the test function**, or on the subtest or `it` block when one function
  covers several intents. Tests for details the intents don't name stay unannotated.
- **One `@intent` line per function, test, or block.** List every intent the code under it keeps
  on that line, separated by spaces. If the entry point already has an `@intent` line, add the new
  ID to it rather than adding a second line.
- **IDs only.** Don't add the file path or the statement. IDs are unique across all intent files,
  so the ID alone finds the intent.
- **An annotation is the only thing in its comment, on a line of its own,** directly above the code
  it links. A trailing comment after code isn't read.
- **SQL queries are annotated like code.** A query that's an intent's entry point gets its
  `@intent` in a `--` comment.
- **Generated code:** annotate the source, never the generated output. If a generator copies
  comments into what it generates (sqlc does this for queries), that copy is expected.
- **Don't annotate migrations.** A later migration can undo an earlier one, leaving its annotation
  pointing at code that no longer runs. Annotate the storage tests that check the schema instead.
- **Keep annotations in sync with the intents.** When an intent is withdrawn, remove its ID from
  every `@intent` line in the same change.

### Example

```go
// @intent [#cancelled-no-reminder] [#default-lead-time]
func (s *Scheduler) onTick(now time.Time) {
	for _, call := range s.callsDueForReminder(now) {
		if call.Cancelled {
			continue
		}
		s.sendReminder(call)
	}
}
```

```go
// @intent [#cancelled-no-reminder]
func TestNoReminderForCancelledCall(t *testing.T) {
	s, notifier := newTestScheduler(t)
	call := s.Schedule("Standup", at(15, 0))
	s.Cancel(call.ID)

	s.onTick(at(14, 45))

	if got := notifier.Sent(); len(got) != 0 {
		t.Fatalf("expected no reminders, got %v", got)
	}
}
```

## 4. Comment styles

Use the language's comment:

| Languages                                                    | Comment                   |
|--------------------------------------------------------------|---------------------------|
| Go, Rust, C, C++, C#, Java, Kotlin, Swift, Dart, JS, TS      | `//`, `/* */`, or ` * `   |
| JSX and TSX markup                                           | `{/* */}`                 |
| Python, Ruby, Elixir, shell, PowerShell, R, Julia            | `#`                       |
| SQL, Lua, Haskell, Elm                                       | `--`                      |
| PHP                                                          | `//`, `/* */`, or `#`     |
| HTML, including Thymeleaf templates                          | `<!-- -->` or `<!--/* */-->` |

If the project uses a language that isn't read by default, add its extension under `languages` in
`.speclint.json` (see the speclint README).

## 5. Finding an intent

Everything that names one intent, in code, tests, and ADRs. The closing bracket keeps the search
from also matching longer IDs that start with this one, like `cancelled-no-reminder-sms`:

```sh
grep -rn "#cancelled-no-reminder\]" --exclude-dir=node_modules --exclude-dir=.git .
```

The intent itself, with its notes and everything linked to it, layer by layer. speclint and
spectrace are their own Go module, a git submodule at `tools/speclint/`, so run them from the repo
root with `go run -C`:

```sh
go run -C tools/speclint ./cmd/spectrace -root ../.. cancelled-no-reminder
go run -C tools/speclint ./cmd/spectrace -root ../.. features/call-reminders.md
```

Given an intent file, it prints the feature's purpose and each intent that isn't withdrawn. It
exits 1 if any intent it shows has no `@intent` link in code or in tests, whatever its status, so
it can check an intent before it's marked implemented.

## 6. Checking the links

Run `speclint` with the project's other checks (for example from a check script that a Claude Code
Stop hook and CI both run). To run it by hand from the repo root:

```sh
go run -C tools/speclint ./cmd/speclint -root ../..
```

It prints `file:line: problem` for each finding and exits 1 if there are any errors. It reports:

- **Missing links.** Every `@status:implemented` intent needs an `@intent` annotation in both code
  and tests. A file is a test if it follows the usual conventions (`*_test.go`, `*.test.*`,
  `*.spec.*`, `test_*.py`, `*_test.py`, `*_spec.rb`, `*Test.java`, `*Tests.cs`, or anything under
  `__tests__/`, `test/`, `tests/`, or `spec/`) or matches one of the `testPatterns` in
  `.speclint.json`.
- **Broken links.** `@intent` lines that name an ID no intent file has, aren't a list of `[#id]`,
  name an ID twice, or follow another `@intent` line in the same comment block.
- **Stale links.** `@intent` lines and ADR Related entries that name withdrawn intents.
- **Malformed intent files.** A missing title or purpose, intents without exactly one valid
  `@status:` and one kebab-case `@id:`, an unknown tag, an intent with no statement, an untagged
  item under Must or Must not, an intent outside those sections, IDs used twice, and `Replaces:`
  lines that name a missing or still-active intent.
- **ADR Related sections** that are missing, empty, mix `universal` with intent references, or
  reference a file or ID that doesn't exist.
- **speclint v1 leftovers.** `.feature` files, `@scenario` lines, and step annotations
  (`@given`, `@when`, `@then`, `@and`, `@but`).

And it warns, without failing, when an intent file or entry point says more than an intent
should. Set `strictLimits` in `.speclint.json` to make these errors:

- **A file with more than 8 active intents.** Keep the ones the feature would be wrong without, or
  split the feature.
- **An `@intent` line in code naming more than 4 intents.** The entry point is doing too much, or
  its intents are too detailed.
- **A statement longer than 40 words.** State the obligation in one sentence and move the detail
  to its notes.

The limits can be changed under `limits` in `.speclint.json`.

The linter skips dot-directories, dependencies, build output, and `testdata`. Add generated-code
directories under `skipDirs` in `.speclint.json`.
