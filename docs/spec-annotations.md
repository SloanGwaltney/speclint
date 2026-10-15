# Spec annotations

How code and tests link back to the Gherkin-like specs in `features/`.

Specs live in `features/*.feature` (or the `featuresDir` set in `.speclint.json`), written with
the `bdd-feature` skill. Every scenario has a `@status:` tag and an `@id:` tag.

The format works in any language. The examples below use Go. Section 4 lists the comment styles.

## 1. Start from the spec

Before writing code for a new feature, find the scenarios it covers:

- Search `features/` for scenarios tagged `@status:not-implemented` that describe the feature.
- If none exist, stop and write them first with the `bdd-feature` skill. Don't build behavior
  that has no spec.
- Don't build on `@status:deprecated` or `@status:withdrawn` scenarios.
- Don't change a scenario's status to `@status:implemented` yourself. That happens only when the
  user says the feature is built (see the skill's change rules).

## 2. Where links go

Every scenario you implement must be traceable from the code where it starts, and every step in
it from the tests that prove it:

- **In code,** put a `@scenario` annotation on each entry point. Step annotations in code aren't
  required, so don't add them.
- **In tests,** put a `@scenario` annotation on the test and a step annotation on the test code
  for each step (see section 3).

Grepping a step's text finds its test, and the test leads to the code it calls.

Each annotation is a comment directly above the line or block it links. A `@scenario` annotation
names scenarios by ID, as many as the code under it implements, all on one line. A step annotation
names its feature file and copies the step's text:

```
@scenario [#<scenario id>] [#<another scenario id>] ...
@<step keyword> [<path to .feature file>] <exact step text from the file>
```

| Spec line           | Annotation                                           |
|---------------------|------------------------------------------------------|
| `Scenario:`         | `@scenario [#<id>]`                                  |
| `Scenario Outline:` | `@scenario [#<id>]`                                  |
| `Given`             | `@given [features/x.feature] <step text>`            |
| `When`              | `@when [features/x.feature] <step text>`             |
| `Then`              | `@then [features/x.feature] <step text>`             |
| `And`               | `@and [features/x.feature] <step text>`              |
| `But`               | `@but [features/x.feature] <step text>`              |

Rules:

- **One `@scenario` line per function, test, or block.** List every scenario the code under it
  implements on that line, separated by spaces: `@scenario [#move-card-forward] [#move-card-back]`.
  Don't add the feature file path or the scenario's title. Scenario IDs are unique across all
  feature files, so the ID alone finds the scenario.
- **Copy step text exactly** from the `.feature` file, keyword removed. That includes outline
  placeholders like `<sound>`. The text is the search key, so don't paraphrase it.
- **`And` and `But` take their own keyword or the one they continue.** "And I clear the search"
  after a `When` is linked by `@and` or `@when`, so one `@when` annotation covers the step in a
  scenario that writes it with `When` and in one that writes it with `And`. An `And` or `But` that
  starts a scenario takes only its own keyword.
- **Step annotations use the repo-relative path** with forward slashes, e.g.
  `[features/call-reminders.feature]`.
- **Step annotations leave the ID off by default.** A step is identified by its file and text,
  so a step shared by several scenarios is annotated once and covers all of them.
- **Add the ID to a step annotation when its text is ambiguous**, meaning the same step text
  appears in more than one scenario in the file but is carried out by different code in each.
  Scope each annotation to its scenario: `@then [features/x.feature#custom-lead-time] I receive
  a reminder`.
- **`@scenario` goes on the entry point**: the function, handler, endpoint, command, or component
  where the behavior starts.
- **Step annotations go on the test code that does the step:** the setup, the action, or the
  assertion. If you have to read past the comment to find that code, the comment is in the wrong
  place.
- **Annotate a step only in a test whose layer carries it out.** When a step happens in another
  layer, like "today is 2026-10-09" when the backend decides today, or a list order that a SQL
  query sets, link it from that layer's test, not from a comment with no code under it. A test
  that relies on another layer can say so in a plain comment that names the test proving it.
- **One piece of code can carry more than one annotation** when it fulfills several steps or
  scenarios. Stack step annotations, one per line, but name every scenario on the one
  `@scenario` line.
- **A `Then` checks an observable outcome.** Annotate the assertion on that outcome, such as the
  emitted event, the returned value, or the UI state.
- **SQL queries are annotated like code.** A query that's a scenario's entry point gets its
  `@scenario` annotation in a `--` comment.
- **Generated code:** annotate the source, never the generated output. If a generator copies
  comments into what it generates (sqlc does this for queries), that copy is expected; change the
  source and regenerate rather than editing it.
- **Don't annotate migrations.** A migration records a past change to the schema, and a later
  migration can undo what an earlier one did, leaving its annotation pointing at code that no
  longer runs. Annotate the storage tests that check the schema's behavior instead, since they run
  against the current schema.
- **Keep annotations in sync with the spec.** When a step is reworded, update its annotations in
  the same change. When a scenario is deprecated or withdrawn, remove its ID from `@scenario` lines and
  any step tags no other active scenario still uses. Don't remove a shared step's tag while
  another active scenario depends on it.

### Example

Spec (`features/call-reminders.feature`):

```gherkin
  @status:not-implemented @id:cancelled-no-reminder
  Scenario: No reminder for a cancelled call
    Given I had a call scheduled for 3:00 PM
    And I cancelled the call
    When the time reaches 2:45 PM
    Then I do not receive a reminder
```

Code, with only the entry point linked:

```go
// @scenario [#cancelled-no-reminder]
func (s *Scheduler) onTick(now time.Time) {
	for _, call := range s.callsDueForReminder(now) {
		if call.Cancelled {
			continue
		}
		s.sendReminder(call)
	}
}
```

## 3. Annotate the tests

Tests carry the scenario and every step, so a grep for a scenario finds its tests next to its
entry points, and a grep for a step finds the test that proves it.

- Put `@scenario` on the test function, or on the subtest/`it` block when one test function
  covers several scenarios.
- Put each step annotation directly above the test code for that step: `@given` on the setup,
  `@when` on the action, `@then` / `@and` / `@but` on the assertion.
- Same rules as production code: one `@scenario` line naming scenarios by ID, and on steps the
  exact text, the repo-relative path, and the scenario ID when the step is ambiguous, and update the tags in the same change when the spec changes.

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

## 4. Comment styles

An annotation must be the only thing in its comment, on a line of its own. A trailing comment
after code isn't read. Use the language's comment:

| Languages                                                    | Comment                   |
|--------------------------------------------------------------|---------------------------|
| Go, Rust, C, C++, C#, Java, Kotlin, Swift, Dart, JS, TS      | `//`, `/* */`, or ` * `   |
| JSX and TSX markup                                           | `{/* */}`                 |
| Python, Ruby, Elixir, shell, PowerShell, R, Julia            | `#`                       |
| SQL, Lua, Haskell, Elm                                       | `--`                      |
| PHP                                                          | `//`, `/* */`, or `#`     |

If the project uses a language that isn't read by default, add its extension under `languages` in
`.speclint.json` (see the speclint README).

```python
# @scenario [#cancelled-no-reminder]
def test_no_reminder_for_cancelled_call(scheduler, notifier):
    # @given [features/call-reminders.feature] I had a call scheduled for 3:00 PM
    call = scheduler.schedule("Standup", at(15, 0))
```

## Finding a spec in the code

Entry points, tests, and ADRs for one scenario. The closing bracket keeps the search from also
matching longer IDs that start with this one, like `cancelled-no-reminder-sms`:

```sh
grep -rn "#cancelled-no-reminder\]" --exclude-dir=node_modules --exclude-dir=.git .
```

Every step annotation and ADR for a feature file. `@scenario` lines name IDs without the file, so
use `spectrace` (below) to find a whole feature's entry points:

```sh
grep -rn "\[features/call-reminders.feature" --exclude-dir=node_modules --exclude-dir=.git .
```

Everything linked to one scenario, layer by layer. speclint and spectrace are their own Go module,
a git submodule at `tools/speclint/`, so run them from the repo root with `go run -C`:

```sh
go run -C tools/speclint ./cmd/spectrace -root ../.. move-card-forward
go run -C tools/speclint ./cmd/spectrace -root ../.. features/job-board.feature#move-card-forward
go run -C tools/speclint ./cmd/spectrace -root ../.. features/job-board.feature
go run -C tools/speclint ./cmd/spectrace -root ../.. -missing features/job-board.feature
```

It prints the scenario's `@scenario` entry points, the tests (and any code) for each step, and the
ADRs that list it, using the same matching rules as `speclint`: a step's annotation counts when it
has no scenario ID or this scenario's ID. It exits 1 if the scenario has no `@scenario` link in
code or in tests, or any step has no link in tests, whatever the scenario's status, so it can check a scenario before it's marked
`implemented`. Given a feature file, it traces each of its scenarios that isn't withdrawn and ends
with how many are fully linked. `-missing` prints only the missing links, under the scenario they
belong to.

Links are grouped by layer, bottom of the stack first. SQL files are the one layer named by
default; any other file is grouped under its top-level directory. A project can name its own
layers under `layers` in `.speclint.json`.

## Checking the links

`speclint` checks these rules. Run it with the project's other checks (for example from a check
script that a Claude Code Stop hook and CI both run). To run it by hand from the repo root:

```sh
go run -C tools/speclint ./cmd/speclint -root ../..
```

It prints `file:line: problem` for each finding and exits 1 if there are any. It reports:

- **Missing links.** Every `@status:implemented` scenario needs a `@scenario` annotation in both
  code and tests, and each of its steps a step annotation in tests. A file is a test if it follows
  the usual conventions (`*_test.go`, `*.test.*`, `*.spec.*`, `test_*.py`, `*_test.py`,
  `*_spec.rb`, `*Test.java`, `*Tests.cs`, or anything under `__tests__/`, `test/`, `tests/`, or
  `spec/`) or matches one of the `testPatterns` in `.speclint.json`.
- **Broken links.** Annotations, in code or tests, that point at a missing file or ID, copy step
  text inexactly, or use the wrong keyword (`@given` for an `And` step). A `@scenario` line that
  isn't a list of `[#id]`, names an ID twice, or follows another `@scenario` line in the same
  comment block.
- **Stale links.** Annotations on withdrawn scenarios, or on steps only withdrawn scenarios use.
- **Malformed specs.** Scenarios without exactly one valid `@status:` and one kebab-case `@id:`,
  IDs used twice, in one feature file or across them, status or ID tags on `Feature`/`Rule`/`Examples`, and `Background`.
- **ADR Related sections** that are missing, empty, mix `universal` with feature references, or
  reference a file, ID, or title that doesn't match.

The linter skips dot-directories, dependencies, build output, and `testdata`. Add generated-code
directories under `skipDirs` in `.speclint.json`.
