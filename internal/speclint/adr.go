package speclint

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var relatedEntryPattern = regexp.MustCompile(`^-\s+\[([^\]#]+)(?:#([^\]]*))?\]\s*(.*)$`)

// checkADRs validates the Related section of each docs/adr/*.md, which links decisions to intents
// as [features/x.md#<id>], or to a whole intent file as [features/x.md]. The text after the link
// is a label for people and isn't checked.
func (l *linter) checkADRs(root string) error {
	paths, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(l.cfg.ADRDir), "*.md"))
	if err != nil {
		return err
	}
	for _, p := range paths {
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		l.checkADR(root, l.cfg.ADRDir+"/"+filepath.Base(p), string(src))
	}
	return nil
}

func (l *linter) checkADR(root, file, src string) {
	inRelated, sawRelated, inFence := false, false, false
	relatedLine := 0
	var universal, featureRefs int

	for i, raw := range strings.Split(src, "\n") {
		line := i + 1
		text := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))

		if strings.HasPrefix(text, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.HasPrefix(text, "## ") {
			inRelated = text == "## Related"
			if inRelated {
				sawRelated, relatedLine = true, line
			}
			continue
		}
		if !inRelated || !strings.HasPrefix(text, "-") {
			continue
		}

		if text == "- universal" {
			universal++
			continue
		}
		m := relatedEntryPattern.FindStringSubmatch(text)
		if m == nil {
			l.report(file, line, "Related entry should be \"- universal\" or \"- [<path>] <label>\"")
			continue
		}
		target, id := m[1], m[2]

		switch {
		case strings.HasPrefix(target, l.cfg.FeaturesDir+"/"):
			featureRefs++
			l.checkADRFeatureRef(file, line, target, id)
		case strings.HasPrefix(target, l.cfg.ADRDir+"/"):
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(target))); err != nil {
				l.report(file, line, "[%s] doesn't exist", target)
			}
		default:
			l.report(file, line, "[%s]: Related entries link to %s/ or %s/", target, l.cfg.FeaturesDir, l.cfg.ADRDir)
		}
	}

	switch {
	case !sawRelated:
		l.report(file, 1, "ADR has no ## Related section")
	case universal == 0 && featureRefs == 0:
		l.report(file, relatedLine, "Related lists no intents and isn't universal")
	case universal > 0 && featureRefs > 0:
		l.report(file, relatedLine, "Related mixes universal with intent references; use one or the other")
	}
}

func (l *linter) checkADRFeatureRef(file string, line int, target, id string) {
	ref := target
	if id != "" {
		ref += "#" + id
	}
	f, ok := l.files[target]
	if !ok {
		l.report(file, line, "[%s]: %s doesn't exist", ref, target)
		return
	}
	if id == "" {
		return
	}
	in := f.intent(id)
	switch {
	case in == nil:
		l.report(file, line, "[%s]: %s has no intent with @id:%s", ref, target, id)
	case in.Status == StatusWithdrawn:
		l.report(file, line, "[%s]: intent is withdrawn; don't list it in Related", ref)
	}
}
