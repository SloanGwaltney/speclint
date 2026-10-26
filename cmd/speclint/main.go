// Command speclint checks that code, tests, and ADRs link correctly to the intents in
// features/*.md, and that intents stay within the limits that keep them high-level. It's its own Go module, usually a git submodule at tools/speclint, so run it
// from the repository root with -C:
//
//	go run -C tools/speclint ./cmd/speclint -root ../..
//
// It prints one "file:line: problem" per finding and exits 1 if there are any errors. Limits that
// are exceeded print as "file:line: warning: ..." and don't change the exit status unless
// strictLimits is set in .speclint.json. See docs/spec-annotations.md for the rules it enforces.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/SloanGwaltney/speclint/internal/speclint"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: speclint [-root dir]")
		flag.PrintDefaults()
	}
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	if flag.NArg() > 0 {
		flag.Usage()
		os.Exit(2)
	}

	problems, err := speclint.Lint(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "speclint:", err)
		os.Exit(2)
	}
	errs, warnings := 0, 0
	for _, p := range problems {
		fmt.Println(p)
		if p.Warning {
			warnings++
		} else {
			errs++
		}
	}
	if len(problems) > 0 {
		fmt.Fprintf(os.Stderr, "speclint: %d problem(s), %d warning(s)\n", errs, warnings)
	}
	if errs > 0 {
		os.Exit(1)
	}
}
