package speclint

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ConfigFile is the optional settings file, read from the repository root. Everything in it
// adjusts the defaults below; a project that's happy with them doesn't need one. See README.md.
const ConfigFile = ".speclint.json"

// Config is what speclint needs to know about a project: where the specs and ADRs are, which
// files to read and how they write comments, which files are tests, and the layers spectrace
// groups links by.
type Config struct {
	FeaturesDir string // repo-relative, forward slashes
	ADRDir      string // repo-relative, forward slashes

	// Limits keep intents high-level. Going over one is a warning, or an error with StrictLimits.
	Limits       Limits
	StrictLimits bool

	languages    map[string]commentStyle // by file extension
	skipDirs     map[string]bool
	testPatterns []*regexp.Regexp // tests beyond the default conventions in isDefaultTestFile
	layers       []layer
}

// Limits caps how much an intent file or entry point says. 0 turns a limit off.
type Limits struct {
	// IntentsPerFile caps the active (not-implemented or implemented) intents in one file.
	IntentsPerFile int
	// IntentsPerEntryPoint caps the intents one @intent line in code names. Tests aren't counted,
	// since one test may check several intents.
	IntentsPerEntryPoint int
	// StatementWords caps the words in an intent's statement. Notes aren't counted.
	StatementWords int
}

// DefaultLimits are the limits used when .speclint.json doesn't set them.
var DefaultLimits = Limits{IntentsPerFile: 8, IntentsPerEntryPoint: 4, StatementWords: 40}

type layer struct {
	name  string
	match *regexp.Regexp
}

// fileConfig is the JSON form of .speclint.json.
type fileConfig struct {
	// FeaturesDir and ADRDir replace the defaults, "features" and "docs/adr".
	FeaturesDir string `json:"featuresDir"`
	ADRDir      string `json:"adrDir"`
	// Limits replaces any of the default limits; 0 turns one off. StrictLimits makes going over a
	// limit an error rather than a warning.
	Limits *struct {
		IntentsPerFile       *int `json:"intentsPerFile"`
		IntentsPerEntryPoint *int `json:"intentsPerEntryPoint"`
		StatementWords       *int `json:"statementWords"`
	} `json:"limits"`
	StrictLimits bool `json:"strictLimits"`
	// Languages maps file extensions to a comment style: "c", "hash", "dash", "c-hash", or "html".
	// It adds to the defaults, or overrides one. A style of "" stops an extension being read.
	Languages map[string]string `json:"languages"`
	// SkipDirs adds directories that are never read. A name matches a directory with that name
	// anywhere; a path with a slash matches that repo-relative path only.
	SkipDirs []string `json:"skipDirs"`
	// TestPatterns adds regular expressions, matched against repo-relative paths, for files that
	// are tests but don't follow the default naming conventions.
	TestPatterns []string `json:"testPatterns"`
	// Layers replaces the default layers. List them bottom of the stack first; match is a regular
	// expression on the repo-relative path, and the first layer that matches a file wins.
	Layers []struct {
		Name  string `json:"name"`
		Match string `json:"match"`
	} `json:"layers"`
}

// commentStyle says how annotations are written in one family of languages.
type commentStyle struct {
	// prefix matches what may precede an annotation on its line: indentation, then the comment
	// opener.
	prefix *regexp.Regexp
	// line matches a line that is only a comment, so comment lines between two @scenario lines
	// keep them in the same comment block.
	line *regexp.Regexp
	// closers are block comment endings trimmed from an annotation that closes on its own line.
	closers []string
}

// styles are the comment styles a language can use, by the name .speclint.json gives them.
var styles = map[string]commentStyle{
	// // and /* */ comments, block comment continuations ( * ), and JSX {/* */}.
	"c": {
		prefix:  regexp.MustCompile(`^\s*(//|/\*|\{/\*|\*)\s*$`),
		line:    regexp.MustCompile(`^\s*(//|/\*|\{/\*|\*)`),
		closers: []string{"*/}", "*/"},
	},
	// # line comments.
	"hash": {
		prefix: regexp.MustCompile(`^\s*#\s*$`),
		line:   regexp.MustCompile(`^\s*#`),
	},
	// -- line comments.
	"dash": {
		prefix: regexp.MustCompile(`^\s*--\s*$`),
		line:   regexp.MustCompile(`^\s*--`),
	},
	// Both // and # line comments, as in PHP.
	"c-hash": {
		prefix:  regexp.MustCompile(`^\s*(//|/\*|\*|#)\s*$`),
		line:    regexp.MustCompile(`^\s*(//|/\*|\*|#)`),
		closers: []string{"*/"},
	},
	// <!-- --> comments, and Thymeleaf's <!--/* */--> comments, which are removed when the template
	// renders.
	"html": {
		prefix:  regexp.MustCompile(`^\s*<!--(/\*)?\s*$`),
		line:    regexp.MustCompile(`^\s*<!--`),
		closers: []string{"*/-->", "-->"},
	},
}

