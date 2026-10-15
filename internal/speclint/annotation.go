package speclint

import (
	"fmt"
	"regexp"
	"strings"
)

// Annotation is one spec link in code or tests: one scenario named on a @scenario line, e.g.
// "@scenario [#some-id] [#other-id]", or a step annotation, e.g.
// "@then [features/x.feature#some-id] I receive a reminder".
type Annotation struct {
	Keyword string // lowercase: scenario, given, when, then, and, but
	// Feature is the bracketed path on a step annotation. A @scenario annotation names no path, so
	// its Feature is the file whose scenario has the ID, filled in once the features are loaded.
	Feature string
	ID      string // the scenario ID after '#', or "" if there isn't one
	Text    string // the step text; empty for @scenario
	File    string // repo-relative path of the source file
	Line    int
	Test    bool // true when the source file is a test
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

// keywordPattern finds an annotation's keyword. What follows it is read by scenarioRefs or
// stepPattern.
var keywordPattern = regexp.MustCompile(`@(scenario|given|when|then|and|but)\b`)

// The path and ID are matched loosely so that malformed references get reported rather than
// silently skipped.
var stepPattern = regexp.MustCompile(`^\s+\[([^\]#]*?)(?:#([^\]]*))?\](.*)`)

// scenarioRef is one "[#id]" on a @scenario line.
var scenarioRef = regexp.MustCompile(`^\[#([^\]\s]*)\]`)

// parseAnnotations finds the spec annotations in a source file. An annotation must be the only
// thing in its comment, on a line of its own, so a string that happens to look like one is
// ignored. It reports a @scenario line that isn't a list of [#id] references, and a second
// @scenario line in the same comment block, since one line names every scenario of the code
// under it. style says how the file's language writes comments.
func parseAnnotations(file, src string, style commentStyle, test bool) ([]Annotation, []Problem) {
	var out []Annotation
	var problems []Problem
	report := func(line int, msg string) {
		problems = append(problems, Problem{File: file, Line: line, Msg: msg})
	}
	scenarioLine := 0 // the @scenario line in the current comment block, or 0
	for i, raw := range strings.Split(src, "\n") {
		line := i + 1
		if !style.line.MatchString(raw) {
			scenarioLine = 0
		}
		loc := keywordPattern.FindStringSubmatchIndex(raw)
		if loc == nil || !style.prefix.MatchString(raw[:loc[0]]) {
			continue
		}
		keyword := raw[loc[2]:loc[3]]
		rest := raw[loc[1]:]

		if keyword == "scenario" {
			// Prose that wraps onto a line starting with "@scenario" isn't an annotation.
			if !strings.HasPrefix(strings.TrimSpace(rest), "[") {
				continue
			}
			ids, ok := scenarioRefs(closeComment(rest, style))
			if !ok {
				report(line, "@scenario names scenarios by ID only, like @scenario [#some-id] [#other-id]")
				continue
			}
			if scenarioLine != 0 {
				report(line, fmt.Sprintf("@scenario is already on line %d of this comment; name every scenario on that one line", scenarioLine))
			} else {
				scenarioLine = line
			}
			seen := map[string]bool{}
			for _, id := range ids {
				if seen[id] {
					report(line, "@scenario names [#"+id+"] twice")
					continue
				}
				seen[id] = true
				out = append(out, Annotation{Keyword: keyword, ID: id, File: file, Line: line, Test: test})
			}
			continue
		}

		m := stepPattern.FindStringSubmatch(rest)
		if m == nil {
			continue
		}
		out = append(out, Annotation{
			Keyword: keyword,
			Feature: strings.TrimSpace(m[1]),
			ID:      strings.TrimSpace(m[2]),
			Text:    closeComment(m[3], style),
			File:    file,
			Line:    line,
			Test:    test,
		})
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

// scenarioRefs reads the IDs from what follows @scenario: one or more [#id], separated by spaces,
// and nothing else.
func scenarioRefs(s string) ([]string, bool) {
	var ids []string
	for s = strings.TrimSpace(s); s != ""; s = strings.TrimSpace(s) {
		m := scenarioRef.FindStringSubmatch(s)
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
