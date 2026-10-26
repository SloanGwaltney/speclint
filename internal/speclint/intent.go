package speclint

import (
	"fmt"
	"regexp"
	"strings"
)

// Statuses an intent can carry, in the order an intent moves through them.
const (
	StatusNotImplemented = "not-implemented"
	StatusImplemented    = "implemented"
	StatusDeprecated     = "deprecated"
	StatusWithdrawn      = "withdrawn"
)

var validStatuses = map[string]bool{
	StatusNotImplemented: true,
	StatusImplemented:    true,
	StatusDeprecated:     true,
	StatusWithdrawn:      true,
}

// Kinds of intent: something the feature must do, or something it must never do. The kind is set
// by the section the intent is listed under.
const (
	KindMust    = "must"
	KindMustNot = "must-not"
)

var idPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// replacesPattern finds the IDs on a "Replaces: @id:a @id:b" note line.
var replacesPattern = regexp.MustCompile(`@id:(\S+)`)

// IntentFile is one parsed features/*.md file: a title, a purpose, and the intents listed under its
// Must and Must not sections.
type IntentFile struct {
	Path    string // repo-relative, forward slashes
	Title   string
	Purpose string // the paragraphs between the title and the first section
	Intents []*Intent
}

// Intent is one obligation: something the feature must or must not do.
type Intent struct {
	ID        string
	Status    string
	Kind      string // KindMust or KindMustNot
	Statement string // the obligation itself, joined onto one line
	Notes     string // anything indented under the item after the statement, as written
	Line      int
	// Replaces lists the IDs named on a "Replaces:" note line, with the line they're on.
	Replaces     []string
	ReplacesLine int
}

// Active reports whether the intent describes something the app does or will do.
func (in *Intent) Active() bool {
	return in.Status == StatusNotImplemented || in.Status == StatusImplemented
}

func (f *IntentFile) intent(id string) *Intent {
	for _, in := range f.Intents {
		if in.ID == id {
			return in
		}
	}
	return nil
}

// parseIntentFile reads a features/*.md file and reports what's malformed in it.
//
// The format: a "# Title" line, a purpose paragraph, then "## Must" and "## Must not" sections.
// Each list item in those sections is an intent: it starts with "- @id:<id> @status:<status>",
// then the statement, on the same line or the indented lines after it. Anything indented under
// the item after a blank line is notes, which aren't checked except for a "Replaces:" line.
// Other "##" sections are free-form.
func parseIntentFile(path, src string) (*IntentFile, []Problem) {
	f := &IntentFile{Path: path}
	var problems []Problem
	report := func(line int, format string, args ...any) {
		problems = append(problems, Problem{File: path, Line: line, Msg: fmt.Sprintf(format, args...)})
	}

	const (
		beforeTitle = iota
		inPurpose
		inSection
	)
	state := beforeTitle
	titleLine := 0
	kind := "" // the section's kind, or "" in a free-form section
	var purpose []string
	var current *Intent
	var statement, notes []string
	inStatement := false
	fence := ""
	seenIDs := map[string]int{}

	finish := func() {
		if current == nil {
			return
		}
		current.Statement = strings.Join(statement, " ")
		current.Notes = strings.TrimRight(dedent(notes), "\n")
		if current.Statement == "" {
			report(current.Line, "intent %s has no statement; write the obligation on the line after its tags", label(current))
		}
		current, statement, notes, inStatement = nil, nil, nil, false
	}

	for i, raw := range strings.Split(src, "\n") {
		line := i + 1
		raw = strings.TrimSuffix(raw, "\r")
		text := strings.TrimSpace(raw)
		indented := raw != "" && (raw[0] == ' ' || raw[0] == '\t')

		// Fenced code is kept in notes but never read as structure.
		if fence != "" || strings.HasPrefix(text, "```") || strings.HasPrefix(text, "~~~") {
			switch {
			case fence == "":
				fence = text[:3]
			case strings.HasPrefix(text, fence):
				fence = ""
			}
			switch {
			case current != nil:
				inStatement = false
				notes = append(notes, raw)
			case state == inPurpose:
				purpose = append(purpose, text)
			}
			continue
		}

		switch {
		case state == beforeTitle:
			if t, ok := strings.CutPrefix(text, "# "); ok {
				f.Title, titleLine, state = strings.TrimSpace(t), line, inPurpose
			} else if text != "" {
				report(line, "an intent file starts with a \"# Title\" line")
				titleLine, state = -1, inPurpose
			}

		case strings.HasPrefix(text, "## ") && !indented:
			finish()
			state = inSection
			switch strings.ToLower(strings.TrimSpace(strings.TrimPrefix(text, "## "))) {
			case "must":
				kind = KindMust
			case "must not":
				kind = KindMustNot
			default:
				kind = ""
			}

		case strings.HasPrefix(text, "# ") && !indented:
			finish()
			report(line, "an intent file has one \"# Title\"; use \"## Must\", \"## Must not\", or another \"##\" section")

		case state == inPurpose:
			purpose = append(purpose, text)

		case (strings.HasPrefix(text, "- ") || strings.HasPrefix(text, "* ")) && !indented:
			finish()
			item := strings.TrimSpace(text[2:])
			tagged := strings.HasPrefix(item, "@id:") || strings.HasPrefix(item, "@status:")
			if kind == "" {
				if tagged {
					report(line, "intents go under \"## Must\" or \"## Must not\"")
				}
				continue
			}
			if !tagged {
				report(line, "every item under \"## %s\" is an intent; start it with @id: and @status:", sectionName(kind))
				continue
			}
			current = &Intent{Kind: kind, Line: line}
			f.Intents = append(f.Intents, current)
			rest := readTags(current, item, line, report)
			if rest != "" {
				statement = append(statement, rest)
			}
			inStatement = true
			if current.ID != "" {
				if first, dup := seenIDs[current.ID]; dup {
					report(line, "intent ID %q is already used on line %d", current.ID, first)
					current.ID = ""
				} else {
					seenIDs[current.ID] = line
				}
			}

		case current != nil && (indented || text == ""):
			if text == "" {
				if len(statement) > 0 {
					inStatement = false
				}
				if !inStatement {
					notes = append(notes, "")
				}
				continue
			}
			r, replaces := strings.CutPrefix(text, "Replaces:")
			if replaces {
				// A Replaces line is a note, even straight after the statement.
				inStatement = false
			}
			if inStatement {
				statement = append(statement, text)
			} else {
				notes = append(notes, raw)
			}
			if replaces {
				for _, m := range replacesPattern.FindAllStringSubmatch(r, -1) {
					current.Replaces = append(current.Replaces, m[1])
				}
				current.ReplacesLine = line
				if len(current.Replaces) == 0 {
					report(line, "Replaces: names the intents this one replaces, like \"Replaces: @id:old-id\"")
				}
			}

		default:
			// Prose in a section, outside any item.
			finish()
		}
	}
	finish()

	f.Purpose = strings.TrimSpace(strings.Join(collapse(purpose), "\n"))
	switch {
	case titleLine == 0:
		report(1, "an intent file starts with a \"# Title\" line")
	case titleLine > 0 && f.Purpose == "":
		report(titleLine, "intent file %q has no purpose; say in a paragraph under the title what the feature is for", f.Title)
	}
	return f, problems
}

