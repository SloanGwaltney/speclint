package speclint

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Trace is one intent and everything linked to it: the @intent annotations in code and tests,
// and the ADRs whose Related section names it or its file. cmd/spectrace prints it.
type Trace struct {
	File   *IntentFile
	Intent *Intent
	Links  []Annotation // ordered by layer, bottom of the stack first
	ADRs   []ADRRef
}

// ADRRef is a line in an ADR's Related section that links to the intent, or to its whole intent
// file when Whole is set.
type ADRRef struct {
	File  string
	Line  int
	Whole bool
}

// Missing lists what the intent lacks, worded for a person to read: an @intent link in code, and
// one in tests. It's empty when the intent is linked in both.
func (t *Trace) Missing() []string {
	var code, test bool
	for _, a := range t.Links {
		if a.Test {
			test = true
		} else {
			code = true
		}
	}
	var out []string
	if !code {
		out = append(out, "no @intent link in code")
	}
	if !test {
		out = append(out, "no @intent link in tests")
	}
	return out
}

// TraceIntent finds the intent ref names, "<id>" or "features/x.md#<id>", and collects everything
// linked to it.
func TraceIntent(root, ref string) (*Trace, error) {
	l, err := newLinter(root)
	if err != nil {
		return nil, err
	}
	if err := l.loadIntents(root); err != nil {
		return nil, err
	}

	file, id, _ := strings.Cut(ref, "#")
	if id == "" {
		file, id = "", file
	}
	var matches []*Trace
	for p, f := range l.files {
		if file != "" && p != file {
			continue
		}
		if in := f.intent(id); in != nil {
			matches = append(matches, &Trace{File: f, Intent: in})
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no intent has @id:%s in %s/*.md", id, l.cfg.FeaturesDir)
	case 1:
	default:
		var paths []string
		for _, m := range matches {
			paths = append(paths, m.File.Path+"#"+id)
		}
		sort.Strings(paths)
		return nil, fmt.Errorf("more than one intent has @id:%s; name one of %s", id, strings.Join(paths, ", "))
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

// TraceFile traces every intent in the intent file at path, e.g. "features/job-board.md", in the
// order they're written. Withdrawn intents are left out, since they're removed from the app and
// have nothing left to link.
func TraceFile(root, path string) (*IntentFile, []*Trace, error) {
	l, err := newLinter(root)
	if err != nil {
		return nil, nil, err
	}
	if err := l.loadIntents(root); err != nil {
		return nil, nil, err
	}
	f := l.files[path]
	if f == nil {
		return nil, nil, fmt.Errorf("no intent file %s; name one as %s/<file>.md", path, l.cfg.FeaturesDir)
	}
	if err := l.loadAnnotations(root); err != nil {
		return nil, nil, err
	}
	var traces []*Trace
	for _, in := range f.Intents {
		if in.Status == StatusWithdrawn {
			continue
		}
		t := &Trace{File: f, Intent: in}
		if err := l.link(root, t); err != nil {
			return nil, nil, err
		}
		traces = append(traces, t)
	}
	return f, traces, nil
}

// link fills in t's annotations and ADRs.
func (l *linter) link(root string, t *Trace) error {
	for _, a := range l.annotations {
		if a.ID == t.Intent.ID && a.Feature == t.File.Path {
			t.Links = append(t.Links, a)
		}
	}
	sortByLayer(t.Links)

	refs, err := l.adrRefs(root, t.File.Path, t.Intent.ID)
	if err != nil {
		return err
	}
	t.ADRs = refs
	return nil
}

// adrRefs returns the Related entries in docs/adr/*.md that link to the intent or its file.
func (l *linter) adrRefs(root, file, id string) ([]ADRRef, error) {
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
		adr := l.cfg.ADRDir + "/" + filepath.Base(p)
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
				if m != nil && m[1] == file && (m[2] == id || m[2] == "") {
					refs = append(refs, ADRRef{File: adr, Line: i + 1, Whole: m[2] == ""})
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
