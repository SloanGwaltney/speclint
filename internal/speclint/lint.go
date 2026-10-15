// Package speclint checks that code, tests, and ADRs link correctly to the specs in
// features/*.feature, as described in docs/spec-annotations.md. A project adjusts what it reads
// with an optional .speclint.json (see config.go).
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

// Problem is one lint finding, reported as "file:line: msg".
type Problem struct {
	File string
	Line int
	Msg  string
}

func (p Problem) String() string {
	return fmt.Sprintf("%s:%d: %s", p.File, p.Line, p.Msg)
}

// Lint checks the repository at root and returns its problems sorted by file and line.
func Lint(root string) ([]Problem, error) {
	l, err := newLinter(root)
	if err != nil {
		return nil, err
	}
	if err := l.loadFeatures(root); err != nil {
		return nil, err
	}
	if err := l.loadAnnotations(root); err != nil {
		return nil, err
	}
	for _, a := range l.annotations {
		l.checkAnnotation(a)
	}
	l.checkCoverage()
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
	return &linter{cfg: cfg, features: map[string]*Feature{}}, nil
}

type linter struct {
	cfg      *Config
	features map[string]*Feature // keyed by repo-relative path
	// scenarioFeature maps each scenario ID to the feature file that has it. IDs are unique across
	// every feature file, so a @scenario annotation needs only the ID.
	scenarioFeature map[string]string
	annotations     []Annotation
	problems        []Problem
}

func (l *linter) report(file string, line int, format string, args ...any) {
	l.problems = append(l.problems, Problem{File: file, Line: line, Msg: fmt.Sprintf(format, args...)})
}

func (l *linter) loadFeatures(root string) error {
	paths, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(l.cfg.FeaturesDir), "*.feature"))
	if err != nil {
		return err
	}
	for _, p := range paths {
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel := l.cfg.FeaturesDir + "/" + filepath.Base(p)
		f, problems := parseFeature(rel, string(src))
		l.features[rel] = f
		l.problems = append(l.problems, problems...)
	}

	// Glob returns paths sorted, so the first file to use an ID keeps it and later ones are
	// reported.
	l.scenarioFeature = map[string]string{}
	for _, p := range paths {
		rel := l.cfg.FeaturesDir + "/" + filepath.Base(p)
		for _, s := range l.features[rel].Scenarios {
			if s.ID == "" {
				continue
			}
			if first, dup := l.scenarioFeature[s.ID]; dup {
				l.report(rel, s.Line, "scenario ID %q is already used in %s:%d; IDs are unique across feature files", s.ID, first, l.features[first].scenario(s.ID).Line)
				continue
			}
			l.scenarioFeature[s.ID] = rel
		}
	}
	return nil
}

// loadAnnotations reads the annotations in every source file. It needs the features loaded first,
// to find the feature file of each scenario a @scenario annotation names.
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
		}
		l.annotations = append(l.annotations, annotations...)
		l.problems = append(l.problems, problems...)
		return nil
	})
	if err != nil {
		return err
	}
	for i, a := range l.annotations {
		if a.Keyword == "scenario" {
			l.annotations[i].Feature = l.scenarioFeature[a.ID]
		}
	}
	return nil
}

