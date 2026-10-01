//go:build integration

package main

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	auditpg "github.com/felixgeelhaar/glossa/platform/internal/audit/adapters/postgres"
	auditapp "github.com/felixgeelhaar/glossa/platform/internal/audit/app"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/db"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

// The composition root wires the Audit context end to end: a sign-in
// over HTTP is recorded directly, the events it causes (the individual
// tenant, its owner) reach the trail through the outbox, and the chain
// the server wrote verifies.
func TestTheServerKeepsAnAuditTrail(t *testing.T) {
	s := startServer(t)
	ada := s.signIn("ada@example.com")
	r := s.do(call{method: "GET", path: "/v1/me", cookie: ada.cookie})
	r.want(t, http.StatusOK, "")
	var me struct {
		Person struct {
			ID                 string `json:"id"`
			IndividualTenantID string `json:"individual_tenant_id"`
		} `json:"person"`
	}
	r.decode(t, &me)
	tenant, err := tenancy.ParseID(me.Person.IndividualTenantID)
	if err != nil {
		t.Fatal(err)
	}
	ctx := tenancy.ContextWithTenant(context.Background(), tenant)
	store := auditpg.NewStore(db.NewUnitOfWork(s.db.App))

	want := []string{"identity.person.signed_in", "identity.tenant.created", "identity.member.added"}
	deadline := time.Now().Add(10 * time.Second)
	var actions []string
	for time.Now().Before(deadline) {
		es, err := store.Entries(ctx, 0, 100)
		if err != nil {
			t.Fatal(err)
		}
		actions = actions[:0]
		for _, e := range es {
			actions = append(actions, e.Action)
			if e.Actor != "person:"+me.Person.ID {
				t.Errorf("%s by %s, want ada", e.Action, e.Actor)
			}
			if e.Action == "identity.person.signed_in" {
				if _, err := uuid.Parse(e.RequestID); err != nil {
					t.Errorf("the sign-in entry has no request id: %q", e.RequestID)
				}
			}
		}
		if len(actions) >= len(want) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	for _, w := range want {
		if !slices.Contains(actions, w) {
			t.Errorf("the trail lacks %s: %v", w, actions)
		}
	}
	report, err := auditapp.New(store).Verify(ctx)
	if err != nil || report.Entries != int64(len(actions)) {
		t.Errorf("verify: %v (%d entries)", err, report.Entries)
	}
	if strings.Contains(s.logs.String(), "was not audited") {
		t.Errorf("the server logged an audit failure:\n%s", s.logs)
	}
}
