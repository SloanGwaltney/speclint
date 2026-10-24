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

// The good fixture covers shared steps, scoped steps, JSX comments, TypeScript tests, and
// not-implemented and withdrawn scenarios, all linked correctly.
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

// parse reads a file's annotations with the comment style its extension has by default.
func parse(file, src string, test bool) ([]Annotation, []Problem) {
	return parseAnnotations(file, src, DefaultConfig().languages[filepath.Ext(file)], test)
}

func TestParseAnnotationsOnlyReadsComments(t *testing.T) {
	src := strings.Join([]string{
		"// @then [features/a.feature] line comment",
		"\t/* @then [features/a.feature] block comment */",
		"      {/* @then [features/a.feature#x] jsx comment */}",
		" * @then [features/a.feature] block continuation",
		`s := "// @then [features/a.feature] in a string"`,
		"x() // @then [features/a.feature] trailing comment",
	}, "\n")

	var got []string
	annotations, _ := parse("f.go", src, false)
	for _, a := range annotations {
		got = append(got, a.Text)
	}
	want := []string{"line comment", "block comment", "jsx comment", "block continuation"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseAnnotationsInSQL(t *testing.T) {
	src := strings.Join([]string{
		"-- @then [features/a.feature] sql comment",
		"    -- @then [features/a.feature] indented sql comment",
		"// @then [features/a.feature] not a sql comment",
		"SELECT 1; -- @then [features/a.feature] trailing comment",
		"SELECT '-- @then [features/a.feature] in a string';",
	}, "\n")

	var got []string
	annotations, _ := parse("queries/jobs.sql", src, false)
	for _, a := range annotations {
		got = append(got, a.Text)
	}
	want := []string{"sql comment", "indented sql comment"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}

	// -- isn't a comment in Go, so it doesn't count there.
	if got, _ := parse("f.go", "-- @then [features/a.feature] x", false); len(got) != 0 {
		t.Errorf("parsed %d annotations from -- in a .go file, want 0", len(got))
	}
}

func TestParseAnnotationsWithHashComments(t *testing.T) {
	src := strings.Join([]string{
		"# @scenario [#a] [#b]",
		"# a plain comment",
		"# @scenario [#c]",
		"def f():",
		"    # @then [features/a.feature] indented hash comment",
		"    x = 1  # @then [features/a.feature] trailing comment",
		`    s = "# @then [features/a.feature] in a string"`,
		"// @then [features/a.feature] not a python comment",
	}, "\n")

	annotations, problems := parse("tests/test_a.py", src, true)
	var got []string
	for _, a := range annotations {
		got = append(got, a.Keyword+":"+a.ID+a.Text)
	}
	want := []string{"scenario:a", "scenario:b", "scenario:c", "then:indented hash comment"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	// Lines 1 to 3 are one comment block, so the second @scenario line is reported.
	if len(problems) != 1 || problems[0].Line != 3 {
		t.Errorf("problems: got %v, want one on line 3", problems)
	}
}

func TestParseAnnotationsWithHTMLComments(t *testing.T) {
	src := strings.Join([]string{
		"<!--/* @scenario [#a] [#b] */-->",
		"<!-- @scenario [#c] -->",
		"<section>",
		"  <!--/* @then [features/a.feature] thymeleaf comment */-->",
		"  <!-- @then [features/a.feature] html comment -->",
		"  <p>x</p> <!-- @then [features/a.feature] trailing comment -->",
		`  <p title="<!-- @then [features/a.feature] in an attribute -->">y</p>`,
		"  <!--",
		"    @then [features/a.feature] inside a comment block",
		"  -->",
		"</section>",
		"// @then [features/a.feature] not an html comment",
	}, "\n")

	annotations, problems := parse("templates/board.html", src, false)
	var got []string
	for _, a := range annotations {
		got = append(got, a.Keyword+":"+a.ID+a.Text)
	}
	want := []string{"scenario:a", "scenario:b", "scenario:c", "then:thymeleaf comment", "then:html comment"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	// Lines 1 and 2 are one comment block, so the second @scenario line is reported.
	if len(problems) != 1 || problems[0].Line != 2 {
		t.Errorf("problems: got %v, want one on line 2", problems)
	}
}

// And and But continue the kind of the step before them, so they take either keyword.
func TestStepKindContinuesTheStepBefore(t *testing.T) {
	f, problems := parseFeature("features/x.feature", `Feature: X
  @status:not-implemented @id:x
  Scenario: X
    And something already true
    Given I have a call
    And I cancelled it
    When I move the card
    But I cancel
    Then it stays
    And nothing is sent
`)
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	var kinds []string
	for _, st := range f.Scenarios[0].Steps {
		kinds = append(kinds, st.Keyword+":"+st.Kind)
	}
	want := []string{"And:and", "Given:given", "And:given", "When:when", "But:when", "Then:then", "And:then"}
	if !reflect.DeepEqual(kinds, want) {
		t.Errorf("got %v, want %v", kinds, want)
	}

	cancelled := f.Scenarios[0].Steps[2]
	for kw, want := range map[string]bool{"and": true, "given": true, "when": false, "but": false} {
		if got := cancelled.Accepts(kw); got != want {
			t.Errorf("Accepts(%q) = %v, want %v", kw, got, want)
		}
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

func TestParseScenarioAnnotations(t *testing.T) {
	src := strings.Join([]string{
		"// @scenario [#a] [#b]\t[#c]",
		"func f() {}",
		"{/* @scenario [#d] */}",
		"// @scenario [#e]",
		"// a plain comment between them",
		"// @scenario [#f]",
		"x()",
		"// @scenario [#g],[#h]",
		"// @scenario names prose that wraps onto a new line",
		"// @scenario [features/a.feature#i] Title",
		"// @scenario [#j] [#j]",
	}, "\n")

	annotations, problems := parse("f.tsx", src, false)
	var got []string
	for _, a := range annotations {
		got = append(got, fmt.Sprintf("%d:%s", a.Line, a.ID))
	}
	want := []string{"1:a", "1:b", "1:c", "3:d", "4:e", "6:f", "11:j"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("annotations: got %v, want %v", got, want)
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
