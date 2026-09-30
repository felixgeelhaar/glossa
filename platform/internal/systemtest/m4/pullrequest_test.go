//go:build system

package m4_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/felixgeelhaar/glossa/platform/internal/integration/adapters/github/githubtest"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
)

// The exit criterion (RFC 0005 §12.3): the same commit, through the
// fake GitHub, produces a check run whose conclusion, error count and
// per-layer counts equal the CLI's. Two surfaces, one verdict.
//
// Wave 4 proved this property at the unit level
// (integration/app/check_agreement_test.go). This is the same property
// end to end: the numbers on the left come out of `glossa check`
// against a real server, and the numbers on the right out of a check
// run a real webhook produced, read back off GitHub's own view of it.
//
// A note on how the check run is produced. Nothing creates a check run
// through `/v1`: `openCheck` is reached only from webhook processing
// (integration/app/github_webhook.go), which writes a queue row that a
// worker turns into a GitHub call. This test therefore does what a
// product does — it delivers a signed `pull_request.opened` — and
// assumes no API shortcut. Should the wave-7 slice that adds one land,
// the shortcut would be an additional way in, not a replacement: the
// property under test is about the check run, not about what triggered
// it.

// checkRunView is a Glossa check on the fake GitHub.
type checkRunView struct {
	Name, HeadSHA, Status, Conclusion, Title string
	Summary                                  string
	Annotations                              int
	Patches                                  int
}

// connectRepository runs the GitHub App's install flow and connects the
// repository to the project.
func (s *scenario) connectRepository() {
	t := s.t
	var intent struct {
		State      string `json:"state"`
		InstallURL string `json:"install_url"`
	}
	s.owner.do(http.MethodPost, s.tenantPath("/github/install-intents"), map[string]any{}, http.StatusCreated, &intent)
	var installation struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}
	s.owner.do(http.MethodPost, s.tenantPath("/github/installations"), map[string]any{
		"state": intent.State, "code": "code-m4", "installation_id": installationID,
	}, http.StatusCreated, &installation)
	if installation.State != "active" {
		t.Fatalf("installation = %+v", installation)
	}
	var connection struct {
		ID string `json:"id"`
	}
	s.owner.do(http.MethodPost, s.tenantPath("/github/connections"), map[string]any{
		"installation_id": installation.ID,
		"repository_id":   repositoryID,
		"project_id":      s.project,
		"application_id":  s.application,
		"default_branch":  defaultBranch,
	}, http.StatusCreated, &connection)
}

