package speclint

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func traceFixture(t *testing.T, ref string) *Trace {
	t.Helper()
	tr, err := TraceScenario(filepath.Join("testdata", "good"), ref)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

// locations lists annotations as "file:line @keyword", in the order given.
func locations(as []Annotation) []string {
	var out []string
	for _, a := range as {
		out = append(out, fmt.Sprintf("%s:%d @%s", a.File, a.Line, a.Keyword))
	}
	return out
}

func TestTraceCollectsEntryPointsAndSteps(t *testing.T) {
	tr := traceFixture(t, "cancelled-no-reminder")

	if tr.Feature != "features/call-reminders.feature" || tr.Scenario.Title != "No reminder for a cancelled call" {
		t.Fatalf("traced %s %q", tr.Feature, tr.Scenario.Title)
	}
	if got, want := locations(tr.Entry), []string{"scheduler.go:3 @scenario", "scheduler_test.go:3 @scenario"}; !reflect.DeepEqual(got, want) {
		t.Errorf("entry points: got %v, want %v", got, want)
	}
	// The shared Given is linked once and counts for this scenario too.
	if got, want := locations(tr.Steps[0].Links), []string{"scheduler.go:6 @given", "scheduler_test.go:5 @given"}; !reflect.DeepEqual(got, want) {
		t.Errorf("given: got %v, want %v", got, want)
	}
	if m := tr.Missing(); len(m) != 0 {
		t.Errorf("expected nothing missing, got %v", m)
	}
}

// A step scoped to another scenario isn't part of this scenario's trace, even with the same text.
func TestTraceSkipsStepsScopedToOtherScenarios(t *testing.T) {
	tr := traceFixture(t, "features/call-reminders.feature#shown-in-list")

	then := tr.Steps[1]
	if then.Step.Text != "I receive a reminder" {
		t.Fatalf("step 2 is %q", then.Step.Text)
	}
	want := []string{"frontend/src/List.tsx:6 @then", "frontend/src/List.test.tsx:4 @then"}
	if got := locations(then.Links); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// Links are ordered from the database up to the UI, each layer's code before its tests.
func TestTraceOrdersLinksByLayer(t *testing.T) {
	tr := traceFixture(t, "default-lead-time")

	want := []string{"reminders.sql:2 @then", "scheduler.go:9 @then", "scheduler_test.go:9 @then"}
	if got := locations(tr.Steps[2].Links); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTraceFindsADRs(t *testing.T) {
	tr := traceFixture(t, "default-lead-time")

	// The fenced example in the ADR names another feature and isn't read.
	want := []ADRRef{
		{File: "docs/adr/0001-poll-every-minute.md", Line: 5},
		{File: "docs/adr/0001-poll-every-minute.md", Line: 6, Whole: true},
	}
	if !reflect.DeepEqual(tr.ADRs, want) {
		t.Errorf("got %+v, want %+v", tr.ADRs, want)
	}
}

func TestTraceReportsMissingLinks(t *testing.T) {
	tr := traceFixture(t, "snooze")

	want := []string{
		"@scenario has no link in code",
		"@scenario has no link in tests",
		"When I snooze it has no link in tests",
		"Then it fires again in 5 minutes has no link in tests",
	}
	if got := tr.Missing(); !reflect.DeepEqual(got, want) {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestTraceRejectsUnknownScenarios(t *testing.T) {
	root := filepath.Join("testdata", "good")
	for _, ref := range []string{"no-such-id", "features/other.feature#snooze"} {
		if _, err := TraceScenario(root, ref); err == nil {
			t.Errorf("%s: expected an error", ref)
		}
	}
}

// A feature file traces each scenario in the order written, leaving out withdrawn ones.
func TestTraceFeatureTracesEachScenarioButWithdrawn(t *testing.T) {
	traces, err := TraceFeature(filepath.Join("testdata", "good"), "features/call-reminders.feature")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, tr := range traces {
		ids = append(ids, tr.Scenario.ID)
	}
	if want := []string{"cancelled-no-reminder", "default-lead-time", "shown-in-list", "snooze"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("traced %v, want %v", ids, want)
	}
	// Each trace is linked as TraceScenario links it alone.
	for _, tr := range traces {
		alone := traceFixture(t, tr.Feature+"#"+tr.Scenario.ID)
		if !reflect.DeepEqual(tr, alone) {
			t.Errorf("%s: feature trace differs from the scenario's own trace", tr.Scenario.ID)
		}
	}
}

func TestTraceFeatureRejectsUnknownFiles(t *testing.T) {
	root := filepath.Join("testdata", "good")
	for _, path := range []string{"features/other.feature", "call-reminders.feature"} {
		if _, err := TraceFeature(root, path); err == nil {
			t.Errorf("%s: expected an error", path)
		}
	}
}