// defaultLanguages maps the extensions read by default to their comment style.
var defaultLanguages = map[string]string{
	// C-family and JavaScript-family
	".go": "c", ".rs": "c", ".c": "c", ".h": "c", ".cc": "c", ".cpp": "c", ".hpp": "c", ".cs": "c",
	".fs": "c", ".java": "c", ".kt": "c", ".kts": "c", ".scala": "c", ".swift": "c", ".dart": "c",
	".zig": "c", ".js": "c", ".jsx": "c", ".mjs": "c", ".cjs": "c", ".ts": "c", ".tsx": "c",
	".mts": "c", ".cts": "c", ".vue": "c", ".svelte": "c",
	// # comments
	".py": "hash", ".rb": "hash", ".ex": "hash", ".exs": "hash", ".sh": "hash", ".bash": "hash",
	".ps1": "hash", ".r": "hash", ".jl": "hash", ".nim": "hash",
	// -- comments
	".sql": "dash", ".lua": "dash", ".hs": "dash", ".elm": "dash",
	// both
	".php": "c-hash",
	// <!-- --> comments
	".html": "html", ".htm": "html",
}

// defaultSkipDirs are never read: dependencies, build output, and test fixtures (which may
// contain deliberately broken annotations). Dot-directories are always skipped too.
var defaultSkipDirs = []string{
	"node_modules", "vendor", "dist", "build", "out", "target", "bin", "obj", "coverage",
	"testdata", "venv", "__pycache__",
}

// defaultLayers lists SQL first, since a query is usually the bottom of the stack. Files no layer
// matches are grouped by their top-level directory.
var defaultLayers = []layer{
	{"SQL query", regexp.MustCompile(`\.sql$`)},
}

// DefaultConfig is the configuration used when the repository has no .speclint.json.
func DefaultConfig() *Config {
	c := &Config{
		FeaturesDir: "features",
		ADRDir:      "docs/adr",
		Limits:      DefaultLimits,
		languages:   map[string]commentStyle{},
		skipDirs:    map[string]bool{},
		layers:      defaultLayers,
	}
	for ext, style := range defaultLanguages {
		c.languages[ext] = styles[style]
	}
	for _, d := range defaultSkipDirs {
		c.skipDirs[d] = true
	}
	return c
}

// LoadConfig reads root/.speclint.json on top of the defaults, or returns the defaults when there
// isn't one.
func LoadConfig(root string) (*Config, error) {
	c := DefaultConfig()
	src, err := os.ReadFile(filepath.Join(root, ConfigFile))
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if err := c.apply(src); err != nil {
		return nil, fmt.Errorf("%s: %w", ConfigFile, err)
	}
	return c, nil
}

