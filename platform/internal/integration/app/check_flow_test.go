package app_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/felixgeelhaar/glossa/platform/internal/integration/adapters/github"
	"github.com/felixgeelhaar/glossa/platform/internal/integration/adapters/github/githubtest"
	"github.com/felixgeelhaar/glossa/platform/internal/integration/app"
	"github.com/felixgeelhaar/glossa/platform/internal/integration/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/checkpolicy"
)

// The Glossa PR check, end to end against the fake GitHub (RFC 0004
// §6.4). These pin the decisions the slice exists for: the check is
// queued the moment the pull request is, it completes when that
// commit's CI has been ingested, an annotation is sent exactly once per
// check run, and a pull request has exactly one comment however often
// the job runs.

const (
	branchName = "feature/payment-copy"
	headSHA    = "9f1c2e3d4b5a69788796a5b4c3d2e1f0a9b8c7d6"
	pullNumber = 7
)

// prEdits retargets a recorded pull_request payload at the fixture's
// installation and repository.
func prEdits() map[string]any {
	return map[string]any{
		"installation.id":           float64(instGitHubID),
		"repository.id":             float64(repoGitHubID),
		"pull_request.head.repo.id": float64(repoGitHubID),
	}
}

// openPR delivers a pull_request event and drains the inbox.
func (f *fixture) openPR(t *testing.T, name, deliveryID string) {
	t.Helper()
	if _, err := f.deliverBody(t, "pull_request", deliveryID, retarget(t, name, prEdits())); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	f.drain(t)
}

// runCheck runs the check worker until the queue is quiet.
func (f *fixture) runCheck(t *testing.T) {
	t.Helper()
	for range 20 {
		worked, err := f.checker.RunOnce(context.Background())
		if err != nil {
			t.Fatalf("check worker: %v", err)
		}
		if !worked {
			return
		}
	}
	t.Fatal("the check worker never settled")
}

// ci makes the sources say this commit's push and usages were ingested.
func (f *fixture) ci(headCommit string) {
	f.sources.set(func(m *memSources) {
		m.status.HeadCommit = headCommit
		m.usages.Builds, m.usages.Commits = 1, []string{headCommit}
	})
}

// theCheck is the fake's single check run on the repository.
func (f *fixture) theCheck(t *testing.T) githubtest.CheckRun {
	t.Helper()
	runs := f.fake.CheckRuns(repoGitHubID)
	if len(runs) != 1 {
		t.Fatalf("%d check runs, want exactly 1: %+v", len(runs), runs)
	}
	return runs[0]
}

// theComment is the pull request's single comment.
func (f *fixture) theComment(t *testing.T) githubtest.Comment {
	t.Helper()
	comments := f.fake.Comments(repoGitHubID, pullNumber)
	if len(comments) != 1 {
		t.Fatalf("%d comments, want exactly 1: %+v", len(comments), comments)
	}
	return comments[0]
}

// TestCheckIsQueuedWhenThePullRequestOpens: the check run exists before
// CI does, so the pull request shows at once that Glossa is looking.
func TestCheckIsQueuedWhenThePullRequestOpens(t *testing.T) {
	f := newFixture(t)
	f.connected(t)
	f.openPR(t, "pull_request.opened", "d-open")
	f.runCheck(t)

	run := f.theCheck(t)
	if run.Status != app.CheckQueued || run.Conclusion != "" {
		t.Fatalf("check run = %+v, want queued with no conclusion", run)
	}
	if run.HeadSHA != headSHA || run.Name != domain.CheckName {
		t.Fatalf("check run = %+v, want %s on %s", run, domain.CheckName, headSHA)
	}
	// The comment already carries the branch's links, and says why there
	// is nothing to report yet.
	body := f.theComment(t).Body
	for _, want := range []string{
		"https://studio.example/t/" + tenantOne.String() + "/p/" + projectID.String() + "/translate?branch=",
		"No Glossa CI run for this commit",
		github.StickyMarker,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("comment does not mention %q:\n%s", want, body)
		}
	}
}

