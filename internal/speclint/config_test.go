package speclint

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeRepo writes files, by repo-relative path, into a new temporary directory and returns it.
func writeRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, src := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestDefaultLayers(t *testing.T) {
	c := DefaultConfig()
	for file, want := range map[string]string{
		// A file a layer matches.
		"internal/db/queries/jobs.sql": "SQL query",
		// Files no layer matches are named after their top-level directory.
		"internal/db/store_test.go":              "internal",
		"frontend/src/components/Board.test.tsx": "frontend",
		"app.go":                                 "root",
	} {
		if got, _ := c.layerOf(file); got != want {
			t.Errorf("layerOf(%s) = %q, want %q", file, got, want)
		}
	}
}

func TestNoConfigFileUsesDefaults(t *testing.T) {
	c, err := LoadConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c, DefaultConfig()) {
		t.Errorf("got %+v, want the defaults", c)
	}
}

func TestConfigFileAdjustsDefaults(t *testing.T) {
	root := writeRepo(t, map[string]string{ConfigFile: `{
		"featuresDir": "specs/",
		"adrDir": "docs/decisions",
		"languages": {".nix": "hash", ".svelte": ""},
		"skipDirs": ["web/generated"],
		"testPatterns": ["^e2e/"],
		"layers": [
			{"name": "storage", "match": "^internal/db/"},
			{"name": "backend", "match": "^internal/"},
			{"name": "frontend", "match": "^web/"}
		]
	}`})
	c, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}

	if c.FeaturesDir != "specs" || c.ADRDir != "docs/decisions" {
		t.Errorf("dirs: got %q and %q", c.FeaturesDir, c.ADRDir)
	}
	if _, ok := c.languages[".nix"]; !ok {
		t.Error(".nix isn't read")
	}
	if _, ok := c.languages[".svelte"]; ok {
		t.Error(".svelte is still read")
	}
	if _, ok := c.languages[".go"]; !ok {
		t.Error(".go, a default, isn't read")
	}
	if !c.skipDir("generated", "web/generated") || c.skipDir("generated", "api/generated") || !c.skipDir("node_modules", "web/node_modules") {
		t.Error("skipDirs: web/generated should be skipped by path, node_modules still by name")
	}
	if !c.isTestFile("e2e/board.ts") || !c.isTestFile("app_test.go") || c.isTestFile("web/board.ts") {
		t.Error("testPatterns: e2e/ and the default conventions should be tests")
	}

	for file, want := range map[string]string{
		"internal/db/store.go": "storage",
		"internal/jobs/run.go": "backend",
		"web/src/Board.tsx":    "frontend",
		"queries/jobs.sql":     "queries", // the configured layers replace the SQL default
	} {
		if got, _ := c.layerOf(file); got != want {
			t.Errorf("layerOf(%s) = %q, want %q", file, got, want)
		}
	}
}

func TestConfigFileErrors(t *testing.T) {
	for _, tt := range []struct{ src, want string }{
		{`{"featuresDirs": "specs"}`, "unknown field"},
		{`{"featuresDir": "../specs"}`, "inside the repository"},
		{`{"languages": {"nix": "hash"}}`, "starting with a dot"},
		{`{"languages": {".nix": "semicolon"}}`, "unknown comment style"},
		{`{"testPatterns": ["("]}`, "testPatterns"},
		{`{"layers": [{"name": "storage"}]}`, "needs a name and a match"},
		{`{"layers": [{"name": "storage", "match": "("}]}`, "layers: storage"},
		{`{"limits": {"intentsPerFile": -1}}`, "can't be negative"},
		{`{"limits": {"intentsPerScenario": 3}}`, "unknown field"},
	} {
		root := writeRepo(t, map[string]string{ConfigFile: tt.src})
		_, err := LoadConfig(root)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: got error %v, want one containing %q", tt.src, err, tt.want)
		}
	}
}

func TestConfigFileSetsLimits(t *testing.T) {
	root := writeRepo(t, map[string]string{ConfigFile: `{"limits": {"intentsPerFile": 5, "statementWords": 0}, "strictLimits": true}`})
	c, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	// Limits that aren't set keep their defaults; 0 turns one off.
	want := Limits{IntentsPerFile: 5, IntentsPerEntryPoint: DefaultLimits.IntentsPerEntryPoint, StatementWords: 0}
	if c.Limits != want || !c.StrictLimits {
		t.Errorf("got %+v strict=%v, want %+v strict=true", c.Limits, c.StrictLimits, want)
	}
}

// The configured features directory, languages, and test patterns are what Lint and the traces
// use.
func TestLintAndTraceUseTheConfig(t *testing.T) {
	root := writeRepo(t, map[string]string{
		ConfigFile: `{"featuresDir": "specs", "languages": {".nix": "hash"}, "testPatterns": ["^checks/"]}`,
		"specs/build.md": `# Build

Builds the project.

## Must

- @id:reproducible @status:implemented
  Building the same source twice gives the same output.
`,
		"flake.nix":        "# @intent [#reproducible]\n{ }\n",
		"checks/build.nix": "# @intent [#reproducible]\n",
	})

	problems, err := Lint(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 0 {
		t.Errorf("expected no problems, got %v", problems)
	}

	tr, err := TraceIntent(root, "reproducible")
	if err != nil {
		t.Fatal(err)
	}
	if tr.File.Path != "specs/build.md" {
		t.Errorf("traced %s", tr.File.Path)
	}
	var where []string
	for _, a := range tr.Links {
		where = append(where, a.File+" "+a.Where())
	}
	if want := []string{"checks/build.nix checks test", "flake.nix root"}; !reflect.DeepEqual(where, want) {
		t.Errorf("links: got %v, want %v", where, want)
	}
}
