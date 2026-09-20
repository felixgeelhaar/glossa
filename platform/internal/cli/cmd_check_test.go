package cli

import (
	"testing"
)

// checkDoc is `glossa check --json`, as far as these tests read it.
type checkDoc struct {
	Passed bool `json:"passed"`
	Policy struct {
		RequireComplete     []string `json:"require_complete"`
		FailOn              string   `json:"fail_on"`
		MissingTranslations string   `json:"missing_translations"`
		Source              string   `json:"source"`
	} `json:"policy"`
	Findings []struct {
		Code, Locale, Key, Severity string
	} `json:"findings"`
}

// pushed is a project on the fake server with one message and a locale
// with nothing in it: the default policy fails on that, so what the
// command exits with is exactly what the policy decided.
func pushed(t *testing.T) (*fakeServer, *workspace) {
	t.Helper()
	srv := newFakeServer(t)
	w := newWorkspace(t).withProject(srv, map[string]string{"en": `{"home.title": "Welcome"}`})
	w.json(&pushJSON{}, "push").want(t, ExitOK)
	srv.mu.Lock()
	srv.locales = []string{"en", "de"}
	srv.mu.Unlock()
	return srv, w
}

// TestCheckFollowsTheProjectPolicy: the exit code is the project's
// call, not the command's, whenever the command can reach the project.
func TestCheckFollowsTheProjectPolicy(t *testing.T) {
	tests := []struct {
		name   string
		policy map[string]any
		want   ExitCode
		source string
	}{
		{
			name: "no stored policy: the command's own default",
			want: ExitCheckFailed, source: "local",
		},
		{
			name:   "the stored default",
			policy: map[string]any{"require_complete": "all", "fail_on": "error", "missing_translations": "error"},
			want:   ExitCheckFailed, source: "project",
		},
		{
			name:   "untranslated keys only warn",
			policy: map[string]any{"require_complete": "all", "fail_on": "error", "missing_translations": "warning"},
			want:   ExitOK, source: "project",
		},
		{
			name:   "no locale has to be complete",
			policy: map[string]any{"require_complete": "none", "fail_on": "error", "missing_translations": "error"},
			want:   ExitOK, source: "project",
		},
		{
			name: "de is not among the required locales",
			policy: map[string]any{"require_complete": "listed", "locales": []string{"en"},
				"fail_on": "error", "missing_translations": "error"},
			want: ExitOK, source: "project",
		},
		{
			name:   "nothing fails the check",
			policy: map[string]any{"require_complete": "all", "fail_on": "never", "missing_translations": "error"},
			want:   ExitOK, source: "project",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, w := pushed(t)
			srv.checkPolicy = tc.policy
			var doc checkDoc
			w.json(&doc, "check").want(t, tc.want)
			if doc.Policy.Source != tc.source {
				t.Errorf("policy source = %q, want %q", doc.Policy.Source, tc.source)
			}
			if doc.Passed != (tc.want == ExitOK) {
				t.Errorf("passed = %v with exit %d", doc.Passed, tc.want)
			}
			// Green or not, the gap is still reported.
			var missing int
			for _, f := range doc.Findings {
				if f.Code == "missing-translation" && f.Locale == "de" {
					missing++
				}
			}
			if missing != 1 {
				t.Errorf("missing-translation findings for de = %d, want 1: %+v", missing, doc.Findings)
			}
		})
	}
}

// TestCheckFlagsBeatTheProjectPolicy pins the precedence the README
// documents: flags > glossa.yaml > the project's policy > the default.
func TestCheckFlagsBeatTheProjectPolicy(t *testing.T) {
	srv, w := pushed(t)
	srv.checkPolicy = map[string]any{"require_complete": "none", "fail_on": "error", "missing_translations": "error"}

	// The project says no locale has to be complete, so it passes.
	var doc checkDoc
	w.json(&doc, "check").want(t, ExitOK)

	// A flag overrules it.
	w.json(&doc, "check", "--require-complete=de").want(t, ExitCheckFailed)
	if len(doc.Policy.RequireComplete) != 1 || doc.Policy.RequireComplete[0] != "de" {
		t.Errorf("require_complete = %v, want [de]", doc.Policy.RequireComplete)
	}

	// glossa.yaml overrules the project too, and a flag overrules that.
	w.write("glossa.yaml", w.read("glossa.yaml")+"check:\n  require_complete: [de]\n")
	w.json(&doc, "check").want(t, ExitCheckFailed)
	w.json(&doc, "check", "--require-complete=none").want(t, ExitOK)
	if doc.Policy.RequireComplete == nil || len(doc.Policy.RequireComplete) != 0 {
		t.Errorf("require_complete = %v, want none", doc.Policy.RequireComplete)
	}
	// missing_translations has no flag and no glossa.yaml key, so the
	// project keeps that one whatever the local config says.
	if doc.Policy.MissingTranslations != "error" {
		t.Errorf("missing_translations = %q, want the project's", doc.Policy.MissingTranslations)
	}
}

// TestCheckOfflineUsesTheLocalPolicy: with no server to ask, the
// command falls back to glossa.yaml and its own default.
func TestCheckOfflineUsesTheLocalPolicy(t *testing.T) {
	w := newWorkspace(t).withProject(nil, map[string]string{
		"en": `{"home.title": "Welcome"}`,
		"de": `{}`,
	})
	var doc checkDoc
	w.json(&doc, "check", "--offline").want(t, ExitCheckFailed)
	if doc.Policy.Source != "local" || doc.Policy.FailOn != "error" {
		t.Errorf("policy = %+v", doc.Policy)
	}
	w.json(&doc, "check", "--offline", "--fail-on=never").want(t, ExitOK)
	if doc.Policy.FailOn != "never" {
		t.Errorf("fail_on = %q, want never", doc.Policy.FailOn)
	}
}

func TestCheckRejectsAFailOnThatIsNotOne(t *testing.T) {
	w := newWorkspace(t).withProject(nil, map[string]string{"en": `{"home.title": "Welcome"}`})
	var doc errorDoc
	w.json(&doc, "check", "--offline", "--fail-on=fatal").want(t, ExitUsage)
	if doc.Error.Code != "invalid_usage" {
		t.Errorf("error = %+v", doc.Error)
	}
}
