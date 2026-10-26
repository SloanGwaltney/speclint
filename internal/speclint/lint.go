// Package speclint checks that code, tests, and ADRs link correctly to the intents in
// features/*.md, and that intents stay small and high-level, as described in
// docs/spec-annotations.md. A project adjusts what it reads with an optional .speclint.json (see
// config.go).
package speclint

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Problem is one lint finding, reported as "file:line: msg", or "file:line: warning: msg" for a
// warning.
type Problem struct {
	File    string
	Line    int
	Msg     string
	Warning bool // a limit was exceeded and strictLimits is off; doesn't fail the run
}

func (p Problem) String() string {
	if p.Warning {
		return fmt.Sprintf("%s:%d: warning: %s", p.File, p.Line, p.Msg)
	}
	return fmt.Sprintf("%s:%d: %s", p.File, p.Line, p.Msg)
}

// Lint checks the repository at root and returns its problems sorted by file and line.
func Lint(root string) ([]Problem, error) {
	l, err := newLinter(root)
	if err != nil {
		return nil, err
	}
	if err := l.loadIntents(root); err != nil {
		return nil, err
	}
	if err := l.loadAnnotations(root); err != nil {
		return nil, err
	}
	for _, a := range l.annotations {
		l.checkAnnotation(a)
	}
	l.checkReplaces()
	l.checkCoverage()
	l.checkLimits()
	if err := l.checkADRs(root); err != nil {
		return nil, err
	}

	sort.SliceStable(l.problems, func(i, j int) bool {
		a, b := l.problems[i], l.problems[j]
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
	return l.problems, nil
}

// newLinter returns a linter for the repository at root, with its .speclint.json loaded.
func newLinter(root string) (*linter, error) {
	cfg, err := LoadConfig(root)
	if err != nil {
		return nil, err
	}
	return &linter{cfg: cfg, files: map[string]*IntentFile{}}, nil
}

type linter struct {
	cfg   *Config
	files map[string]*IntentFile // keyed by repo-relative path
	// intentFile maps each intent ID to the file that has it. IDs are unique across every intent
	// file, so an @intent annotation needs only the ID.
	intentFile  map[string]string
	annotations []Annotation
	problems    []Problem
}

func (l *linter) report(file string, line int, format string, args ...any) {
	l.problems = append(l.problems, Problem{File: file, Line: line, Msg: fmt.Sprintf(format, args...)})
}

// overLimit reports a limit that was exceeded: a warning, or an error with strictLimits.
func (l *linter) overLimit(file string, line int, format string, args ...any) {
	l.problems = append(l.problems, Problem{File: file, Line: line, Msg: fmt.Sprintf(format, args...), Warning: !l.cfg.StrictLimits})
}

// lookup returns the intent with an ID, from any intent file, or nil.
func (l *linter) lookup(id string) *Intent {
	if f, ok := l.intentFile[id]; ok {
		return l.files[f].intent(id)
	}
	return nil
}

func (l *linter) loadIntents(root string) error {
	dir := filepath.Join(root, filepath.FromSlash(l.cfg.FeaturesDir))

	// speclint v1 read Gherkin .feature files. Say so rather than silently ignoring them.
	old, err := filepath.Glob(filepath.Join(dir, "*.feature"))
	if err != nil {
		return err
	}
	for _, p := range old {
		l.report(l.cfg.FeaturesDir+"/"+filepath.Base(p), 1, "speclint v2 reads intents from %s/*.md, not .feature files; convert this file (see \"Moving from v1\" in the speclint README)", l.cfg.FeaturesDir)
	}

	paths, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return err
	}
	for _, p := range paths {
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel := l.cfg.FeaturesDir + "/" + filepath.Base(p)
		f, problems := parseIntentFile(rel, string(src))
		l.files[rel] = f
		l.problems = append(l.problems, problems...)
	}

	// Glob returns paths sorted, so the first file to use an ID keeps it and later ones are
	// reported.
	l.intentFile = map[string]string{}
	for _, p := range paths {
		rel := l.cfg.FeaturesDir + "/" + filepath.Base(p)
		for _, in := range l.files[rel].Intents {
			if in.ID == "" {
				continue
			}
			if first, dup := l.intentFile[in.ID]; dup {
				l.report(rel, in.Line, "intent ID %q is already used in %s:%d; IDs are unique across intent files", in.ID, first, l.files[first].intent(in.ID).Line)
				continue
			}
			l.intentFile[in.ID] = rel
		}
	}
	return nil
}

// loadAnnotations reads the annotations in every source file. It needs the intents loaded first,
// to find the file of each intent an annotation names.
func (l *linter) loadAnnotations(root string) error {
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && l.cfg.skipDir(d.Name(), rel) {
				return filepath.SkipDir
			}
			return nil
		}
		style, ok := l.cfg.languages[path.Ext(rel)]
		if !ok {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		annotations, problems := parseAnnotations(rel, string(src), style, l.cfg.isTestFile(rel))
		layer, rank := l.cfg.layerOf(rel)
		for i := range annotations {
			annotations[i].Layer, annotations[i].layerRank = layer, rank
			annotations[i].Feature = l.intentFile[annotations[i].ID]
		}
		l.annotations = append(l.annotations, annotations...)
		l.problems = append(l.problems, problems...)
		return nil
	})
	return err
}

