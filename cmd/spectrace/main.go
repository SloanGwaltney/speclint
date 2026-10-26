// Command spectrace shows an intent and everything linked to it: its statement and notes, the
// code and tests that name it with @intent, layer by layer, and the ADRs behind it. Name one
// intent, or an intent file to show its purpose and each of its intents that isn't withdrawn.
// It's its own Go module, so run it from the repository root with -C:
//
//	go run -C tools/speclint ./cmd/spectrace -root ../.. move-card-forward
//	go run -C tools/speclint ./cmd/spectrace -root ../.. features/job-board.md#move-card-forward
//	go run -C tools/speclint ./cmd/spectrace -root ../.. features/job-board.md
//
// It exits 1 if any intent it shows has no @intent link in code or in tests, whatever the
// intent's status, so it can check an intent before it's marked implemented.
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
		fmt.Fprintln(os.Stderr, "usage: spectrace [-root dir] <intent-id | features/x.md#intent-id | features/x.md>")
		flag.PrintDefaults()
	}
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	ref := flag.Arg(0)

	missing := 0
	if strings.HasSuffix(ref, ".md") {
		f, traces, err := speclint.TraceFile(*root, ref)
		if err != nil {
			fmt.Fprintln(os.Stderr, "spectrace:", err)
			os.Exit(2)
		}
		fmt.Printf("# %s\n%s\n", f.Title, f.Path)
		if f.Purpose != "" {
			fmt.Printf("\n%s\n", indent(f.Purpose, ""))
		}
		linked := 0
		for _, t := range traces {
			fmt.Printf("\n%s\n\n", strings.Repeat("-", 72))
			n := report(os.Stdout, t)
			missing += n
			if n == 0 {
				linked++
			}
		}
		fmt.Printf("\n%d of %d intents linked in code and tests.\n", linked, len(traces))
	} else {
		t, err := speclint.TraceIntent(*root, ref)
		if err != nil {
			fmt.Fprintln(os.Stderr, "spectrace:", err)
			os.Exit(2)
		}
		missing = report(os.Stdout, t)
	}
	if missing > 0 {
		os.Exit(1)
	}
}

// report writes the trace and returns how many links are missing.
func report(out io.Writer, t *speclint.Trace) int {
	in := t.Intent
	kind := "Must"
	if in.Kind == speclint.KindMustNot {
		kind = "Must not"
	}
	fmt.Fprintf(out, "%s: %s\n", kind, in.Statement)
	fmt.Fprintf(out, "  %s:%d  @id:%s  @status:%s\n", t.File.Path, in.Line, in.ID, in.Status)
	if in.Notes != "" {
		fmt.Fprintf(out, "\n%s\n", indent(in.Notes, "  "))
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "\nLinked from (@intent)")
	if len(t.Links) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, a := range t.Links {
		fmt.Fprintf(w, "  %s\t%s:%d\n", a.Where(), a.File, a.Line)
	}
	fmt.Fprintln(w, "\nDecisions (ADR Related)")
	if len(t.ADRs) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, r := range t.ADRs {
		scope := "this intent"
		if r.Whole {
			scope = "the whole file"
		}
		fmt.Fprintf(w, "  %s:%d\t%s\n", r.File, r.Line, scope)
	}
	w.Flush()

	missing := t.Missing()
	fmt.Fprintln(out)
	if len(missing) == 0 {
		fmt.Fprintln(out, "Linked in code and tests.")
		return 0
	}
	for _, m := range missing {
		fmt.Fprintln(out, "MISSING:", m)
	}
	return len(missing)
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n")
}
