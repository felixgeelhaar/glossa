//go:build integration

package cli_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixgeelhaar/glossa/platform/internal/cli"
)

// contextLoop is M3's usage upload against the real server (RFC 0004
// §6.3): the bundler plugin's document through `glossa context push`,
// then `glossa extract --upload`, both answered by the Context API; the
// usages are then readable where a translator looks for them.
func contextLoop(t *testing.T, r runner, s *server, owner session, p string) {
	s.do(owner.call("POST", p+"/applications", map[string]any{"slug": "web", "name": "Web", "platform": "web"}), http.StatusCreated, nil)
	commit := strings.Repeat("c0ffee12", 5)
	doc := `{"schema":"glossa.usages/v1","application":"web","commit":"` + commit + `","branch":"main",
	  "tool":{"name":"@glossa/unplugin","version":"0.1.0"},
	  "usages":[
	    {"key":"checkout.pay","file":"src/Checkout.vue","line":12,"column":7,"component":"Checkout","route":"/checkout","kind":"t"},
	    {"key":"gone.key","file":"src/Checkout.vue","line":20,"column":7,"component":"Checkout","route":"/checkout","kind":"t"}]}`
	if err := os.MkdirAll(filepath.Join(r.dir, ".glossa"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.dir, ".glossa", "usages.json"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	var pushed struct {
		Source   string `json:"source"`
		Replayed bool   `json:"replayed"`
		Build    struct {
			ID              string `json:"id"`
			Usages          int    `json:"usages"`
			UnknownKeys     int    `json:"unknown_keys"`
			OnDefaultBranch bool   `json:"on_default_branch"`
		} `json:"build"`
	}
	r.run(cli.ExitOK, &pushed, "context", "push", ".glossa/usages.json")
	if pushed.Source != "plugin" || pushed.Replayed || pushed.Build.Usages != 2 || pushed.Build.UnknownKeys != 1 || !pushed.Build.OnDefaultBranch {
		t.Fatalf("context push = %+v", pushed)
	}
	first := pushed.Build.ID
	r.run(cli.ExitOK, &pushed, "context", "push", ".glossa/usages.json")
	if !pushed.Replayed || pushed.Build.ID != first {
		t.Errorf("second push = %+v", pushed)
	}

	// extract finds nothing here (the generated accessors are skipped),
	// and still records an extract build of the commit.
	r.run(cli.ExitOK, nil, "extract", "--upload", "--application", "web", "--commit", commit, "--branch", "main")

	var usages struct {
		Usages []struct {
			File, Component, Route string
		} `json:"usages"`
	}
	s.do(owner.call("GET", p+"/messages/checkout.pay/usages", nil), http.StatusOK, &usages)
	if len(usages.Usages) != 1 || usages.Usages[0].File != "src/Checkout.vue" || usages.Usages[0].Route != "/checkout" {
		t.Errorf("usages of checkout.pay = %+v", usages)
	}
	var builds struct {
		Items []struct{ Source string } `json:"items"`
	}
	s.do(owner.call("GET", p+"/context-builds", nil), http.StatusOK, &builds)
	if len(builds.Items) != 2 || builds.Items[0].Source != "extract" || builds.Items[1].Source != "plugin" {
		t.Errorf("builds = %+v", builds)
	}
}