// deliver posts one signed webhook, the way GitHub does.
func (s *scenario) deliver(name, delivery string, edits map[string]any) {
	t := s.t
	body := githubtest.Fixture(name)
	if len(edits) > 0 {
		body = retarget(t, body, edits)
	}
	req, err := githubtest.WebhookRequest(s.d.base+"/v1/integrations/github/webhooks",
		[]byte(webhookSecret), githubtest.FixtureEvent(name), delivery, body)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("deliver %s: %v", name, err)
	}
	defer resp.Body.Close()
	var ack struct {
		Accepted bool `json:"accepted"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ack); err != nil || resp.StatusCode != http.StatusAccepted {
		t.Fatalf("deliver %s: HTTP %d (%v)", name, resp.StatusCode, err)
	}
	if !ack.Accepted {
		t.Fatalf("deliver %s (%s): the inbox had seen it already", name, delivery)
	}
}

// retarget rewrites dotted paths in a webhook fixture.
func retarget(t interface{ Fatal(...any) }, body []byte, edits map[string]any) []byte {
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	for path, v := range edits {
		parts := strings.Split(path, ".")
		at := doc
		for _, p := range parts[:len(parts)-1] {
			next, ok := at[p].(map[string]any)
			if !ok {
				t.Fatal(fmt.Errorf("webhook fixture has no %s", path))
			}
			at = next
		}
		at[parts[len(parts)-1]] = v
	}
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func prEdits(branch, commit string, number int, openedAt time.Time) map[string]any {
	return map[string]any{
		"number":                  number,
		"pull_request.number":     number,
		"pull_request.head.ref":   branch,
		"pull_request.head.sha":   commit,
		"pull_request.created_at": openedAt.UTC().Format(time.RFC3339),
	}
}

// openPullRequest connects the repository and opens the pull request
// whose head commit is the one the CLI just graded.
func (s *scenario) openPullRequest() {
	t := s.t
	s.connectRepository()
	s.deliver("pull_request.opened", "m4-pr-opened", prEdits(prBranch, headCommit, prNumber, time.Now()))

	eventually(t, 60*time.Second, "the branch to open", func() (bool, string) {
		for _, b := range list[struct {
			Name     string `json:"name"`
			State    string `json:"state"`
			PRNumber int    `json:"pr_number"`
		}](s.owner, s.projectPath("/branches"), nil) {
			if b.Name == prBranch && b.State == "open" {
				return true, ""
			}
		}
		return false, "no open branch"
	})

	// The branch's CI: the same catalogs `main` holds, so the two
	// surfaces grade the same content, plus the build's usages at the
	// pull request's head commit.
	var out pushJSON
	s.ci.run("push", "--translations", "--branch", prBranch, "--pr", strconv.Itoa(prNumber),
		"--commit", headCommit, "--json")
	_ = out
	s.branchUsages(prBranch, headCommit, "usages.pr.json")
}

// branchUsages uploads the build's usages for one branch and commit.
func (s *scenario) branchUsages(branch, commit, name string) {
	t := s.t
	raw, err := os.ReadFile(filepath.Join(s.repo.dir, "usages.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc["commit"], doc["branch"] = commit, branch
	writeJSON(t, filepath.Join(s.repo.dir, name), doc)
	var pushed contextPushJSON
	s.ci.ok(&pushed, "context", "push", name)
}

// waitForCheck waits until the Glossa check for a commit has completed.
func (s *scenario) waitForCheck(commit string) (checkRunView, bool) {
	var got checkRunView
	ok, state := softly(3*time.Minute, func() (bool, string) {
		for _, r := range s.d.github.CheckRuns(repositoryID) {
			if r.HeadSHA != commit || r.Name != "Glossa" {
				continue
			}
			got = checkRunView{
				Name: r.Name, HeadSHA: r.HeadSHA, Status: r.Status, Conclusion: r.Conclusion,
				Title: r.Title, Summary: r.Summary, Annotations: len(r.Annotations), Patches: r.Patches,
			}
			if r.Status == "completed" && r.Conclusion != "" {
				return true, ""
			}
			return false, fmt.Sprintf("%s/%s after %d patches", r.Status, r.Conclusion, r.Patches)
		}
		return false, "no check run yet"
	})
	if !ok {
		s.t.Logf("the check run for %s never completed: %s", short(commit), state)
	}
	return got, ok
}

// agreement is the exit criterion. It compares the CLI's verdict with
// the pull request's, number by number.
func (s *scenario) agreement() {
	run, ok := s.waitForCheck(headCommit)
	if !ok {
		s.gap("12.3", "the pull request's Glossa check never completed, so there is nothing to compare "+
			"the CLI's verdict with")
		return
	}
	s.prCheck = run
	s.prLayers = parseLayerTable(run.Summary)

	// The CLI's side. The workflow runs two commands on one commit, and
	// the second — `glossa capture --check` — is the one that has every
	// layer the server has, because the visual layer needs a browser
	// the plain check has no access to. That is the comparable run; the
	// plain `glossa check` document is §12.2's artifact.
	cli := s.cliCheck
	s.agreedWith = "`glossa check`"
	if s.cliCapture.Check != nil {
		cli = *s.cliCapture.Check
		s.agreedWith = "`glossa capture --check`"
	}
	cliLayers := byLayer(cli.Findings)

	prErrors, prWarnings, prWaived := totalsOf(s.prLayers)
	s.agreeRows = []agreementRow{
		{What: "conclusion", CLI: string(cli.Conclusion), PR: run.Conclusion,
			Agrees: string(cli.Conclusion) == run.Conclusion},
		{What: "errors", CLI: strconv.Itoa(cli.Errors), PR: strconv.Itoa(prErrors),
			Agrees: cli.Errors == prErrors},
		{What: "warnings", CLI: strconv.Itoa(cli.Warnings), PR: strconv.Itoa(prWarnings),
			Agrees: cli.Warnings == prWarnings},
		{What: "waived", CLI: strconv.Itoa(cli.Waived), PR: strconv.Itoa(prWaived),
			Agrees: cli.Waived == prWaived},
	}
	layers := map[domain.Layer]bool{}
	for l := range cliLayers {
		layers[l] = true
	}
	for l := range s.prLayers {
		layers[l] = true
	}
	names := make([]string, 0, len(layers))
	for l := range layers {
		names = append(names, string(l))
	}
	sort.Strings(names)
	for _, n := range names {
		l := domain.Layer(n)
		a, b := cliLayers[l], s.prLayers[l]
		s.agreeRows = append(s.agreeRows, agreementRow{
			What:   "layer `" + n + "` (e/w/x)",
			CLI:    fmt.Sprintf("%d/%d/%d", a.Errors, a.Warnings, a.Waived),
			PR:     fmt.Sprintf("%d/%d/%d", b.Errors, b.Warnings, b.Waived),
			Agrees: a == b,
		})
	}
	if !allAgree(s.agreeRows) {
		// The cause, once, rather than the symptom per row. It is not a
		// fixture accident: the two surfaces compute different findings
		// over different scopes.
		s.gap("12.3", "the terminal and the pull request do not reach the same verdict, and the reason is "+
			"structural rather than a fixture accident. `glossa check` **recomputes every layer over the "+
			"whole project** — `snapshot.FromServer` reads the active messages and every translation, and "+
			"`quality/app.RunIn` runs Structure, Parity and Completeness over all of them. The pull-request "+
			"check **reads what is already stored, for the branch's own keys only**: "+
			"`integration/adapters/sources.Checks.qaFindings` returns nothing at all when the branch proposes "+
			"no new key and no source change (`len(branchKeys) == 0`), and otherwise returns the warnings "+
			"Localization kept when each translation was *written* plus a live terminology check. A parity "+
			"break a later source revision created is stored nowhere, so the pull request cannot see it; and "+
			"missing and outdated translations are rolled up to **one finding per locale** on the pull "+
			"request against **one per message and locale** in the terminal. Two surfaces, two scopes, two "+
			"arithmetics. The rows above are where that shows.")
		for _, row := range s.agreeRows {
			if !row.Agrees {
				s.t.Logf("  %s: `glossa check` %s, the check run %s", row.What, row.CLI, row.PR)
			}
		}
	}
	if run.Conclusion == "failure" && run.Annotations == 0 {
		s.gap("12.3", "the failing check run carries no annotations")
	}
	// The located findings are the ones GitHub can annotate.
	located := 0
	for _, f := range cli.Findings {
		if f.Locus.File != "" && f.Locus.Line > 0 {
			located++
		}
	}
	if located > 0 && run.Annotations == 0 {
		s.gap("12.3", "%d findings name a file and a line and none of them was annotated", located)
	}
	s.stickyFound = len(s.stickyComments())
	if s.stickyFound != 1 {
		s.gap("12.3", "the pull request carries %d sticky comments, want exactly one", s.stickyFound)
	}
	if !strings.Contains(run.Summary, fmt.Sprintf("check policy v%d", s.policyVersion)) {
		s.gap("12.3", "the check run's summary does not name the policy version it graded against (v%d)", s.policyVersion)
	}
	s.note("12.3", "The check run on `%s` is `%s`/`%s` — %q — with %d annotations and one sticky comment, "+
		"and its per-layer table carries %d errors, %d warnings and %d waived.",
		short(headCommit), run.Status, run.Conclusion, run.Title, run.Annotations, prErrors, prWarnings, prWaived)
	if allAgree(s.agreeRows) {
		s.note("12.3", "Two surfaces, one verdict: every number above matches.")
	}
}

func allAgree(rows []agreementRow) bool {
	for _, r := range rows {
		if !r.Agrees {
			return false
		}
	}
	return true
}

func totalsOf(m map[domain.Layer]layerCount) (errors, warnings, waived int) {
	for _, c := range m {
		errors += c.Errors
		warnings += c.Warnings
		waived += c.Waived
	}
	return
}

// layerRow reads one line of the check run's "Findings by layer" table.
var layerRow = regexp.MustCompile(`(?m)^\| ([a-z]+) \| (\d+) \| (\d+) \| (\d+) \|$`)

// parseLayerTable reads the per-layer counts back out of the check
// run's own summary — the surface a person reads, not a second
// computation of it.
func parseLayerTable(summary string) map[domain.Layer]layerCount {
	out := map[domain.Layer]layerCount{}
	for _, m := range layerRow.FindAllStringSubmatch(summary, -1) {
		layer := domain.Layer(m[1])
		if !layer.Valid() {
			continue
		}
		e, _ := strconv.Atoi(m[2])
		w, _ := strconv.Atoi(m[3])
		x, _ := strconv.Atoi(m[4])
		out[layer] = layerCount{Errors: e, Warnings: w, Waived: x}
	}
	return out
}

type comment struct {
	ID    int64
	Body  string
	Login string
}

func (s *scenario) stickyComments() []comment {
	var out []comment
	for _, c := range s.d.github.Comments(repositoryID, prNumber) {
		if strings.Contains(c.Body, "<!-- glossa:sticky -->") {
			out = append(out, comment{ID: c.ID, Body: c.Body, Login: c.Login})
		}
	}
	return out
}
