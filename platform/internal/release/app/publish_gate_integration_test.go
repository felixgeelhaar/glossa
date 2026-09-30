//go:build integration

package app_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	catalogdomain "github.com/felixgeelhaar/glossa/platform/internal/catalog/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/checkpolicy"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/pagination"
	"github.com/felixgeelhaar/glossa/platform/internal/release/app"
	"github.com/felixgeelhaar/glossa/platform/internal/release/domain"
)

// setPolicy stores the project's check-policy document, which is where
// the publish gate reads its requirements from.
func (h *harness) setPolicy(t *testing.T, project uuid.UUID, doc checkpolicy.Policy) {
	t.Helper()
	ctx := h.owner()
	id := catalogdomain.ProjectID(project)
	p, err := h.catalog.GetProject(ctx, id)
	if err != nil {
		t.Fatalf("get project: %v", err)
	}
	settings := p.Settings
	settings.CheckPolicy = &doc
	if _, err := h.catalog.UpdateProject(ctx, id, p.Version, catalogdomain.ProjectChange{Settings: &settings}); err != nil {
		t.Fatalf("save policy: %v", err)
	}
}

// gated is a project with two messages, German complete in neither
// sense the policy could ask for: one of the two keys, approved.
func gated(t *testing.T, h *harness) uuid.UUID {
	t.Helper()
	p := h.project(t, []string{"de", "fr"}, map[string]string{
		"app.title": "Brotwerk",
		"app.sub":   "Fresh every morning",
	})
	h.translate(t, p, "app.title", "de", "Brotwerk", "approved")
	h.drain(t)
	return p
}

// lastDeployment is the newest entry in an environment's history.
func (h *harness) lastDeployment(t *testing.T, project uuid.UUID, environment string) domain.Deployment {
	t.Helper()
	ds, _, err := h.svc.ListDeployments(h.owner(), project, environment, pagination.Page{Size: 10})
	if err != nil {
		t.Fatalf("list deployments: %v", err)
	}
	if len(ds) == 0 {
		t.Fatalf("%s has no deployments", environment)
	}
	return ds[0]
}

func TestPublishRefusedBelowTheEnvironmentsCompleteness(t *testing.T) {
	h := newHarness(t)
	p := gated(t, h)
	h.setPolicy(t, p, checkpolicy.Policy{Environments: map[string]checkpolicy.Environment{
		"production": {RequireComplete: checkpolicy.RequiredLocales("de")},
	}})

	_, _, err := h.svc.Publish(h.owner(), p, app.PublishInput{Environment: "production"}, "")
	if !errors.Is(err, domain.ErrPolicyNotMet) {
		t.Fatalf("Publish = %v, want ErrPolicyNotMet", err)
	}
	var notMet *domain.PolicyNotMetError
	if !errors.As(err, &notMet) || len(notMet.Unmet) != 1 || notMet.Unmet[0].Locale != "de" {
		t.Fatalf("refusal = %+v, want de named", err)
	}
	// Nothing was written: not the release, not the pointer, not even
	// the artifacts.
	if n := count(t, "SELECT count(*) FROM release_releases"); n != 0 {
		t.Errorf("%d releases recorded by a refused publish", n)
	}
	if got := len(h.objects.Keys()); got != 0 {
		t.Errorf("%d objects uploaded by a refused publish", got)
	}

	// The same project publishes to staging, which the policy does not
	// name: a document that says nothing about an environment gates
	// nothing there.
	if _, _, err := h.svc.Publish(h.owner(), p, app.PublishInput{Environment: "staging"}, ""); err != nil {
		t.Fatalf("publish to an ungated environment: %v", err)
	}
}

func TestForcedPublishWithoutAReasonIsRefused(t *testing.T) {
	h := newHarness(t)
	p := gated(t, h)
	h.setPolicy(t, p, checkpolicy.Policy{Environments: map[string]checkpolicy.Environment{
		"production": {RequireComplete: checkpolicy.RequiredLocales("de")},
	}})

	for _, reason := range []string{"", "   "} {
		_, _, err := h.svc.Publish(h.owner(), p, app.PublishInput{Environment: "production", Force: true, ForceReason: reason}, "")
		if !errors.Is(err, domain.ErrForceNeedsReason) {
			t.Fatalf("Publish(force, reason=%q) = %v, want ErrForceNeedsReason", reason, err)
		}
	}
	if n := count(t, "SELECT count(*) FROM release_releases"); n != 0 {
		t.Errorf("%d releases recorded by a refused force", n)
	}
}