// apply adjusts the configuration with the JSON from a .speclint.json file.
func (c *Config) apply(src []byte) error {
	var f fileConfig
	dec := json.NewDecoder(bytes.NewReader(src))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return err
	}

	dir := func(field, v string) (string, error) {
		v = path.Clean(strings.Trim(filepath.ToSlash(v), "/"))
		if v == "." || strings.HasPrefix(v, "..") || filepath.IsAbs(v) {
			return "", fmt.Errorf("%s must be a path inside the repository, like \"docs/adr\"", field)
		}
		return v, nil
	}
	var err error
	if f.FeaturesDir != "" {
		if c.FeaturesDir, err = dir("featuresDir", f.FeaturesDir); err != nil {
			return err
		}
	}
	if f.ADRDir != "" {
		if c.ADRDir, err = dir("adrDir", f.ADRDir); err != nil {
			return err
		}
	}

	if f.Limits != nil {
		for _, l := range []struct {
			name string
			from *int
			to   *int
		}{
			{"intentsPerFile", f.Limits.IntentsPerFile, &c.Limits.IntentsPerFile},
			{"intentsPerEntryPoint", f.Limits.IntentsPerEntryPoint, &c.Limits.IntentsPerEntryPoint},
			{"statementWords", f.Limits.StatementWords, &c.Limits.StatementWords},
		} {
			if l.from == nil {
				continue
			}
			if *l.from < 0 {
				return fmt.Errorf("limits: %s can't be negative; use 0 to turn it off", l.name)
			}
			*l.to = *l.from
		}
	}
	c.StrictLimits = f.StrictLimits

	exts := make([]string, 0, len(f.Languages))
	for ext := range f.Languages {
		exts = append(exts, ext)
	}
	sort.Strings(exts)
	for _, ext := range exts {
		name := f.Languages[ext]
		if !strings.HasPrefix(ext, ".") {
			return fmt.Errorf("languages: %q should be a file extension starting with a dot", ext)
		}
		if name == "" {
			delete(c.languages, ext)
			continue
		}
		style, ok := styles[name]
		if !ok {
			return fmt.Errorf("languages: %s has unknown comment style %q; use c, hash, dash, c-hash, or html", ext, name)
		}
		c.languages[ext] = style
	}

	for _, d := range f.SkipDirs {
		c.skipDirs[strings.Trim(filepath.ToSlash(d), "/")] = true
	}

	for _, p := range f.TestPatterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return fmt.Errorf("testPatterns: %w", err)
		}
		c.testPatterns = append(c.testPatterns, re)
	}

	if f.Layers != nil {
		c.layers = nil
		for _, l := range f.Layers {
			if l.Name == "" || l.Match == "" {
				return errors.New("layers: every layer needs a name and a match")
			}
			re, err := regexp.Compile(l.Match)
			if err != nil {
				return fmt.Errorf("layers: %s: %w", l.Name, err)
			}
			c.layers = append(c.layers, layer{l.Name, re})
		}
	}
	return nil
}

// skipDir reports whether a directory, given by name and repo-relative path, is never read.
func (c *Config) skipDir(name, rel string) bool {
	return strings.HasPrefix(name, ".") || c.skipDirs[name] || c.skipDirs[rel]
}

// isTestFile reports whether a repo-relative path is a test, by the default conventions or one of
// the configured test patterns.
func (c *Config) isTestFile(p string) bool {
	if isDefaultTestFile(p) {
		return true
	}
	for _, re := range c.testPatterns {
		if re.MatchString(p) {
			return true
		}
	}
	return false
}

// isDefaultTestFile reports whether a repo-relative path is a test by the usual conventions:
//
//   - Go: *_test.go
//   - JavaScript and TypeScript: *.test.*, *.spec.*, __tests__/
//   - Python: test_*.py, *_test.py
//   - Ruby: *_spec.rb, *_test.rb
//   - Java, Kotlin, C#, and similar: *Test.*, *Tests.*, *Spec.*
//   - anything under a test/, tests/, or spec/ directory
func isDefaultTestFile(p string) bool {
	base := path.Base(p)
	stem := strings.TrimSuffix(base, path.Ext(base))
	dirs := "/" + path.Dir(p) + "/"
	return strings.HasSuffix(stem, "_test") ||
		strings.HasSuffix(stem, "_spec") ||
		strings.HasPrefix(base, "test_") ||
		strings.Contains(base, ".test.") ||
		strings.Contains(base, ".spec.") ||
		strings.HasSuffix(stem, "Test") ||
		strings.HasSuffix(stem, "Tests") ||
		strings.HasSuffix(stem, "Spec") ||
		strings.Contains(dirs, "/__tests__/") ||
		strings.Contains(dirs, "/test/") ||
		strings.Contains(dirs, "/tests/") ||
		strings.Contains(dirs, "/spec/")
}

// layerOf returns the name of the layer a file sits in and its rank, bottom of the stack first. A
// file no layer matches is named after its top-level directory, or "root" for a file at the top,
// and ranks after every named layer.
func (c *Config) layerOf(file string) (string, int) {
	for i, l := range c.layers {
		if l.match.MatchString(file) {
			return l.name, i
		}
	}
	if dir, _, ok := strings.Cut(file, "/"); ok {
		return dir, len(c.layers)
	}
	return "root", len(c.layers)
}