// TestCheckFailsOnAnInvalidMessageAndAnnotatesIt is the slice's main
// story: CI pushes a branch with a new key and a message Glossa could
// not accept, and the check fails with an annotation on the line the
// product asks for it.
func TestCheckFailsOnAnInvalidMessageAndAnnotatesIt(t *testing.T) {
	f := newFixture(t)
	f.connected(t)
	f.openPR(t, "pull_request.opened", "d-open")

	f.ci(headSHA)
	f.sources.set(func(m *memSources) {
		m.status.NewKeys = []string{"checkout.pay", "checkout.total"}
		m.status.Invalid = []app.InvalidMessage{
			{Key: "checkout.total", Code: "invalid_content", Detail: "unmatched '{' at offset 12"},
		}
		// The rejected key is not in the catalog, so its usage is an
		// unknown key — which is what gives the annotation its file:line.
		m.usages.Unknown = []app.UnknownKey{
			{Key: "checkout.total", File: "src/checkout/PaymentFooter.vue", Line: 42},
		}
		m.quality.Untranslated = map[string]int{"de": 2, "fr": 2}
	})
	f.runCheck(t)

	run := f.theCheck(t)
	if run.Status != app.CheckCompleted || run.Conclusion != app.ConclusionFailure {
		t.Fatalf("check run = %+v, want a completed failure", run)
	}
	var found bool
	for _, a := range run.Annotations {
		if a.Path == "src/checkout/PaymentFooter.vue" && a.StartLine == 42 && a.AnnotationLevel == "failure" &&
			strings.Contains(a.Message, "unmatched") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no failure annotation at src/checkout/PaymentFooter.vue:42: %+v", run.Annotations)
	}
	// The summary is Markdown with a table per locale.
	for _, want := range []string{"| Locale |", "| de |", "| fr |", "Invalid messages", "checkout.total"} {
		if !strings.Contains(run.Summary, want) {
			t.Fatalf("summary does not mention %q:\n%s", want, run.Summary)
		}
	}
	f.theComment(t) // exactly one
}

// TestFixingTheBranchTurnsTheCheckGreenWithoutDuplicating is the second
// half of the story, and the one that catches the two mistakes this
// slice exists to avoid: GitHub appends annotations, and a second
// comment is worse than no comment.
func TestFixingTheBranchTurnsTheCheckGreenWithoutDuplicating(t *testing.T) {
	f := newFixture(t)
	f.connected(t)
	f.openPR(t, "pull_request.opened", "d-open")

	f.ci(headSHA)
	f.sources.set(func(m *memSources) {
		m.status.NewKeys = []string{"checkout.pay"}
		m.status.Invalid = []app.InvalidMessage{{Key: "checkout.pay", Code: "invalid_content", Detail: "unmatched '{'"}}
		m.usages.Unknown = []app.UnknownKey{{Key: "checkout.pay", File: "src/Pay.vue", Line: 12}}
		m.quality.Untranslated = map[string]int{"de": 1, "fr": 1}
	})
	f.runCheck(t)
	if c := f.theCheck(t).Conclusion; c != app.ConclusionFailure {
		t.Fatalf("conclusion = %q, want failure", c)
	}
	annotated := len(f.theCheck(t).Annotations)
	comment := f.theComment(t).ID

	// Running the same job again changes nothing: the annotations were
	// already sent to this run, and the comment is updated in place.
	if _, err := f.checks.Wake(context.Background(), []int64{repoGitHubID}, branchName, f.clock); err != nil {
		t.Fatal(err)
	}
	f.runCheck(t)
	if n := len(f.theCheck(t).Annotations); n != annotated {
		t.Fatalf("annotations = %d after a repeat, want %d: GitHub appends, so each goes once per run", n, annotated)
	}

	// The message is fixed and the translations land.
	f.sources.set(func(m *memSources) {
		m.status.Invalid = nil
		m.usages.Unknown = nil
		m.quality.Untranslated = map[string]int{"de": 0, "fr": 0}
	})
	if _, err := f.checks.Wake(context.Background(), []int64{repoGitHubID}, branchName, f.clock); err != nil {
		t.Fatal(err)
	}
	f.runCheck(t)

	run := f.theCheck(t)
	if run.Conclusion != app.ConclusionSuccess {
		t.Fatalf("conclusion = %q, want success", run.Conclusion)
	}
	if n := len(run.Annotations); n != annotated {
		t.Fatalf("annotations = %d, want %d: the fixed report adds none and repeats none", n, annotated)
	}
	if got := f.theComment(t); got.ID != comment {
		t.Fatalf("comment id = %d, want the same comment %d updated in place", got.ID, comment)
	}
	if !strings.Contains(f.theComment(t).Body, "Ready") {
		t.Fatalf("the comment was not updated:\n%s", f.theComment(t).Body)
	}
}

