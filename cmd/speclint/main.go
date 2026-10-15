// Command speclint checks that code, tests, and ADRs link correctly to the specs in
// features/*.feature. It's its own Go module, usually a git submodule at tools/speclint, so run it
// from the repository root with -C:
//
//	go run -C tools/speclint ./cmd/speclint -root ../..
//
// It prints one "file:line: problem" per finding and exits 1 if there are any. See
// docs/spec-annotations.md for the rules it enforces.
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
	for _, p := range problems {
		fmt.Println(p)
	}
	if len(problems) > 0 {
		fmt.Fprintf(os.Stderr, "speclint: %d problem(s)\n", len(problems))
		os.Exit(1)
	}
}