// checkAnnotation reports an annotation that doesn't name a live intent.
func (l *linter) checkAnnotation(a Annotation) {
	in := l.lookup(a.ID)
	switch {
	case in == nil:
		l.report(a.File, a.Line, "@intent [#%s]: no intent file has an intent with @id:%s", a.ID, a.ID)
	case in.Status == StatusWithdrawn:
		l.report(a.File, a.Line, "@intent [#%s]: intent is withdrawn; remove it from this line", a.ID)
	}
}

// checkReplaces reports a "Replaces:" line that names an intent that doesn't exist, or one that's
// still active: an intent is replaced by deprecating it.
func (l *linter) checkReplaces() {
	for _, p := range l.sortedFiles() {
		for _, in := range l.files[p].Intents {
			for _, id := range in.Replaces {
				old := l.lookup(id)
				switch {
				case old == nil:
					l.report(p, in.ReplacesLine, "Replaces: no intent file has an intent with @id:%s", id)
				case old == in:
					l.report(p, in.ReplacesLine, "Replaces: an intent can't replace itself")
				case old.Active():
					l.report(p, in.ReplacesLine, "Replaces: @id:%s is still %s; deprecate it when it's replaced", id, old.Status)
				}
			}
		}
	}
}

// checkCoverage reports implemented intents with no @intent annotation in code or in tests.
func (l *linter) checkCoverage() {
	index := map[string]*found{}
	for _, a := range l.annotations {
		if index[a.ID] == nil {
			index[a.ID] = &found{}
		}
		if a.Test {
			index[a.ID].test = true
		} else {
			index[a.ID].code = true
		}
	}
	for _, p := range l.sortedFiles() {
		for _, in := range l.files[p].Intents {
			if in.Status != StatusImplemented || in.ID == "" || l.intentFile[in.ID] != p {
				continue
			}
			got := found{}
			if v := index[in.ID]; v != nil {
				got = *v
			}
			msg := fmt.Sprintf("implemented intent %s has no @intent [#%s] annotation", in.ID, in.ID)
			switch {
			case !got.code && !got.test:
				l.report(p, in.Line, "%s in code or tests", msg)
			case !got.code:
				l.report(p, in.Line, "%s in code", msg)
			case !got.test:
				l.report(p, in.Line, "%s in tests", msg)
			}
		}
	}
}

// checkLimits reports intent files, statements, and entry points that say more than the limits
// allow. They're the sign that intents are turning into a list of every behavior.
func (l *linter) checkLimits() {
	lim := l.cfg.Limits
	for _, p := range l.sortedFiles() {
		f := l.files[p]
		active := 0
		for _, in := range f.Intents {
			if in.Active() {
				active++
			}
			if words := len(strings.Fields(in.Statement)); lim.StatementWords > 0 && words > lim.StatementWords && in.Active() {
				l.overLimit(p, in.Line, "intent %s's statement is %d words (limit %d); state the obligation in one sentence and move the detail to its notes", label(in), words, lim.StatementWords)
			}
		}
		if lim.IntentsPerFile > 0 && active > lim.IntentsPerFile {
			l.overLimit(p, 1, "%d active intents (limit %d); keep the ones the feature would be wrong without, and leave the rest to tests, or split the file", active, lim.IntentsPerFile)
		}
	}

	if lim.IntentsPerEntryPoint > 0 {
		seen := map[string]bool{}
		for _, a := range l.annotations {
			key := fmt.Sprintf("%s:%d", a.File, a.Line)
			if a.Test || seen[key] || a.Count <= lim.IntentsPerEntryPoint {
				continue
			}
			seen[key] = true
			l.overLimit(a.File, a.Line, "@intent names %d intents (limit %d); an entry point with this many is either doing too much or its intents are too detailed", a.Count, lim.IntentsPerEntryPoint)
		}
	}
}

func (l *linter) sortedFiles() []string {
	paths := make([]string, 0, len(l.files))
	for p := range l.files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// found records whether something is annotated in code, in tests, or both.
type found struct{ code, test bool }