func TestForcedPublishRecordsItsReason(t *testing.T) {
	h := newHarness(t)
	p := gated(t, h)
	h.setPolicy(t, p, checkpolicy.Policy{Environments: map[string]checkpolicy.Environment{
		"production": {RequireComplete: checkpolicy.RequiredLocales("de")},
	}})
	const reason = "the launch is tomorrow; the German copy lands on Thursday"

	rel, _, err := h.svc.Publish(h.owner(), p,
		app.PublishInput{Environment: "production", Force: true, ForceReason: reason}, "")
	if err != nil {
		t.Fatalf("forced publish: %v", err)
	}

	// Read back through the application: the environment's history says
	// the publish went out against the policy, and why.
	d := h.lastDeployment(t, p, "production")
	if d.ReleaseID != rel.ID || d.Action != domain.ActionPublish {
		t.Fatalf("deployment = %+v, want the forced publish", d)
	}
	if !d.Override.Forced || d.Override.Reason != reason {
		t.Fatalf("override = %+v, want forced with the reason", d.Override)
	}
	// And durably: the row itself carries it, so the audit outlives the
	// process that wrote it.
	if n := count(t, "SELECT count(*) FROM release_deployments WHERE forced AND force_reason = $1", reason); n != 1 {
		t.Errorf("%d rows carry the reason, want 1", n)
	}

	// A publish that meets the gate records no override, however the
	// caller asked: "forced" in the history means the policy was
	// overridden, never that somebody passed a flag.
	h.translate(t, p, "app.sub", "de", "Jeden Morgen frisch", "approved")
	h.drain(t)
	if _, _, err := h.svc.Publish(h.owner(), p,
		app.PublishInput{Environment: "production", Force: true, ForceReason: "belt and braces"}, ""); err != nil {
		t.Fatalf("publish once de is complete: %v", err)
	}
	if d := h.lastDeployment(t, p, "production"); d.Override.Forced || d.Override.Reason != "" {
		t.Errorf("override = %+v on a publish that met the gate", d.Override)
	}
}

func TestEnvironmentWithoutRequireCompleteInheritsTheDocuments(t *testing.T) {
	h := newHarness(t)
	p := gated(t, h)
	// German is complete; French has nothing. The document requires
	// German, and the environment block names only a review state.
	h.translate(t, p, "app.sub", "de", "Jeden Morgen frisch", "approved")
	h.drain(t)
	h.setPolicy(t, p, checkpolicy.Policy{
		RequireComplete: []string{"de"},
		Environments: map[string]checkpolicy.Environment{
			"production": {RequireReview: checkpolicy.ReviewApproved},
		},
	})

	// "I did not say" is not "every locale": French is untranslated and
	// the publish goes through.
	if _, _, err := h.svc.Publish(h.owner(), p, app.PublishInput{Environment: "production"}, ""); err != nil {
		t.Fatalf("Publish = %v, want the document's [de] inherited, not every locale", err)
	}

	// Saying `require_complete: null` is how a project asks for every
	// locale, and then French does stop it.
	h.setPolicy(t, p, checkpolicy.Policy{
		RequireComplete: []string{"de"},
		Environments: map[string]checkpolicy.Environment{
			"production": {RequireComplete: checkpolicy.AllLocales()},
		},
	})
	_, _, err := h.svc.Publish(h.owner(), p, app.PublishInput{Environment: "production"}, "")
	if !errors.Is(err, domain.ErrPolicyNotMet) {
		t.Fatalf("Publish = %v, want ErrPolicyNotMet for fr", err)
	}
}

func TestPublishEnforcesRequireReview(t *testing.T) {
	h := newHarness(t)
	p := gated(t, h)
	// Completeness is not the question here: no locale is required.
	h.setPolicy(t, p, checkpolicy.Policy{Environments: map[string]checkpolicy.Environment{
		"development": {RequireComplete: checkpolicy.RequiredLocales(), RequireReview: checkpolicy.ReviewApproved},
		"production":  {RequireComplete: checkpolicy.RequiredLocales(), RequireReview: checkpolicy.ReviewApproved},
	}})

	// development ships drafts and needs_review, so its releases have
	// not reached approved.
	_, _, err := h.svc.Publish(h.owner(), p, app.PublishInput{Environment: "development"}, "")
	if !errors.Is(err, domain.ErrPolicyNotMet) {
		t.Fatalf("Publish to development = %v, want ErrPolicyNotMet", err)
	}
	// production ships approved text only, so it has.
	if _, _, err := h.svc.Publish(h.owner(), p, app.PublishInput{Environment: "production"}, ""); err != nil {
		t.Fatalf("Publish to production = %v, want nil", err)
	}
	// And the refusal is overridable like any other, with a reason.
	if _, _, err := h.svc.Publish(h.owner(), p, app.PublishInput{
		Environment: "development", Force: true, ForceReason: "a designer needs the draft on the staging cluster",
	}, ""); err != nil {
		t.Fatalf("forced publish to development: %v", err)
	}
	if d := h.lastDeployment(t, p, "development"); !d.Override.Forced {
		t.Errorf("deployment = %+v, want forced", d)
	}
}
