package speclint

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Trace is everything linked to one scenario: the annotations on its entry points and on each
// step, and the ADRs whose Related section names it. cmd/spectrace prints it.
type Trace struct {
	Feature  string // repo-relative path of the .feature file
	Scenario *Scenario
	// Entry holds the @scenario annotations, Steps the step annotations in step order.
	Entry []Annotation
	Steps []StepTrace
	ADRs  []ADRRef
}

// StepTrace is one step of the scenario and the annotations that carry it out or test it.
type StepTrace struct {
	Step  Step
	Links []Annotation
}

// ADRRef is a line in an ADR's Related section that links to the scenario, or to its whole
// feature file when Whole is set.
type ADRRef struct {
	File  string
	Line  int
	Whole bool
}

// Missing lists what the scenario lacks, worded for a person to read: a link in code or in tests
// for its @scenario, and a link in tests for each step. Step links in code are optional. It's
// empty when the scenario is fully linked.
func (t *Trace) Missing() []string {
	var out []string
	check := func(what string, links []Annotation, needCode bool) {
		var code, test bool
		for _, a := range links {
			if a.Test {
				test = true
			} else {
				code = true
			}
		}
		if needCode && !code {
			out = append(out, what+" has no link in code")
		}
		if !test {
			out = append(out, what+" has no link in tests")
		}
	}
	check("@scenario", t.Entry, true)
	for _, s := range t.Steps {
		check(fmt.Sprintf("%s %s", s.Step.Keyword, s.Step.Text), s.Links, false)
	}
	return out
}

// TraceScenario finds the scenario ref names, "<id>" or "features/x.feature#<id>", and collects
// everything linked to it, with the same matching rules speclint uses: a step annotation counts
// when its file, keyword, and text match, and it has no ID or the scenario's ID.
func TraceScenario(root, ref string) (*Trace, error) {
	l, err := newLinter(root)
	if err != nil {
		return nil, err
	}
	if err := l.loadFeatures(root); err != nil {
		return nil, err
	}

	feature, id, _ := strings.Cut(ref, "#")
	if id == "" {
		feature, id = "", feature
	}
	var matches []*Trace
	for path, f := range l.features {
		if feature != "" && path != feature {
			continue
		}
		if s := f.scenario(id); s != nil {
			matches = append(matches, &Trace{Feature: path, Scenario: s})
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no scenario has @id:%s in %s/*.feature", id, l.cfg.FeaturesDir)
	case 1:
	default:
		var paths []string
		for _, m := range matches {
			paths = append(paths, m.Feature+"#"+id)
		}
		sort.Strings(paths)
		return nil, fmt.Errorf("more than one scenario has @id:%s; name one of %s", id, strings.Join(paths, ", "))
	}

	if err := l.loadAnnotations(root); err != nil {
		return nil, err
	}
	t := matches[0]
	if err := l.link(root, t); err != nil {
		return nil, err
	}
	return t, nil
}

// TraceFeature traces every scenario in the feature file at path, e.g.
// "features/job-board.feature", in the order they're written. Withdrawn scenarios are left out,
// since they're removed from the app and have nothing left to link.
func TraceFeature(root, path string) ([]*Trace, error) {
	l, err := newLinter(root)
	if err != nil {
		return nil, err
	}
	if err := l.loadFeatures(root); err != nil {
		return nil, err
	}
	f := l.features[path]
	if f == nil {
		return nil, fmt.Errorf("no feature file %s; name one as %s/<file>.feature", path, l.cfg.FeaturesDir)
	}
	if err := l.loadAnnotations(root); err != nil {
		return nil, err
	}
	var traces []*Trace
	for _, s := range f.Scenarios {
		if s.Status == "withdrawn" {
			continue
		}
		t := &Trace{Feature: path, Scenario: s}
		if err := l.link(root, t); err != nil {
			return nil, err
		}
		traces = append(traces, t)
	}
	return traces, nil
}

// link fills in t's entry points, step links, and ADRs from the loaded annotations.
func (l *linter) link(root string, t *Trace) error {
	id := t.Scenario.ID
	t.Steps = make([]StepTrace, len(t.Scenario.Steps))
	for i, st := range t.Scenario.Steps {
		t.Steps[i].Step = st
	}
	for _, a := range l.annotations {
		if a.Feature != t.Feature {
			continue
		}
		if a.Keyword == "scenario" {
			if a.ID == id {
				t.Entry = append(t.Entry, a)
			}
			continue
		}
		if a.ID != "" && a.ID != id {
			continue
		}
		for i, st := range t.Scenario.Steps {
			if st.Accepts(a.Keyword) && a.Text == st.Text {
				t.Steps[i].Links = append(t.Steps[i].Links, a)
			}
		}
	}
	sortByLayer(t.Entry)
	for i := range t.Steps {
		sortByLayer(t.Steps[i].Links)
	}

	refs, err := l.adrRefs(root, t.Feature, id)
	if err != nil {
		return err
	}
	t.ADRs = refs
	return nil
}

// adrRefs returns the Related entries in docs/adr/*.md that link to the scenario or its feature.
func (l *linter) adrRefs(root, feature, id string) ([]ADRRef, error) {
	paths, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(l.cfg.ADRDir), "*.md"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var refs []ADRRef
	for _, p := range paths {
		src, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		file := l.cfg.ADRDir + "/" + filepath.Base(p)
		inRelated, inFence := false, false
		for i, raw := range strings.Split(string(src), "\n") {
			text := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
			switch {
			case strings.HasPrefix(text, "```"):
				inFence = !inFence
			case inFence:
			case strings.HasPrefix(text, "## "):
				inRelated = text == "## Related"
			case inRelated:
				m := relatedEntryPattern.FindStringSubmatch(text)
				if m != nil && m[1] == feature && (m[2] == id || m[2] == "") {
					refs = append(refs, ADRRef{File: file, Line: i + 1, Whole: m[2] == ""})
				}
			}
		}
	}
	return refs, nil
}

// sortByLayer orders annotations by layer, bottom of the stack first, with each layer's code before
// its tests. Files outside the named layers are grouped by top-level directory.
func sortByLayer(as []Annotation) {
	sort.SliceStable(as, func(i, j int) bool {
		a, b := as[i], as[j]
		if a.layerRank != b.layerRank {
			return a.layerRank < b.layerRank
		}
		if a.Layer != b.Layer {
			return a.Layer < b.Layer
		}
		if a.Test != b.Test {
			return !a.Test
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
}
