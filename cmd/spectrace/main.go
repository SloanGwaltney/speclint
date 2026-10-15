// Command spectrace shows everything linked to a scenario, layer by layer: its entry points, the
// tests (and any code) for each step, and the ADRs behind it. Name one scenario, or a feature file to
// trace each of its scenarios that isn't withdrawn. It's its own Go module, so run it from the
// repository root with -C:
//
//	go run -C tools/speclint ./cmd/spectrace -root ../.. move-card-forward
//	go run -C tools/speclint ./cmd/spectrace -root ../.. features/job-board.feature#move-card-forward
//	go run -C tools/speclint ./cmd/spectrace -root ../.. features/job-board.feature
//	go run -C tools/speclint ./cmd/spectrace -root ../.. -missing features/job-board.feature
//
// With -missing, it prints only the links that are missing, under the scenario they belong to,
// and nothing for a fully linked scenario.
//
// It exits 1 if any traced scenario has no @scenario link in code or in tests, or any step has no
// link in tests, whatever the scenario's status. It reads annotations the way speclint does; see docs/spec-annotations.md.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/SloanGwaltney/speclint/internal/speclint"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: spectrace [-root dir] [-missing] <scenario-id | features/x.feature#scenario-id | features/x.feature>")
		flag.PrintDefaults()
	}
	root := flag.String("root", ".", "repository root")
	onlyMissing := flag.Bool("missing", false, "print only the missing links")
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	traces, err := trace(*root, flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "spectrace:", err)
		os.Exit(2)
	}
	missing := 0
	for i, t := range traces {
		if *onlyMissing {
			missing += reportMissing(os.Stdout, t)
			continue
		}
		if i > 0 {
			fmt.Fprintf(os.Stdout, "\n%s\n\n", strings.Repeat("-", 72))
		}
		missing += report(os.Stdout, t)
	}
	if len(traces) > 1 {
		summary(os.Stdout, traces)
	}
	if missing > 0 {
		os.Exit(1)
	}
}

// trace traces the scenario ref names, or every scenario in a feature file when ref is a path to
// one with no "#<id>".
func trace(root, ref string) ([]*speclint.Trace, error) {
	if strings.HasSuffix(ref, ".feature") {
		return speclint.TraceFeature(root, ref)
	}
	t, err := speclint.TraceScenario(root, ref)
	if err != nil {
		return nil, err
	}
	return []*speclint.Trace{t}, nil
}

// reportMissing writes the scenario's missing links under its ID, or nothing when it's fully
// linked, and returns how many are missing.
func reportMissing(out io.Writer, t *speclint.Trace) int {
	missing := t.Missing()
	if len(missing) == 0 {
		return 0
	}
	fmt.Fprintf(out, "%s#%s  %s\n", t.Feature, t.Scenario.ID, t.Scenario.Title)
	for _, m := range missing {
		fmt.Fprintln(out, "  MISSING:", m)
	}
	return len(missing)
}

// summary writes how many of the traced scenarios are fully linked.
func summary(out io.Writer, traces []*speclint.Trace) {
	linked := 0
	for _, t := range traces {
		if len(t.Missing()) == 0 {
			linked++
		}
	}
	fmt.Fprintf(out, "\n%d of %d scenarios fully linked.\n", linked, len(traces))
}

// report writes the trace and returns how many links are missing.
func report(out io.Writer, t *speclint.Trace) int {
	s := t.Scenario
	fmt.Fprintf(out, "Scenario: %s\n", s.Title)
	fmt.Fprintf(out, "  %s:%d  @id:%s  @status:%s\n", t.Feature, s.Line, s.ID, s.Status)

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "\nEntry points (@scenario)")
	links(w, t.Entry)
	for _, st := range t.Steps {
		fmt.Fprintf(w, "\n%s %s  (%s:%d)\n", st.Step.Keyword, st.Step.Text, t.Feature, st.Step.Line)
		links(w, st.Links)
	}
	fmt.Fprintln(w, "\nDecisions (ADR Related)")
	if len(t.ADRs) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, r := range t.ADRs {
		scope := "this scenario"
		if r.Whole {
			scope = "the whole feature"
		}
		fmt.Fprintf(w, "  %s:%d\t%s\n", r.File, r.Line, scope)
	}
	w.Flush()

	missing := t.Missing()
	fmt.Fprintln(out)
	if len(missing) == 0 {
		fmt.Fprintf(out, "Fully linked: @scenario in code and tests, and all %d steps in tests.\n", len(t.Steps))
		return 0
	}
	for _, m := range missing {
		fmt.Fprintln(out, "MISSING:", m)
	}
	return len(missing)
}

func links(w io.Writer, as []speclint.Annotation) {
	if len(as) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, a := range as {
		scoped := ""
		if a.ID != "" && a.Keyword != "scenario" {
			scoped = "  (scoped #" + a.ID + ")"
		}
		fmt.Fprintf(w, "  %s\t%s:%d\t@%s%s\n", a.Where(), a.File, a.Line, a.Keyword, scoped)
	}
}