// checkAnnotation reports an annotation that doesn't resolve to a live scenario or step in the
// spec it names.
func (l *linter) checkAnnotation(a Annotation) {
	if a.Keyword == "scenario" {
		switch {
		case a.Feature == "":
			l.report(a.File, a.Line, "@scenario [#%s]: no feature file has a scenario with @id:%s", a.ID, a.ID)
		case l.features[a.Feature].scenario(a.ID).Status == StatusWithdrawn:
			l.report(a.File, a.Line, "@scenario [#%s]: scenario is withdrawn; remove it from this line", a.ID)
		}
		return
	}

	ref := a.Feature
	if a.ID != "" {
		ref += "#" + a.ID
	}

	if !strings.HasPrefix(a.Feature, l.cfg.FeaturesDir+"/") || !strings.HasSuffix(a.Feature, ".feature") || strings.Contains(a.Feature, `\`) {
		l.report(a.File, a.Line, "@%s [%s]: the path must be repo-relative with forward slashes, like [%s/x.feature]", a.Keyword, ref, l.cfg.FeaturesDir)
		return
	}
	f, ok := l.features[a.Feature]
	if !ok {
		l.report(a.File, a.Line, "@%s [%s]: %s doesn't exist", a.Keyword, ref, a.Feature)
		return
	}

	var scoped *Scenario
	if a.ID != "" {
		if scoped = f.scenario(a.ID); scoped == nil {
			l.report(a.File, a.Line, "@%s [%s]: %s has no scenario with @id:%s", a.Keyword, ref, a.Feature, a.ID)
			return
		}
		if scoped.Status == StatusWithdrawn {
			l.report(a.File, a.Line, "@%s [%s]: scenario is withdrawn; remove this annotation", a.Keyword, ref)
			return
		}
	}

	candidates := f.Scenarios
	if scoped != nil {
		candidates = []*Scenario{scoped}
	}
	var live, withdrawn bool
	var otherKeyword string
	for _, s := range candidates {
		for _, st := range s.Steps {
			if st.Text != a.Text {
				continue
			}
			if !st.Accepts(a.Keyword) {
				otherKeyword = "@" + strings.ToLower(st.Keyword)
				if st.Kind != strings.ToLower(st.Keyword) {
					otherKeyword += " or @" + st.Kind
				}
				continue
			}
			if s.Status == StatusWithdrawn {
				withdrawn = true
			} else {
				live = true
			}
		}
	}
	where := a.Feature
	if scoped != nil {
		where = "scenario " + scoped.ID
	}
	switch {
	case live:
	case withdrawn:
		l.report(a.File, a.Line, "@%s [%s] %s: step is only used by withdrawn scenarios; remove this annotation", a.Keyword, ref, a.Text)
	case otherKeyword != "":
		l.report(a.File, a.Line, "@%s [%s] %s: the spec's step takes %s", a.Keyword, ref, a.Text, otherKeyword)
	default:
		l.report(a.File, a.Line, "@%s [%s] %s: no such step in %s", a.Keyword, ref, a.Text, where)
	}
}

// checkCoverage reports implemented scenarios with no @scenario annotation in code or in tests,
// and steps of them with no annotation in tests. Step annotations in code are allowed but not
// required.
func (l *linter) checkCoverage() {
	type key struct{ feature, keyword, id, text string }
	index := map[key]*found{}
	for _, a := range l.annotations {
		k := key{a.Feature, a.Keyword, a.ID, a.Text}
		if index[k] == nil {
			index[k] = &found{}
		}
		if a.Test {
			index[k].test = true
		} else {
			index[k].code = true
		}
	}
	has := func(k key) found {
		if v := index[k]; v != nil {
			return *v
		}
		return found{}
	}

	paths := make([]string, 0, len(l.features))
	for p := range l.features {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, p := range paths {
		for _, s := range l.features[p].Scenarios {
			if s.Status != StatusImplemented || s.ID == "" {
				continue
			}
			got := has(key{p, "scenario", s.ID, ""})
			l.reportMissing(p, s.Line, got, fmt.Sprintf("implemented scenario %s has no @scenario [#%s] annotation", s.ID, s.ID))

			for _, st := range s.Steps {
				kw := strings.ToLower(st.Keyword)
				got := found{code: true}
				for _, k := range []string{kw, st.Kind} {
					for _, id := range []string{"", s.ID} {
						got.test = got.test || has(key{p, k, id, st.Text}).test
					}
				}
				want := "@" + kw
				if st.Kind != kw {
					want += " or @" + st.Kind
				}
				l.reportMissing(p, st.Line, got, fmt.Sprintf("step %q of implemented scenario %s has no %s [%s] annotation", st.Keyword+" "+st.Text, s.ID, want, p))
			}
		}
	}
}

func (l *linter) reportMissing(file string, line int, got found, msg string) {
	switch {
	case !got.code && !got.test:
		l.report(file, line, "%s in code or tests", msg)
	case !got.code:
		l.report(file, line, "%s in code", msg)
	case !got.test:
		l.report(file, line, "%s in tests", msg)
	}
}

// found records whether something is annotated in code, in tests, or both.
type found struct{ code, test bool }
