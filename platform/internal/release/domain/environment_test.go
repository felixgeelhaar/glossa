package domain_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/release/domain"
)

func TestNewPolicy(t *testing.T) {
	p, err := domain.NewPolicy([]string{"approved", "draft", "approved"}, false)
	if err != nil || !slices.Equal(p.States, []string{"draft", "approved"}) || p.IncludeOutdated {
		t.Fatalf("policy = %+v, %v", p, err)
	}
	for _, bad := range [][]string{nil, {}, {"rejected"}, {"approved", "published"}} {
		if _, err := domain.NewPolicy(bad, true); !errors.Is(err, domain.ErrInvalidPolicy) {
			t.Errorf("NewPolicy(%v) = %v", bad, err)
		}
	}
}

func TestDefaultPolicies(t *testing.T) {
	cases := map[string][]string{
		"production":  {"approved"},
		"staging":     {"approved"},
		"preview":     {"draft", "needs_review", "approved"},
		"development": {"draft", "needs_review", "approved"},
		"pr-42":       {"draft", "needs_review", "approved"},
	}
	for env, want := range cases {
		if got := domain.DefaultPolicy(env); !slices.Equal(got.States, want) || !got.IncludeOutdated {
			t.Errorf("%s: %+v", env, got)
		}
	}
}

func TestPolicyCovers(t *testing.T) {
	prod := domain.DefaultPolicy("production")
	preview := domain.DefaultPolicy("preview")
	strict := domain.Policy{States: []string{"approved"}}
	cases := []struct {
		name        string
		target, rel domain.Policy
		want        bool
	}{
		{"same", prod, prod, true},
		{"preview release into production", prod, preview, false},
		{"production release into preview", preview, prod, true},
		{"outdated into a policy without outdated", strict, prod, false},
		{"no outdated into a policy with outdated", prod, strict, true},
	}
	for _, tc := range cases {
		if got := tc.target.Covers(tc.rel); got != tc.want {
			t.Errorf("%s: Covers = %v", tc.name, got)
		}
	}
}

func TestEnvironment(t *testing.T) {
	now := time.Date(2026, 9, 19, 8, 0, 0, 0, time.UTC)
	project := uuid.New()
	if _, err := domain.NewEnvironment(project, "a", domain.DefaultPolicy("a"), now); !errors.Is(err, domain.ErrInvalidEnvironment) {
		t.Errorf("reserved name: %v", err)
	}
	if _, err := domain.NewEnvironment(project, "qa", domain.Policy{States: []string{"rejected"}}, now); !errors.Is(err, domain.ErrInvalidPolicy) {
		t.Errorf("bad policy: %v", err)
	}
	e, err := domain.NewEnvironment(project, "qa", domain.DefaultPolicy("qa"), now)
	if err != nil || e.Version != 1 || e.Current != uuid.Nil {
		t.Fatalf("%+v, %v", e, err)
	}
	r := uuid.New()
	if !e.Point(r, now) || e.Version != 2 || e.Current != r {
		t.Fatalf("point: %+v", e)
	}
	if e.Point(r, now) || e.Version != 2 {
		t.Error("pointing at the served release changed the environment")
	}
	if e.ChangePolicy(domain.DefaultPolicy("qa"), now) {
		t.Error("an equal policy is a change")
	}
	if !e.ChangePolicy(domain.DefaultPolicy("production"), now) || e.Version != 3 {
		t.Errorf("policy change: %+v", e)
	}
}

func TestDeliveryKey(t *testing.T) {
	now := time.Now()
	if _, err := domain.NewDeliveryKey(uuid.New(), "", "person:x", now); !errors.Is(err, domain.ErrInvalidKeyName) {
		t.Errorf("empty name: %v", err)
	}
	k, err := domain.NewDeliveryKey(uuid.New(), "web", "person:x", now)
	if err != nil || !k.Active() {
		t.Fatal(err)
	}
	if err := k.Revoke("person:y", now); err != nil || k.Active() || k.RevokedBy != "person:y" {
		t.Fatalf("revoke: %v %+v", err, k)
	}
	if err := k.Revoke("person:y", now); !errors.Is(err, domain.ErrKeyRevoked) {
		t.Errorf("second revoke: %v", err)
	}
}
