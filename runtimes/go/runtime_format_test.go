package glossa

// messageformat/testdata/glossa/runtime-format.json through the runtime's
// rendering path (runtimes/SPEC.md §5): each precompiled message is
// shipped in a release artifact, resolved and formatted by Localizer.T.

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

const runtimeFormatFixture = "../../messageformat/testdata/glossa/runtime-format.json"

// runtimeFormatSkips lists cases whose expected output the Go formatter
// (messageformat.Format) doesn't reproduce because of CLDR data gaps in
// its engine, keyed by "<description> <params>", with the reason. Keep it
// short; the stale-entry check keeps it honest.
var runtimeFormatSkips = map[string]string{
	// The same gap messageformat's glossaFormatSkips records for its MF1
	// fixture: go-intl v0.2.17 drops the no-break space of the German
	// percent pattern "#,##0 %".
	"de percent [p=0.256]": "go-intl: German percent lacks the CLDR no-break space (26% vs 26 %)",
}

type runtimeFormatCase struct {
	Description   string          `json:"description"`
	Locale        string          `json:"locale"`
	Message       json.RawMessage `json:"message"`
	Params        []formatParam   `json:"params"`
	BidiIsolation string          `json:"bidiIsolation"`
	Exp           string          `json:"exp"`
	ExpErrors     []struct {
		Type string `json:"type"`
	} `json:"expErrors"`
}

type formatParam struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value any    `json:"value"`
}

func (tc runtimeFormatCase) key() string {
	parts := make([]string, len(tc.Params))
	for i, p := range tc.Params {
		parts[i] = fmt.Sprintf("%s=%v", p.Name, p.Value)
	}
	return tc.Description + " [" + strings.Join(parts, " ") + "]"
}

func (tc runtimeFormatCase) args(t *testing.T) Args {
	args := Args{}
	for _, p := range tc.Params {
		args[p.Name] = p.Value
		if p.Type == "datetime" {
			ts, err := time.Parse(time.RFC3339, p.Value.(string))
			if err != nil {
				t.Fatal(err)
			}
			args[p.Name] = ts
		}
	}
	return args
}

func TestRuntimeFormatFixture(t *testing.T) {
	b, err := os.ReadFile(runtimeFormatFixture)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Tests []runtimeFormatCase `json:"tests"`
	}
	if err := json.Unmarshal(b, &fixture); err != nil {
		t.Fatal(err)
	}
	used, passed := map[string]bool{}, 0
	for _, tc := range fixture.Tests {
		if reason, ok := runtimeFormatSkips[tc.key()]; ok {
			used[tc.key()] = true
			t.Logf("skip %s: %s", tc.key(), reason)
			continue
		}
		if checkRuntimeFormat(t, tc) {
			passed++
		}
	}
	for key := range runtimeFormatSkips {
		if !used[key] {
			t.Errorf("stale runtimeFormatSkips entry %q", key)
		}
	}
	t.Logf("runtime-format.json: %d passed, %d skipped", passed, len(runtimeFormatSkips))
}

func checkRuntimeFormat(t *testing.T, tc runtimeFormatCase) bool {
	t.Helper()
	rel := buildReleaseModels(t, "rel_fmt", 1, map[string]map[string]any{tc.Locale: {"m": tc.Message}})
	log := &errorLog{}
	c := newTestClient(t, Config{Bundled: rel.fs(), OnError: log.handle})
	got := c.For(tc.Locale).T("m", tc.args(t), BidiIsolation(tc.BidiIsolation != "none"))
	ok := true
	if got != tc.Exp {
		t.Errorf("%s: got %q, want %q", tc.key(), got, tc.Exp)
		ok = false
	}
	errs := log.all()
	if len(errs) != min(len(tc.ExpErrors), 1) {
		t.Errorf("%s: reported %v, want the format errors %v", tc.key(), errs, tc.ExpErrors)
		return false
	}
	for _, want := range tc.ExpErrors {
		if errs[0].Type != ErrorFormat || !strings.Contains(errs[0].Detail, want.Type) {
			t.Errorf("%s: reported %+v, want a format error with %s", tc.key(), errs[0], want.Type)
			ok = false
		}
	}
	return ok
}
