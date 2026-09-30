//go:build system

package m4_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/felixgeelhaar/glossa/platform/internal/cli"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
)

// Waivers and the policy rollout (RFC 0005 §12.4).

// waivers waives the `term_missing`, watches the counts move and the
// conclusion stay, then moves the German source under it and watches it
// come back.
func (s *scenario) waivers() {
	var target *domain.Finding
	for i, f := range s.cliCheck.Findings {
		if f.Layer == domain.LayerTerminology && f.Code == "term_missing" {
			target = &s.cliCheck.Findings[i]
			break
		}
	}
	if target == nil {
		s.gap("12.4", "there is no `term_missing` finding to waive (the terminology layer produced %s)",
			codeList(codesOf(s.cliCheck.Findings, domain.LayerTerminology)))
		return
	}
	before := s.cliCheck

	// A waiver without a reason is a 400; that is the whole mechanism.
	_, err := s.owner.try(http.MethodPost, s.projectPath("/waivers"),
		map[string]any{"fingerprint": target.Fingerprint, "reason": "   "}, http.StatusCreated, nil)
	var ae *apiError
	switch {
	case err == nil:
		s.gap("12.4", "a waiver with a blank reason was accepted")
	case asAPIError(err, &ae) && ae.code() != "waiver_reason_required":
		s.gap("12.4", "a blank reason was refused as %q, want waiver_reason_required", ae.code())
	default:
		s.waiverSteps = append(s.waiverSteps, step{
			What: "a waiver with a blank reason",
			Then: "400 `" + ae.code() + "` — the reason is required and non-empty"})
	}

	const reason = "“cesta” is what Brotwerk's Spanish customers call it; the termbase's word is the newer one."
	var waiver struct {
		ID             string `json:"id"`
		Fingerprint    string `json:"fingerprint"`
		Reason         string `json:"reason"`
		Scope          string `json:"scope"`
		SourceRevision int    `json:"source_revision"`
		Active         bool   `json:"active"`
	}
	// The waiver is made against the source revision of the finding the
	// person is looking at. It is passed explicitly because the default
	// — the newest *stored* finding with this fingerprint — is not
	// available here: `glossa check` computes live and stores nothing,
	// so a waiver made from the terminal has no stored finding to take
	// a revision from.
	create := map[string]any{"fingerprint": target.Fingerprint, "reason": reason}
	if target.SourceRevision != nil {
		create["source_revision"] = *target.SourceRevision
	}
	s.owner.do(http.MethodPost, s.projectPath("/waivers"), create, http.StatusCreated, &waiver)
	if target.SourceRevision != nil && waiver.SourceRevision != *target.SourceRevision {
		s.gap("12.4", "the waiver was recorded against source revision %d, want the finding's %d",
			waiver.SourceRevision, *target.SourceRevision)
	}
	s.waiverSteps = append(s.waiverSteps, step{
		What: fmt.Sprintf("waive `%s` (`%s`) with a reason", target.Code, short(target.Fingerprint)),
		Then: fmt.Sprintf("waiver `%s`, scope `%s`, against source revision %d",
			short(waiver.ID), waiver.Scope, waiver.SourceRevision)})

	after := s.check()
	waived := findingByFingerprint(after.Findings, target.Fingerprint)
	switch {
	case waived == nil:
		s.gap("12.4", "the waived finding vanished from the check: a waived finding is still computed and still reported")
	case waived.Severity != domain.Waived:
		s.gap("12.4", "the waived finding is still `%s` in `glossa check`, because **the check never reads the "+
			"project's waivers**: nothing in `cli/cmd_check.go` fetches them, and `quality/app.RunIn` — the "+
			"function the CLI, the capture check and Studio all call — takes findings and a policy and no "+
			"waivers at all. `domain.Waivers` is applied in exactly one place, `quality/app.RecordRun` "+
			"(runs.go:91), which is the server recording a check run. A waiver therefore changes no local "+
			"check's counts and no local conclusion, which is what §12.4 asks it to change", waived.Severity)
	case waived.Waiver != waiver.ID:
		s.gap("12.4", "the waived finding names waiver %q, want %q", waived.Waiver, waiver.ID)
	}
	switch {
	case after.Waived != before.Waived+1:
		s.gap("12.4", "waived went from %d to %d, want one more", before.Waived, after.Waived)
	case after.Warnings != before.Warnings-1:
		s.gap("12.4", "warnings went from %d to %d, want one fewer", before.Warnings, after.Warnings)
	}
	if after.Conclusion != before.Conclusion {
		s.gap("12.4", "the conclusion moved from `%s` to `%s`: a waiver may not change a verdict",
			before.Conclusion, after.Conclusion)
	}
	got := "gone from the run"
	if waived != nil {
		got = "`" + string(waived.Severity) + "`"
	}
	s.waiverSteps = append(s.waiverSteps, step{
		What: "re-run `glossa check`",
		Then: fmt.Sprintf("the finding is %s; %d→%d warnings, %d→%d waived, conclusion `%s`→`%s`",
			got, before.Warnings, after.Warnings, before.Waived, after.Waived, before.Conclusion, after.Conclusion)})

	// The German the translator waived is not the German that now
	// ships: move the source revision under it.
	s.repo.head["de"][keyTermMissing] = "Warenkorb dauerhaft speichern"
	s.repo.write(s.t, s.repo.head)
	s.ci.run("push", "--json")

	back := s.check()
	reborn := findingByFingerprint(back.Findings, target.Fingerprint)
	switch {
	case reborn == nil:
		s.gap("12.4", "the finding did not come back after its source revision changed")
	case reborn.Severity == domain.Waived:
		s.gap("12.4", "the finding is still waived after its source revision changed (waiver at revision %d, "+
			"finding at %v)", waiver.SourceRevision, reborn.SourceRevision)
	default:
		s.waiverSteps = append(s.waiverSteps, step{
			What: "change the German source behind it",
			Then: fmt.Sprintf("the finding is `%s` again: the waiver was made against source revision %d and the "+
				"finding is now at %d", reborn.Severity, waiver.SourceRevision, deref(reborn.SourceRevision))})
	}
	if back.Waived != before.Waived {
		s.gap("12.4", "after the source moved, waived is %d, want %d again", back.Waived, before.Waived)
	}

	// `glossa waive --list` is the other surface on the same waiver.
	var listed struct {
		Waivers []struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
			Active bool   `json:"active"`
		} `json:"waivers"`
	}
	s.ci.ok(&listed, "waive", "--list")
	if len(listed.Waivers) != 1 || listed.Waivers[0].Reason != reason {
		s.gap("12.4", "`glossa waive --list` shows %d waivers, want the one with its reason", len(listed.Waivers))
	}
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func asAPIError(err error, out **apiError) bool {
	e, ok := err.(*apiError)
	if ok {
		*out = e
	}
	return ok
}

