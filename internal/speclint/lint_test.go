package speclint

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func lintFixture(t *testing.T, name string) []string {
	t.Helper()
	problems, err := Lint(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range problems {
		got = append(got, p.String())
	}
	return got
}

// The good fixture covers a statement on the tag line, notes with a fenced example, a Replaces
// line, free-form sections, JSX and SQL comments, TypeScript tests, and intents in every status,
// all linked correctly.
func TestGoodRepoHasNoProblems(t *testing.T) {
	if got := lintFixture(t, "good"); len(got) != 0 {
		t.Errorf("expected no problems, got:\n%s", strings.Join(got, "\n"))
	}
}

// The bad fixture has one of each kind of problem. testdata/bad/want.txt is the expected output.
func TestBadRepoReportsEachProblem(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("testdata", "bad", "want.txt"))
	if err != nil {
		t.Fatal(err)
	}
	wantLines := strings.Split(strings.TrimSpace(strings.ReplaceAll(string(want), "\r\n", "\n")), "\n")
	got := lintFixture(t, "bad")

	if strings.Join(got, "\n") != strings.Join(wantLines, "\n") {
		t.Errorf("got:\n%s\n\nwant:\n%s", strings.Join(got, "\n"), strings.Join(wantLines, "\n"))
	}
}

// Going over a limit is a warning by default, and an error with strictLimits.
func TestStrictLimitsMakesLimitsErrors(t *testing.T) {
	files := map[string]string{
		"features/x.md": "# X\n\nDoes x.\n\n## Must\n\n- @id:a @status:not-implemented\n  One two three four five.\n",
	}
	for _, strict := range []bool{false, true} {
		files[ConfigFile] = fmt.Sprintf(`{"limits": {"statementWords": 3}, "strictLimits": %v}`, strict)
		problems, err := Lint(writeRepo(t, files))
		if err != nil {
			t.Fatal(err)
		}
		if len(problems) != 1 || problems[0].Warning == strict {
			t.Errorf("strict=%v: got %v, want one problem with Warning=%v", strict, problems, !strict)
		}
	}
}