// TestRerequestedCheckRunsAgainWithAFreshLedger: a rerun is a new check
// run on GitHub, so its annotations start from nothing.
func TestRerequestedCheckRunsAgainWithAFreshLedger(t *testing.T) {
	f := newFixture(t)
	f.connected(t)
	f.openPR(t, "pull_request.opened", "d-open")

	f.ci(headSHA)
	f.sources.set(func(m *memSources) {
		m.status.NewKeys = []string{"checkout.pay"}
		m.usages.Unknown = []app.UnknownKey{{Key: "stray.key", File: "src/Pay.vue", Line: 3}}
	})
	f.runCheck(t)
	first := f.theCheck(t)
	if len(first.Annotations) != 1 {
		t.Fatalf("annotations = %+v, want one unknown key", first.Annotations)
	}

	// Somebody presses Re-run. The fake keeps the old run, as GitHub
	// does, so the new one is a second row — and that is the point: the
	// ledger belongs to the run.
	body := retarget(t, "check_run.rerequested", map[string]any{
		"installation.id":                   float64(instGitHubID),
		"repository.id":                     float64(repoGitHubID),
		"check_run.head_sha":                headSHA,
		"check_run.check_suite.head_sha":    headSHA,
		"check_run.check_suite.head_branch": branchName,
	})
	if _, err := f.deliverBody(t, "check_run", "d-rerun", body); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	// GitHub would have deleted or superseded the old run; the fake only
	// keeps it, so the stored id is cleared by the rerequest and the
	// worker finds the existing run again. Either way the ledger is empty
	// and the annotations are sent once more, not twice over.
	f.runCheck(t)
	runs := f.fake.CheckRuns(repoGitHubID)
	if len(runs) != 1 {
		t.Fatalf("%d check runs, want 1: %+v", len(runs), runs)
	}
	if n := len(runs[0].Annotations); n != 2 {
		t.Fatalf("annotations = %d, want 2: a rerun sends the report again", n)
	}
	f.theComment(t) // still exactly one
}

// TestCheckCompletesNeutralWhenCINeverRuns is the thirty-minute wait.
func TestCheckCompletesNeutralWhenCINeverRuns(t *testing.T) {
	f := newFixture(t)
	f.connected(t)
	f.openPR(t, "pull_request.opened", "d-open")
	f.runCheck(t)
	if c := f.theCheck(t).Status; c != app.CheckQueued {
		t.Fatalf("status = %q, want it still queued while it waits", c)
	}

	// Nothing arrives. The leased sweep makes it due again, and the
	// worker — the one place that completes a check — concludes.
	f.clock = f.clock.Add(domain.CheckWait + time.Minute)
	if err := f.checker.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.runCheck(t)

	run := f.theCheck(t)
	if run.Status != app.CheckCompleted || run.Conclusion != app.ConclusionNeutral {
		t.Fatalf("check run = %+v, want a neutral conclusion", run)
	}
	if !strings.Contains(run.Title, "No Glossa CI run for this commit") {
		t.Fatalf("title = %q", run.Title)
	}
}