func findingByFingerprint(fs []domain.Finding, fp string) *domain.Finding {
	for i, f := range fs {
		if f.Fingerprint == fp {
			return &fs[i]
		}
	}
	return nil
}

// check re-runs the workflow's check command and returns its document.
func (s *scenario) check() checkJSON {
	var out checkJSON
	res := s.ci.run("check", "--terminology", "--json")
	if err := json.Unmarshal([]byte(res.stdout), &out); err != nil {
		s.t.Fatalf("glossa check --json: %v\n%s\n%s", err, res.stdout, res.stderr)
	}
	return out
}

// policyRollout saves policy v4 — the one that promotes the visual
// layer from `warn` to `enforce` — first as a dry run, then with a
// grace, and watches the open pull request keep its version while a new
// one gets the new rule.
func (s *scenario) policyRollout() {
	// The preview, before the save.
	preview := s.previewPolicy(policyV4(), 14)
	if !preview.DryRun {
		s.gap("12.4", "a dry-run save reported dry_run=false")
	}
	// The preview shows the policy as it *would* stand; nothing may be
	// stored, which only the stored document can say.
	var stored policyState
	s.owner.do(http.MethodGet, s.projectPath("/check-policy"), nil, http.StatusOK, &stored)
	if stored.Version != s.policyVersion {
		s.gap("12.4", "the dry run moved the stored policy to v%d", stored.Version)
	}
	im := preview.Impact
	s.policySteps = append(s.policySteps, step{
		What: "`POST …/check-policy` with `dry_run: true`",
		Then: fmt.Sprintf("%d stored findings examined over %d runs: %d raised, %d lowered, %d silenced; "+
			"%d refs newly failing (%s); **%d open pull requests** would newly fail",
			im.Findings, im.Runs, im.Raised, im.Lowered, im.Silenced,
			im.NewlyFailing, codeList(im.NewlyFailingRefs), im.OpenPullRequests)})
	switch {
	case im.OpenPullRequests < 1:
		s.gap("12.4", "the impact preview says %d open pull requests would newly fail, want at least the one "+
			"that is open. It examined %d stored findings over %d runs and raised %d of them; newly-failing "+
			"refs: %v. The preview reads *stored* findings, and the only findings this project has stored are "+
			"the capture ingest's — so a policy change is previewed against whatever happens to have been "+
			"recorded, not against what a check would compute",
			im.OpenPullRequests, im.Findings, im.Runs, im.Raised, im.NewlyFailingRefs)
	case !contains(im.NewlyFailingRefs, prBranch):
		s.gap("12.4", "the impact preview's newly-failing refs are %v, want `%s` among them", im.NewlyFailingRefs, prBranch)
	}
	// The known shortfall, stated rather than worked around.
	s.note("12.4", "The preview counts the pull requests that would newly fail and names the **refs**, not the "+
		"pull requests: `CheckPolicyImpact` carries `newly_failing_refs` (strings) and `open_pull_requests` "+
		"(an integer). §12.4 asks the preview to *name* the one pull request that would newly fail; today a "+
		"reader gets its branch and a count, and has to look the number up. The ref→PR mapping exists "+
		"server-side (`Catalog.OpenPullRequests`) and is used only to compute the count.")
	if im.OpenPullRequests != 1 {
		s.note("12.4", "The preview counted %d open pull requests, not one.", im.OpenPullRequests)
	}

	// Now the save, with a grace that pins what is already open.
	saved := s.savePolicy(policyV4(), 14)
	if saved.Version != s.policyVersion+1 {
		s.gap("12.4", "the save produced v%d, want v%d", saved.Version, s.policyVersion+1)
	}
	if saved.GraceUntil == "" || saved.PinnedVersion == 0 {
		s.gap("12.4", "the save recorded no grace (grace_until %q, pinned_version %d), so nothing pins the "+
			"open pull request", saved.GraceUntil, saved.PinnedVersion)
	} else {
		s.policySteps = append(s.policySteps, step{
			What: fmt.Sprintf("save v%d with a 14-day grace", saved.Version),
			Then: fmt.Sprintf("v%d keeps grading the pull requests opened before it, until %s",
				saved.PinnedVersion, saved.GraceUntil)})
	}
	newVersion := saved.Version

	// The open pull request: a new commit on it, and the check has to
	// say which version it used.
	s.deliver("pull_request.synchronize", "m4-pr-sync",
		prEdits(prBranch, headCommit, prNumber, time.Now().Add(-2*time.Hour)))
	pinned, ok := softly(2*time.Minute, func() (bool, string) {
		run, done := s.waitForCheck(headCommit)
		if !done {
			return false, "no completed run"
		}
		if strings.Contains(run.Summary, fmt.Sprintf("check policy v%d", s.policyVersion)) &&
			strings.Contains(run.Summary, "the version this pull request was opened under") {
			return true, ""
		}
		return false, firstLineContaining(run.Summary, "check policy v")
	})
	if !pinned {
		s.gap("12.4", "the open pull request's check does not say it is graded against v%d: %s", s.policyVersion, ok)
	} else {
		s.policySteps = append(s.policySteps, step{
			What: fmt.Sprintf("the open pull request (`%s`, opened under v%d)", prBranch, s.policyVersion),
			Then: fmt.Sprintf("still graded against v%d, and its summary says so: “Graded against the project's "+
				"check policy v%d — the version this pull request was opened under.”", s.policyVersion, s.policyVersion)})
	}

	// A new pull request gets v4 immediately, and the visual layer now
	// gates. It needs the visual evidence on its own commit, which is
	// what its CI captures.
	s.repo.write(s.t, s.repo.head)
	// Opened after the save, and said so in whole seconds: GitHub's
	// `created_at` is RFC 3339 to the second and `effective_from` is
	// not, so a pull request opened in the same second as the save
	// would round to just before it and be pinned.
	s.deliver("pull_request.opened", "m4-pr2-opened",
		prEdits(laterBranch, laterCommit, laterPRNumber, time.Now().Add(time.Minute)))
	eventually(s.t, 60*time.Second, "the second branch to open", func() (bool, string) {
		for _, b := range list[struct {
			Name  string `json:"name"`
			State string `json:"state"`
		}](s.owner, s.projectPath("/branches"), nil) {
			if b.Name == laterBranch && b.State == "open" {
				return true, ""
			}
		}
		return false, "not yet"
	})
	s.ci.run("push", "--translations", "--branch", laterBranch, "--pr", strconv.Itoa(laterPRNumber),
		"--commit", laterCommit, "--json")
	s.branchUsages(laterBranch, laterCommit, "usages.pr2.json")
	for range 2 {
		s.ci.run("capture", "--check", "--upload", "--no-coverage", "--base-url", s.appURL,
			"--commit", laterCommit, "--branch", laterBranch, "--json")
	}

	later, done := s.waitForCheck(laterCommit)
	switch {
	case !done:
		s.gap("12.4", "the second pull request's check never completed")
	case !strings.Contains(later.Summary, fmt.Sprintf("check policy v%d", newVersion)):
		s.gap("12.4", "the new pull request was graded against %q, want v%d",
			firstLineContaining(later.Summary, "check policy v"), newVersion)
	case later.Conclusion != "failure":
		s.gap("12.4", "the new pull request concluded `%s`; with `visual` at `enforce` and a clipped button on "+
			"its commit it has to fail. It cannot, for the same reason §12.2's crop cannot: the capture "+
			"manifest has no `findings` field, so no visual finding is ever stored, so the pull-request check "+
			"has none to grade and `visual: enforce` gates nothing", later.Conclusion)
	default:
		visual := parseLayerTable(later.Summary)[domain.LayerVisual]
		s.policySteps = append(s.policySteps, step{
			What: fmt.Sprintf("a pull request opened after the save (`%s`)", laterBranch),
			Then: fmt.Sprintf("graded against v%d and `%s`: the visual layer carries %d errors",
				newVersion, later.Conclusion, visual.Errors)})
	}
	s.policyVersion = newVersion
}

func contains(items []string, want string) bool {
	for _, s := range items {
		if s == want {
			return true
		}
	}
	return false
}

func firstLineContaining(text, want string) string {
	for _, l := range strings.Split(text, "\n") {
		if strings.Contains(l, want) {
			return strings.TrimSpace(l)
		}
	}
	return "(the summary names no policy version)"
}

var _ = cli.ExitOK
