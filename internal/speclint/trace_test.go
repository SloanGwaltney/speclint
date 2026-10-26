package speclint

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

func traceFixture(t *testing.T, ref string) *Trace {
	t.Helper()
	tr, err := TraceIntent(filepath.Join("testdata", "good"), ref)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

// locations lists annotations as "file:line", in the order given.
func locations(as []Annotation) []string {
	var out []string
	for _, a := range as {
		out = append(out, fmt.Sprintf("%s:%d", a.File, a.Line))
	}
	return out
}

func TestTraceShowsTheIntentAndItsNotes(t *testing.T) {
	tr := traceFixture(t, "cancelled-no-reminder")

	in := tr.Intent
	if tr.File.Path != "features/call-reminders.md" || in.Kind != KindMustNot || in.Statement != "A cancelled call never produces a reminder." {
		t.Fatalf("traced %s %s %q", tr.File.Path, in.Kind, in.Statement)
	}
	want := "Cancellation can arrive after the reminder is queued, so this is checked\nat send time, not schedule time.\n\n```text\n- @id:not-an-intent @status:implemented fenced examples aren't read\n```"
	if in.Notes != want {
		t.Errorf("notes:\n%s\nwant:\n%s", in.Notes, want)
	}
	if got, want := locations(tr.Links), []string{"scheduler.go:3", "scheduler_test.go:3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("links: got %v, want %v", got, want)
	}
	if m := tr.Missing(); len(m) != 0 {
		t.Errorf("expected nothing missing, got %v", m)
	}
}

// Links are ordered from the database up to the UI, each layer's code before its tests.
func TestTraceOrdersLinksByLayer(t *testing.T) {
	tr := traceFixture(t, "features/call-reminders.md#default-lead-time")

	want := []string{"reminders.sql:2", "scheduler.go:3", "scheduler_test.go:3"}
	if got := locations(tr.Links); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTraceFindsADRs(t *testing.T) {
	tr := traceFixture(t, "default-lead-time")

	// The fenced example in the ADR names another file and isn't read.
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

	want := []string{"no @intent link in code", "no @intent link in tests"}
	if got := tr.Missing(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTraceRejectsUnknownIntents(t *testing.T) {
	root := filepath.Join("testdata", "good")
	for _, ref := range []string{"no-such-id", "features/other.md#snooze"} {
		if _, err := TraceIntent(root, ref); err == nil {
			t.Errorf("%s: expected an error", ref)
		}
	}
}

// A file traces each intent in the order written, leaving out withdrawn ones.
func TestTraceFileTracesEachIntentButWithdrawn(t *testing.T) {
	f, traces, err := TraceFile(filepath.Join("testdata", "good"), "features/call-reminders.md")
	if err != nil {
		t.Fatal(err)
	}
	if f.Title != "Call reminders" {
		t.Errorf("title %q", f.Title)
	}
	var got []string
	for _, tr := range traces {
		got = append(got, tr.Intent.ID)
	}
	want := []string{"default-lead-time", "shown-in-list", "snooze", "reminder-default-sound", "reminder-chosen-sound", "cancelled-no-reminder"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("traced %v, want %v", got, want)
	}
	// Each trace is linked as TraceIntent links it alone.
	for _, tr := range traces {
		alone := traceFixture(t, tr.File.Path+"#"+tr.Intent.ID)
		if !reflect.DeepEqual(tr.Links, alone.Links) || !reflect.DeepEqual(tr.ADRs, alone.ADRs) {
			t.Errorf("%s: file trace differs from the intent's own trace", tr.Intent.ID)
		}
	}
}

func TestTraceFileRejectsUnknownFiles(t *testing.T) {
	root := filepath.Join("testdata", "good")
	for _, path := range []string{"features/other.md", "call-reminders.md"} {
		if _, _, err := TraceFile(root, path); err == nil {
			t.Errorf("%s: expected an error", path)
		}
	}
}