// TestTwoJobsForOnePullRequestDoNotRaceTheComment: the queue row is the
// pull request, so claiming it is the lock. Two workers pulling at once
// leave one comment, not two.
func TestTwoJobsForOnePullRequestDoNotRaceTheComment(t *testing.T) {
	f := newFixture(t)
	f.connected(t)
	f.openPR(t, "pull_request.opened", "d-open")
	f.ci(headSHA)

	var wg sync.WaitGroup
	worked := make([]bool, 4)
	errs := make([]error, 4)
	for i := range worked {
		wg.Go(func() { worked[i], errs[i] = f.checker.RunOnce(context.Background()) })
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
	}
	n := 0
	for _, w := range worked {
		if w {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d workers claimed the pull request, want exactly 1", n)
	}
	f.theComment(t)
	f.theCheck(t)
}

// TestAPullRequestOnAnUnconnectedRepositoryIsIgnored: no connection, no
// check — and nothing is said on GitHub.
func TestAPullRequestOnAnUnconnectedRepositoryIsIgnored(t *testing.T) {
	f := newFixture(t)
	ctx := manager(tenantOne, "person:one")
	f.connect(t, ctx, "code-1") // the installation, but no Git connection

	f.openPR(t, "pull_request.opened", "d-open")
	f.runCheck(t)

	if runs := f.fake.CheckRuns(repoGitHubID); len(runs) != 0 {
		t.Fatalf("check runs on an unconnected repository: %+v", runs)
	}
	if comments := f.fake.Comments(repoGitHubID, pullNumber); len(comments) != 0 {
		t.Fatalf("comments on an unconnected repository: %+v", comments)
	}
	if _, err := f.checks.Check(context.Background(), repoGitHubID, pullNumber); err == nil {
		t.Fatal("a check was queued for a repository that feeds no project")
	}
}

// TestSynchronizeMakesANewCheckRunAndKeepsTheComment: a new commit is a
// new check run with an empty ledger, but the comment is the pull
// request's and stays.
func TestSynchronizeMakesANewCheckRunAndKeepsTheComment(t *testing.T) {
	f := newFixture(t)
	f.connected(t)
	f.openPR(t, "pull_request.opened", "d-open")
	f.ci(headSHA)
	f.sources.set(func(m *memSources) {
		m.usages.Unknown = []app.UnknownKey{{Key: "stray.key", File: "src/Pay.vue", Line: 3}}
	})
	f.runCheck(t)
	comment := f.theComment(t).ID

	edits := prEdits()
	next := "0123456789abcdef0123456789abcdef01234567"
	edits["pull_request.head.sha"] = next
	if _, err := f.deliverBody(t, "pull_request", "d-sync", retarget(t, "pull_request.synchronize", edits)); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	f.ci(next)
	f.runCheck(t)

	runs := f.fake.CheckRuns(repoGitHubID)
	if len(runs) != 2 {
		t.Fatalf("%d check runs, want one per commit: %+v", len(runs), runs)
	}
	for _, r := range runs {
		if n := len(r.Annotations); n != 1 {
			t.Fatalf("check run %d on %s has %d annotations, want 1", r.ID, r.HeadSHA, n)
		}
	}
	if got := f.theComment(t).ID; got != comment {
		t.Fatalf("comment id = %d, want the pull request's own comment %d", got, comment)
	}
}

// TestRequiredLocalesDecideTheConclusion: the check's verdict is the
// project's policy, the same `require_complete` and `fail_on` as
// `glossa check` — never a second one.
func TestRequiredLocalesDecideTheConclusion(t *testing.T) {
	f := newFixture(t)
	f.connected(t)
	f.openPR(t, "pull_request.opened", "d-open")
	f.ci(headSHA)
	f.sources.set(func(m *memSources) {
		m.policy = checkpolicy.Policy{RequireComplete: []string{"de"}}
		m.status.NewKeys = []string{"checkout.pay"}
		m.quality.Untranslated = map[string]int{"de": 0, "fr": 1}
	})
	f.runCheck(t)
	if c := f.theCheck(t).Conclusion; c != app.ConclusionSuccess {
		t.Fatalf("conclusion = %q: fr is not required, so its gap is a warning", c)
	}

	f.sources.set(func(m *memSources) { m.quality.Untranslated = map[string]int{"de": 1, "fr": 1} })
	if _, err := f.checks.Wake(context.Background(), []int64{repoGitHubID}, branchName, f.clock); err != nil {
		t.Fatal(err)
	}
	f.runCheck(t)
	if c := f.theCheck(t).Conclusion; c != app.ConclusionFailure {
		t.Fatalf("conclusion = %q: de is required, so its gap fails", c)
	}
}