// readTags reads the @id: and @status: tags at the start of an intent's first line and returns
// the rest of the line, which is the start of the statement.
func readTags(in *Intent, item string, line int, report func(int, string, ...any)) string {
	var statuses, ids []string
	fields := strings.Fields(item)
	n := 0
	for _, field := range fields {
		if !strings.HasPrefix(field, "@") {
			break
		}
		n++
		switch {
		case strings.HasPrefix(field, "@id:"):
			ids = append(ids, strings.TrimPrefix(field, "@id:"))
		case strings.HasPrefix(field, "@status:"):
			statuses = append(statuses, strings.TrimPrefix(field, "@status:"))
		default:
			report(line, "unknown tag %s; an intent has only @id: and @status:", field)
		}
	}

	switch {
	case len(ids) == 0:
		report(line, "intent has no @id: tag")
	case len(ids) > 1:
		report(line, "intent has %d @id: tags; it needs exactly one", len(ids))
	case !idPattern.MatchString(ids[0]):
		report(line, "intent ID %q isn't kebab-case (lowercase letters, digits, hyphens)", ids[0])
	default:
		in.ID = ids[0]
	}

	switch {
	case len(statuses) == 0:
		report(line, "intent %s has no @status: tag", label(in))
	case len(statuses) > 1:
		report(line, "intent %s has %d @status: tags; it needs exactly one", label(in), len(statuses))
	case !validStatuses[statuses[0]]:
		report(line, "intent %s has unknown status %q", label(in), statuses[0])
	default:
		in.Status = statuses[0]
	}
	return strings.Join(fields[n:], " ")
}

func label(in *Intent) string {
	if in.ID == "" {
		return "on this line"
	}
	return in.ID
}

func sectionName(kind string) string {
	if kind == KindMustNot {
		return "Must not"
	}
	return "Must"
}

// dedent removes the indentation every non-blank line shares, and leading and trailing blank lines.
func dedent(lines []string) string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	prefix := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " \t"))
		if prefix < 0 || n < prefix {
			prefix = n
		}
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if len(l) >= prefix && prefix > 0 {
			l = l[prefix:]
		}
		out[i] = strings.TrimRight(l, " \t")
	}
	return strings.Join(out, "\n")
}

// collapse joins wrapped lines into paragraphs, keeping blank lines between them.
func collapse(lines []string) []string {
	var out []string
	var para []string
	flush := func() {
		if len(para) > 0 {
			out = append(out, strings.Join(para, " "))
			para = nil
		}
	}
	for _, l := range lines {
		if l == "" {
			flush()
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			continue
		}
		para = append(para, l)
	}
	flush()
	return out
}
