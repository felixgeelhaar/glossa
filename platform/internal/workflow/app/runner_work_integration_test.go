//go:build integration

package app_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	identity "github.com/felixgeelhaar/glossa/platform/internal/identity/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
	widentity "github.com/felixgeelhaar/glossa/platform/internal/workflow/adapters/identity"
	"github.com/felixgeelhaar/glossa/platform/internal/workflow/adapters/postgres"
	"github.com/felixgeelhaar/glossa/platform/internal/workflow/adapters/sources"
	"github.com/felixgeelhaar/glossa/platform/internal/workflow/app"
	"github.com/felixgeelhaar/glossa/platform/internal/workflow/domain"
)

// askThenApprove asks one reviewer for an approval on entry, and when
// one distinct from the author grants it, sets the review state as them.
const askThenApprove = `{
  "schema": "glossa.workflow/v1", "name": "ask-then-approve", "subject": "translation",
  "chart": { "id": "ask-then-approve", "initial": "asking", "states": {
    "asking": { "type": "atomic", "entry": ["ask"], "transitions": [
      { "event": "approval.granted", "target": "done", "guard": "four_eyes", "actions": ["approve"] },
      { "event": "approval.denied", "target": "done" } ] },
    "done": { "type": "final" } } },
  "guards": { "four_eyes": { "use": "approvals_at_least", "n": 1, "distinct_from_author": true } },
  "actions": {
    "ask": { "use": "request_approval", "n": 1, "from": { "role": "reviewer" } },
    "approve": { "use": "set_review_state", "state": "approved" } }
}`

// TestTheRunnerWithAssignmentsAndApprovals runs the runner against the
// real WorkService: the approval request joins the step's transaction,
// the reviewer's grant comes back as workflow.approval.granted naming
// the instance, and the review state is set as that reviewer.
func TestTheRunnerWithAssignmentsAndApprovals(t *testing.T) {
	h := newRunHarness(t)
	w := newWorkHarness(t, h.tenant)
	work := app.NewWorkService(postgres.NewWorkTransactor(w.uow), widentity.NewDirectory(w.ids),
		sources.NewAuthors(h.catalog, h.localization))
	h.useWork(t, work)

	project, _ := h.project(t)
	h.bindDoc(t, project, askThenApprove)
	translator, translatorActor := h.as([]string{"translator"}, "de")
	h.translate(t, translator, project, "Willkommen")
	h.drain(t)

	inst := h.list(t, project)[0]
	log := h.log(t, inst.ID)
	if inst.State != "asking" || len(log) != 1 || log[0].Actions[0].Outcome != app.ActionDone {
		t.Fatalf("after the revision: %+v, log %+v", inst, log)
	}
	var approval uuid.UUID
	var requestedBy string
	if err := env.Super.QueryRow(context.Background(),
		"SELECT id, created_by FROM workflow_approvals WHERE instance_id = $1", inst.ID).Scan(&approval, &requestedBy); err != nil {
		t.Fatalf("the approval request did not commit with the transition: %v", err)
	}
	if requestedBy != translatorActor.String() {
		t.Errorf("the approval was requested by %s, want the translator whose revision moved the instance", requestedBy)
	}

	// The author cannot grant their own text; a reviewer can.
	reviewerMember := w.member("rita@example.com", []string{"reviewer"}, []string{"de"}, identity.VendorID{})
	reviewer := w.as(reviewerMember)
	rp, _ := authz.From(reviewer)
	h.actors.principals[outbox.Actor(rp.Actor.String())] = rp
	if _, err := work.Decide(reviewer, approval, domain.VerdictGranted, ""); err != nil {
		t.Fatal(err)
	}
	h.drain(t)

	done, err := h.instances.GetInstance(h.ctx(), inst.ID)
	if err != nil || done.Status != app.InstanceFinished {
		t.Fatalf("after the grant: %+v (%v), log %+v", done, err, h.log(t, inst.ID))
	}
	log = h.log(t, inst.ID)
	last := log[len(log)-1]
	if last.Event != "approval.granted" || last.Actor != rp.Actor.String() || len(last.Guards) != 1 || !last.Guards[0].Passed {
		t.Errorf("last row = %+v", last)
	}
	v, err := h.localization.GetTranslation(translator, project, "home.title", "de")
	if err != nil || v.State != "approved" {
		t.Fatalf("translation = %+v (%v)", v, err)
	}
	revs, _, err := h.localization.TranslationRevisions(translator, project, "home.title", "de", pageOf(1))
	if err != nil || revs[0].Provenance.By != rp.Actor.String() {
		t.Errorf("approved by %+v, want the reviewer", revs)
	}
}