func TestParseIntentFile(t *testing.T) {
	f, problems := parseIntentFile("features/x.md", `# Call reminders

Reminds people before a call.
Wrapped onto a second line.

Second paragraph.

## Must

- @status:implemented @id:on-time When the lead time arrives, the person
  is reminded.

  Notes, kept as written.
    Indented more.
- @id:chosen-sound @status:not-implemented
  Plays the chosen sound.
  Replaces: @id:old-sound

## Must not

* @id:cancelled @status:implemented
  Never reminds for a cancelled call.
`)
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	if f.Title != "Call reminders" || f.Purpose != "Reminds people before a call. Wrapped onto a second line.\n\nSecond paragraph." {
		t.Errorf("title %q, purpose %q", f.Title, f.Purpose)
	}

	var got []string
	for _, in := range f.Intents {
		got = append(got, fmt.Sprintf("%d %s %s %s: %s | %q %v", in.Line, in.ID, in.Status, in.Kind, in.Statement, in.Notes, in.Replaces))
	}
	want := []string{
		`10 on-time implemented must: When the lead time arrives, the person is reminded. | "Notes, kept as written.\n  Indented more." []`,
		`15 chosen-sound not-implemented must: Plays the chosen sound. | "Replaces: @id:old-sound" [old-sound]`,
		`21 cancelled implemented must-not: Never reminds for a cancelled call. | "" []`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestParseIntentFileWithoutTitle(t *testing.T) {
	for _, src := range []string{"", "Just text.\n\n## Must\n"} {
		_, problems := parseIntentFile("features/x.md", src)
		if len(problems) != 1 || !strings.Contains(problems[0].Msg, "# Title") {
			t.Errorf("%q: got %v, want one problem about the title", src, problems)
		}
	}
}

// parse reads a file's annotations with the comment style its extension has by default.
func parse(file, src string, test bool) ([]Annotation, []Problem) {
	return parseAnnotations(file, src, DefaultConfig().languages[filepath.Ext(file)], test)
}

// ids lists annotations as "line:id".
func ids(as []Annotation) []string {
	var out []string
	for _, a := range as {
		out = append(out, fmt.Sprintf("%d:%s", a.Line, a.ID))
	}
	return out
}

func TestParseAnnotationsOnlyReadsComments(t *testing.T) {
	src := strings.Join([]string{
		"// @intent [#line]",
		"",
		"\t/* @intent [#block] */",
		"",
		"      {/* @intent [#jsx] */}",
		"",
		" * @intent [#continuation]",
		`s := "// @intent [#string]"`,
		"x() // @intent [#trailing]",
	}, "\n")

	annotations, _ := parse("f.go", src, false)
	want := []string{"1:line", "3:block", "5:jsx", "7:continuation"}
	if got := ids(annotations); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseAnnotationsInSQL(t *testing.T) {
	src := strings.Join([]string{
		"-- @intent [#sql]",
		"SELECT 1;",
		"    -- @intent [#indented]",
		"SELECT 1;",
		"// @intent [#not-sql]",
		"SELECT 1; -- @intent [#trailing]",
		"SELECT '-- @intent [#string]';",
	}, "\n")

	annotations, _ := parse("queries/jobs.sql", src, false)
	if got, want := ids(annotations), []string{"1:sql", "3:indented"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// -- isn't a comment in Go, so it doesn't count there.
	if got, _ := parse("f.go", "-- @intent [#x]", false); len(got) != 0 {
		t.Errorf("parsed %d annotations from -- in a .go file, want 0", len(got))
	}
}

func TestParseAnnotationsWithHashComments(t *testing.T) {
	src := strings.Join([]string{
		"# @intent [#a] [#b]",
		"# a plain comment",
		"# @intent [#c]",
		"def f():",
		"    # @intent [#indented]",
		"    x = 1  # @intent [#trailing]",
		`    s = "# @intent [#string]"`,
		"// @intent [#not-python]",
	}, "\n")

	annotations, problems := parse("tests/test_a.py", src, true)
	if got, want := ids(annotations), []string{"1:a", "1:b", "3:c", "5:indented"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	// Lines 1 to 3 are one comment block, so the second @intent line is reported.
	if len(problems) != 1 || problems[0].Line != 3 {
		t.Errorf("problems: got %v, want one on line 3", problems)
	}
}

func TestParseAnnotationsWithHTMLComments(t *testing.T) {
	src := strings.Join([]string{
		"<!--/* @intent [#a] [#b] */-->",
		"<!-- @intent [#c] -->",
		"<section>",
		"  <!--/* @intent [#thymeleaf] */-->",
		"  <p>x</p>",
		"  <!-- @intent [#html] -->",
		"  <p>x</p> <!-- @intent [#trailing] -->",
		`  <p title="<!-- @intent [#attribute] -->">y</p>`,
		"  <!--",
		"    @intent [#inside-a-block]",
		"  -->",
		"</section>",
		"// @intent [#not-html]",
	}, "\n")

	annotations, problems := parse("templates/board.html", src, false)
	want := []string{"1:a", "1:b", "2:c", "4:thymeleaf", "6:html"}
	if got := ids(annotations); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	// Lines 1 and 2 are one comment block, so the second @intent line is reported.
	if len(problems) != 1 || problems[0].Line != 2 {
		t.Errorf("problems: got %v, want one on line 2", problems)
	}
}

func TestParseIntentAnnotations(t *testing.T) {
	src := strings.Join([]string{
		"// @intent [#a] [#b]\t[#c]",
		"func f() {}",
		"{/* @intent [#d] */}",
		"// @intent [#e]",
		"// a plain comment between them",
		"// @intent [#f]",
		"x()",
		"// @intent [#g],[#h]",
		"// @intent names prose that wraps onto a new line",
		"// @intent [features/a.md#i] Title",
		"// @intent [#j] [#j]",
	}, "\n")

	annotations, problems := parse("f.tsx", src, false)
	want := []string{"1:a", "1:b", "1:c", "3:d", "4:e", "6:f", "11:j"}
	if got := ids(annotations); !reflect.DeepEqual(got, want) {
		t.Errorf("annotations: got %v, want %v", got, want)
	}
	if annotations[0].Count != 3 || annotations[6].Count != 1 {
		t.Errorf("counts: got %d and %d, want 3 and 1", annotations[0].Count, annotations[6].Count)
	}

	var lines []int
	for _, p := range problems {
		lines = append(lines, p.Line)
	}
	// 3, 4, and 6 are one block of comment lines, whatever their comment style.
	// 8 and 10 aren't lists of [#id], 9 is prose, and 11 names one ID twice.
	if want := []int{4, 6, 8, 10, 11}; !reflect.DeepEqual(lines, want) {
		t.Errorf("problems on lines %v, want %v: %v", lines, want, problems)
	}
}

// Annotations from speclint v1 are reported, so a project moving to v2 finds every one.
func TestParseAnnotationsReportsV1Annotations(t *testing.T) {
	src := strings.Join([]string{
		"// @scenario [#a]",
		"// @given [features/a.feature] I have a call",
		"// @and [features/a.feature#a] it's cancelled",
		"// @then the prose that wraps isn't an annotation",
	}, "\n")
	annotations, problems := parse("f_test.go", src, true)
	if len(annotations) != 0 {
		t.Errorf("annotations: %v", annotations)
	}
	var lines []int
	for _, p := range problems {
		lines = append(lines, p.Line)
	}
	if want := []int{1, 2, 3}; !reflect.DeepEqual(lines, want) {
		t.Errorf("problems on lines %v, want %v: %v", lines, want, problems)
	}
}

func TestIsTestFile(t *testing.T) {
	for path, want := range map[string]bool{
		"app_test.go":                   true,
		"frontend/src/App.test.tsx":     true,
		"frontend/src/App.spec.ts":      true,
		"frontend/src/__tests__/App.ts": true,
		"tests/test_board.py":           true,
		"app/board_test.py":             true,
		"spec/board_spec.rb":            true,
		"src/test/java/BoardTest.java":  true,
		"Board.Tests/BoardTests.cs":     true,
		"app.go":                        false,
		"frontend/src/App.tsx":          false,
		"frontend/src/testing.ts":       false,
		"app/board.py":                  false,
		"src/main/java/Contest.java":    false,
		"src/attest/verify.rs":          false,
	} {
		if got := isDefaultTestFile(path); got != want {
			t.Errorf("isDefaultTestFile(%q) = %v, want %v", path, got, want)
		}
	}
}
