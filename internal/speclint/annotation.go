package speclint

import (
	"fmt"
	"regexp"
	"strings"
)

// Annotation is one intent named on an @intent line in code or tests, e.g. the first of
// "@intent [#some-id] [#other-id]".
type Annotation struct {
	ID   string // the intent ID after '#'
	File string // repo-relative path of the source file
	Line int
	Test bool // true when the source file is a test
	// Count is how many intents the @intent line names, this one included.
	Count int
	// Feature is the intent file that has the ID, filled in once the intent files are loaded, or
	// "" if none has it.
	Feature string
	// Layer names where the source file sits in the app, e.g. "storage" (see layerOf in
	// config.go). layerRank orders layers from the bottom of the stack up.
	Layer     string
	layerRank int
}

// Where names the layer an annotation sits in, and whether it's a test, e.g. "storage test".
func (a Annotation) Where() string {
	if a.Test {
		return a.Layer + " test"
	}
	return a.Layer
}

// keywordPattern finds an annotation's keyword. Step keywords and @scenario are speclint v1
// annotations, found so they can be reported.
var keywordPattern = regexp.MustCompile(`@(intent|scenario|given|when|then|and|but)\b`)

// intentRef is one "[#id]" on an @intent line.
var intentRef = regexp.MustCompile(`^\[#([^\]\s]*)\]`)

// parseAnnotations finds the @intent annotations in a source file. An annotation must be the only
// thing in its comment, on a line of its own, so a string that happens to look like one is
// ignored. It reports an @intent line that isn't a list of [#id] references, a second @intent line
// in the same comment block, since one line names every intent of the code under it, and
// annotations left over from speclint v1. style says how the file's language writes comments.
func parseAnnotations(file, src string, style commentStyle, test bool) ([]Annotation, []Problem) {
	var out []Annotation
	var problems []Problem
	report := func(line int, msg string) {
		problems = append(problems, Problem{File: file, Line: line, Msg: msg})
	}
	intentLine := 0 // the @intent line in the current comment block, or 0
	for i, raw := range strings.Split(src, "\n") {
		line := i + 1
		if !style.line.MatchString(raw) {
			intentLine = 0
		}
		loc := keywordPattern.FindStringSubmatchIndex(raw)
		if loc == nil || !style.prefix.MatchString(raw[:loc[0]]) {
			continue
		}
		keyword := raw[loc[2]:loc[3]]
		rest := closeComment(raw[loc[1]:], style)
		// Prose that wraps onto a line starting with "@intent" or "@then" isn't an annotation.
		if !strings.HasPrefix(rest, "[") {
			continue
		}

		switch keyword {
		case "intent":
		case "scenario":
			report(line, "@scenario is speclint v1; link intents with @intent [#id]")
			continue
		default:
			report(line, fmt.Sprintf("@%s is a speclint v1 step annotation; v2 links intents, not steps, so remove it", keyword))
			continue
		}

		ids, ok := intentRefs(rest)
		if !ok {
			report(line, "@intent names intents by ID only, like @intent [#some-id] [#other-id]")
			continue
		}
		if intentLine != 0 {
			report(line, fmt.Sprintf("@intent is already on line %d of this comment; name every intent on that one line", intentLine))
		} else {
			intentLine = line
		}
		seen := map[string]bool{}
		var named []Annotation
		for _, id := range ids {
			if seen[id] {
				report(line, "@intent names [#"+id+"] twice")
				continue
			}
			seen[id] = true
			named = append(named, Annotation{ID: id, File: file, Line: line, Test: test})
		}
		for i := range named {
			named[i].Count = len(named)
		}
		out = append(out, named...)
	}
	return out, problems
}

// closeComment trims the end of a block comment that closes on the same line, like */ or */}.
func closeComment(s string, style commentStyle) string {
	s = strings.TrimSpace(s)
	for _, c := range style.closers {
		if t, ok := strings.CutSuffix(s, c); ok {
			return strings.TrimSpace(t)
		}
	}
	return s
}

// intentRefs reads the IDs from what follows @intent: one or more [#id], separated by spaces, and
// nothing else.
func intentRefs(s string) ([]string, bool) {
	var ids []string
	for s = strings.TrimSpace(s); s != ""; s = strings.TrimSpace(s) {
		m := intentRef.FindStringSubmatch(s)
		if m == nil {
			return nil, false
		}
		ids = append(ids, m[1])
		s = s[len(m[0]):]
		if s != "" && s[0] != ' ' && s[0] != '\t' {
			return nil, false
		}
	}
	return ids, len(ids) > 0
}
