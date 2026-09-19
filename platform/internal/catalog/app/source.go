package app

import (
	"context"

	"github.com/felixgeelhaar/glossa/platform/internal/catalog/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
)

// The read ports other contexts use. They are ordinary use cases —
// authorized, one tenant transaction each — so a caller can't learn
// more through them than through the API.

// MessagesByKeys returns the messages among keys that exist, by key.
// Malformed keys are simply absent.
func (s *Service) MessagesByKeys(ctx context.Context, project domain.ProjectID, keys []string) (map[string]domain.Message, error) {
	if err := authz.Require(ctx, authz.CatalogRead); err != nil {
		return nil, err
	}
	var parsed []domain.MessageKey
	for _, k := range keys {
		if key, err := domain.ParseMessageKey(k); err == nil {
			parsed = append(parsed, key)
		}
	}
	out := map[string]domain.Message{}
	err := s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		if _, err := st.Project(ctx, project); err != nil {
			return err
		}
		found, err := st.MessagesByKeys(ctx, project, parsed)
		for k, m := range found {
			out[string(k)] = m
		}
		return err
	})
	return out, err
}

// SourceRevisionOf returns revision n of a message's source.
func (s *Service) SourceRevisionOf(ctx context.Context, project domain.ProjectID, id domain.MessageID, n int) (domain.SourceRevision, error) {
	if err := authz.Require(ctx, authz.CatalogRead); err != nil {
		return domain.SourceRevision{}, err
	}
	var rev domain.SourceRevision
	err := s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		if _, err := st.MessageByID(ctx, project, id); err != nil {
			return err
		}
		var err error
		rev, err = st.SourceRevision(ctx, id, n)
		return err
	})
	return rev, err
}

// SourceSnapshot is what a release takes from the catalog: the project
// (source locale) and every active message with its current source, in
// key order. Obsolete messages are not released.
type SourceSnapshot struct {
	Project  domain.Project
	Messages []domain.Message
}

// ReleaseSource reads a project's releasable source in one transaction.
// The Release context joins it with Localization's
// TranslationSnapshot by message ID.
func (s *Service) ReleaseSource(ctx context.Context, project domain.ProjectID) (SourceSnapshot, error) {
	if err := authz.Require(ctx, authz.CatalogRead); err != nil {
		return SourceSnapshot{}, err
	}
	var snap SourceSnapshot
	err := s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		var err error
		if snap.Project, err = st.Project(ctx, project); err != nil {
			return err
		}
		snap.Messages, err = st.ActiveMessages(ctx, project)
		return err
	})
	return snap, err
}
