package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/context/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

// Usage listing limits.
const (
	DefaultUsageLimit = 100
	MaxUsageLimit     = 1000
)

// UsageQuery narrows a message's usages.
type UsageQuery struct {
	// Branch selects a branch view: that branch's latest builds, falling
	// back to the default branch's. Empty is the default branch's view.
	Branch string
	// Limit caps the usages returned (default 100, at most 1000).
	Limit int
}

func (q UsageQuery) limit() int {
	switch {
	case q.Limit <= 0:
		return DefaultUsageLimit
	case q.Limit > MaxUsageLimit:
		return MaxUsageLimit
	}
	return q.Limit
}

// CurrentBuilds returns the builds whose usages are current in a view
// (RFC 0004 §2.2): per application and source, the latest build on the
// default branch, or on branch where it rebuilt that application.
// Needs catalog.read.
func (s *Service) CurrentBuilds(ctx context.Context, project uuid.UUID, branch string) ([]domain.Build, error) {
	view, err := s.readView(ctx, project, branch)
	if err != nil {
		return nil, err
	}
	var out []domain.Build
	err = s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		current, err := currentBuilds(ctx, st, project, view)
		if err != nil || len(current) == 0 {
			return err
		}
		out, err = st.Builds(ctx, current)
		return err
	})
	return out, err
}

// MessageUsages returns a message's current usages: the default branch
// first, then by application, file and line. Needs catalog.read.
func (s *Service) MessageUsages(ctx context.Context, project, message uuid.UUID, q UsageQuery) ([]UsageView, error) {
	view, err := s.readView(ctx, project, q.Branch)
	if err != nil {
		return nil, err
	}
	var out []UsageView
	err = s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		current, err := currentBuilds(ctx, st, project, view)
		if err != nil || len(current) == 0 {
			return err
		}
		out, err = st.MessageUsages(ctx, message, current, q.limit())
		return err
	})
	return out, err
}

// KeyUsages is MessageUsages for the message a key names now
// (ErrMessageNotFound). Needs catalog.read.
func (s *Service) KeyUsages(ctx context.Context, project uuid.UUID, key string, q UsageQuery) ([]UsageView, error) {
	if err := authz.Require(ctx, authz.CatalogRead); err != nil {
		return nil, err
	}
	ids, err := s.catalog.MessageIDs(ctx, project, []string{key})
	if err != nil {
		return nil, err
	}
	id, ok := ids[key]
	if !ok {
		return nil, ErrMessageNotFound
	}
	return s.MessageUsages(ctx, project, id, q)
}

// Unused is the result of UnusedMessages.
type Unused struct {
	// Messages are the active messages without a usage in any current
	// build, in key order.
	Messages []MessageRef
	// CurrentBuilds counts the builds considered: with none, every
	// message is unused because nothing was uploaded yet.
	CurrentBuilds int
	// Active counts the project's active messages: the context coverage
	// is (Active - len(Messages)) / Active.
	Active int
}

// UnusedMessages lists the active messages with no usage in any current
// build of the view (RFC 0004 §2.2). They are reported, never obsoleted:
// dynamic IDs can't be seen. Needs catalog.read.
func (s *Service) UnusedMessages(ctx context.Context, project uuid.UUID, branch string) (Unused, error) {
	if err := authz.Require(ctx, authz.CatalogRead); err != nil {
		return Unused{}, err
	}
	view, err := parseView(branch)
	if err != nil {
		return Unused{}, err
	}
	active, err := s.catalog.ActiveMessages(ctx, project)
	if err != nil {
		return Unused{}, err
	}
	used := map[uuid.UUID]bool{}
	var out Unused
	err = s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		current, err := currentBuilds(ctx, st, project, view)
		if err != nil || len(current) == 0 {
			return err
		}
		out.CurrentBuilds = len(current)
		ids, err := st.UsedMessages(ctx, current)
		for _, id := range ids {
			used[id] = true
		}
		return err
	})
	if err != nil {
		return Unused{}, err
	}
	out.Messages = []MessageRef{}
	for _, m := range active {
		if !used[m.ID] {
			out.Messages = append(out.Messages, m)
		}
	}
	out.Active = len(active)
	if view == "" {
		t, _ := tenancy.FromContext(ctx)
		s.metrics.Coverage(t, project, out.Active, out.Active-len(out.Messages))
	}
	return out, nil
}

// readView authorizes a read of project's context in a branch view.
func (s *Service) readView(ctx context.Context, project uuid.UUID, branch string) (domain.Branch, error) {
	if err := authz.Require(ctx, authz.CatalogRead); err != nil {
		return "", err
	}
	view, err := parseView(branch)
	if err != nil {
		return "", err
	}
	return view, s.catalog.Project(ctx, project)
}

func currentBuilds(ctx context.Context, st Store, project uuid.UUID, view domain.Branch) ([]uuid.UUID, error) {
	all, err := st.BuildSummaries(ctx, project)
	if err != nil {
		return nil, err
	}
	return domain.CurrentBuilds(all, view), nil
}
