package domain_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/felixgeelhaar/glossa/platform/internal/context/domain"
)

// The server validates an upload by the glossa.usages/v1 schema's rules
// (runtimes/testdata/schemas/usages.v1.schema.json). These tests hold
// ParseUpload to the schema itself: over the example, every fixture's
// expected document and a list of variants, ParseUpload accepts exactly
// what the schema accepts — apart from the deviations listed in
// deviations, each with its reason.

// testdata is runtimes/testdata, found from this file's location.
func testdata(t *testing.T, parts ...string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(append([]string{filepath.Dir(file), "..", "..", "..", "..", "runtimes", "testdata"}, parts...)...)
}

func usagesSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	f, err := os.Open(testdata(t, "schemas", "usages.v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	doc, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err := c.AddResource("usages.v1.json", doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("usages.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func schemaAccepts(t *testing.T, s *jsonschema.Schema, doc []byte) bool {
	t.Helper()
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(doc))
	if err != nil {
		return false
	}
	return s.Validate(inst) == nil
}

// variant replaces the value at path (object members and array indexes)
// in a copy of doc; a nil value deletes it.
type variant struct {
	why   string
	path  []any
	value any
}

var u0 = []any{"usages", 0}

func at(path []any, more ...any) []any { return append(append([]any{}, path...), more...) }

var deleted = struct{}{}

// variants are the Python checker's (runtimes/testdata/gen/check_usages.py)
// plus the edges of every length and pattern.
var variants = []variant{
	{"SHA-256 commit", []any{"commit"}, strings.Repeat("a", 64)},
	{"dotfile directory and '...' segment", at(u0, "file"), ".storybook/.../preview.ts"},
	{"scoped tool name and prerelease version", []any{"tool"}, map[string]any{"name": "@glossa/unplugin", "version": "0.1.0-rc.1+build.5"}},
	{"nested branch name", []any{"branch"}, "renovate/vite-6.x"},
	{"short commit", []any{"commit"}, "9f2c1e7"},
	{"41-digit commit", []any{"commit"}, strings.Repeat("a", 41)},
	{"uppercase commit", []any{"commit"}, strings.Repeat("A", 40)},
	{"branch with '..'", []any{"branch"}, "feat/../main"},
	{"branch component starting with '.'", []any{"branch"}, "feat/.hidden"},
	{"branch with a space", []any{"branch"}, "my branch"},
	{"branch ending in '.'", []any{"branch"}, "release."},
	{"branch with a brace", []any{"branch"}, "feat/{x}"},
	{"branch with a tilde", []any{"branch"}, "a~1"},
	{"empty branch", []any{"branch"}, ""},
	{"256-byte branch", []any{"branch"}, strings.Repeat("b", 256)},
	{"application that isn't a slug", []any{"application"}, "Web App"},
	{"application with a trailing hyphen", []any{"application"}, "web-"},
	{"unknown schema version", []any{"schema"}, "glossa.usages/v2"},
	{"missing tool version", []any{"tool"}, map[string]any{"name": "glossa"}},
	{"tool name with a space", []any{"tool", "name"}, "glossa extract"},
	{"tool version that isn't semver", []any{"tool", "version"}, "v1"},
	{"missing usages", []any{"usages"}, deleted},
	{"absolute path", at(u0, "file"), "/src/App.vue"},
	{"parent segment", at(u0, "file"), "src/../App.vue"},
	{"leading parent segment", at(u0, "file"), "../App.vue"},
	{"current-directory segment", at(u0, "file"), "./src/App.vue"},
	{"backslash separators", at(u0, "file"), "src\\App.vue"},
	{"drive letter", at(u0, "file"), "C:/src/App.vue"},
	{"empty segment", at(u0, "file"), "src//App.vue"},
	{"trailing slash", at(u0, "file"), "src/"},
	{"non-ASCII file", at(u0, "file"), "src/Größe.vue"},
	{"line 0", at(u0, "line"), 0},
	{"column 0", at(u0, "column"), 0},
	{"missing column", at(u0, "column"), deleted},
	{"fractional line", at(u0, "line"), 1.5},
	{"kind outside the enum", at(u0, "kind"), "go"},
	{"typed is now accessor", at(u0, "kind"), "typed"},
	{"key that isn't a message key", at(u0, "key"), "Checkout Pay"},
	{"key of 200 characters", at(u0, "key"), strings.Repeat("a", 200)},
	{"key over 200 characters", at(u0, "key"), strings.Repeat("a", 201)},
	{"route that is a URL", at(u0, "route"), "https://shop.example.com/checkout"},
	{"route with a query", at(u0, "route"), "/checkout?step=2"},
	{"empty route", at(u0, "route"), ""},
	{"route of 1024 characters", at(u0, "route"), "/" + strings.Repeat("r", 1023)},
	{"route over 1024 characters", at(u0, "route"), "/" + strings.Repeat("r", 1024)},
	{"empty component", at(u0, "component"), ""},
	{"component with spaces", at(u0, "component"), "PaymentFooter > PrimaryButton"},
	{"Go method component", at(u0, "component"), "mail.(*Mailer).Send"},
	{"component of 512 characters", at(u0, "component"), strings.Repeat("C", 512)},
	{"component over 512 characters", at(u0, "component"), strings.Repeat("C", 513)},
	{"no component", at(u0, "component"), deleted},
	{"no route", at(u0, "route"), deleted},
	{"branch '@'", []any{"branch"}, "@"},
	{"branch component ending in .lock", []any{"branch"}, "feat/x.lock/y"},
	{"branch starting with '-'", []any{"branch"}, "-x"},
	{"component with a no-break space", at(u0, "component"), "Payment Footer"},
	{"extra top-level field", []any{"digest"}, "abc"},
	{"extra usage field", at(u0, "messageId"), "msg_1"},
}

// deviations are the variants where ParseUpload deliberately differs
// from the schema, and why.
var deviations = map[string]string{
	// Unknown fields are ignored within v1 so a newer collector's
	// optional field doesn't break an older server; the schema is closed
	// so collectors can't drift.
	"extra top-level field": "ignored",
	"extra usage field":     "ignored",
	// Git refuses these branch names (git-check-ref-format); no
	// repository has one.
	"branch '@'":                       "refused",
	"branch component ending in .lock": "refused",
	"branch starting with '-'":         "refused",
	// The schema's \S is ECMA-262's, which excludes Unicode white space;
	// Go's regexp (the validator in this test) is ASCII only.
	"component with a no-break space": "refused",
}

func mutate(t *testing.T, doc []byte, v variant) []byte {
	t.Helper()
	var root any
	if err := json.Unmarshal(doc, &root); err != nil {
		t.Fatal(err)
	}
	parent := root
	for _, step := range v.path[:len(v.path)-1] {
		switch s := step.(type) {
		case string:
			parent = parent.(map[string]any)[s]
		case int:
			parent = parent.([]any)[s]
		}
	}
	last := v.path[len(v.path)-1]
	switch p := parent.(type) {
	case map[string]any:
		if v.value == deleted {
			delete(p, last.(string))
		} else {
			p[last.(string)] = v.value
		}
	case []any:
		p[last.(int)] = v.value
	}
	out, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestParseUploadAgreesWithTheSchemaOnVariants(t *testing.T) {
	s := usagesSchema(t)
	example, err := os.ReadFile(testdata(t, "schemas", "examples", "usages.v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !schemaAccepts(t, s, example) {
		t.Fatal("the schema rejects its own example")
	}
	if _, err := domain.ParseUpload(example); err != nil {
		t.Fatalf("ParseUpload rejects the schema's example: %v", err)
	}
	for _, v := range variants {
		t.Run(v.why, func(t *testing.T) {
			doc := mutate(t, example, v)
			want := schemaAccepts(t, s, doc)
			switch deviations[v.why] {
			case "ignored":
				want = true
			case "refused":
				want = false
			}
			_, err := domain.ParseUpload(doc)
			if got := err == nil; got != want {
				t.Errorf("ParseUpload accepted = %t (%v), schema accepted = %t\n%s", got, err, want, doc)
			}
		})
	}
}

func TestParseUploadAcceptsEveryFixtureDocument(t *testing.T) {
	s := usagesSchema(t)
	cases, err := filepath.Glob(testdata(t, "usages", "*", "expected.json"))
	if err != nil || len(cases) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, path := range cases {
		t.Run(filepath.Base(filepath.Dir(path)), func(t *testing.T) {
			doc, err := os.ReadFile(path) //nolint:gosec // fixtures in the repository
			if err != nil {
				t.Fatal(err)
			}
			if !schemaAccepts(t, s, doc) {
				t.Fatal("the schema rejects the fixture")
			}
			if _, err := domain.ParseUpload(doc); err != nil {
				t.Errorf("ParseUpload: %v", err)
			}
		})
	}
}
