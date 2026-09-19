// Package catalog adapts Catalog's application service to Context's
// Catalog port: the only way Context reads projects, applications and
// messages (RFC 0002 §4, contexts reach each other through application
// ports). Every call is an ordinary authorized Catalog use case, so
// Context learns nothing its caller couldn't read through the API.
package catalog

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	catalogapp "github.com/felixgeelhaar/glossa/platform/internal/catalog/app"
	catalogdomain "github.com/felixgeelhaar/glossa/platform/internal/catalog/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/context/app"
	"github.com/felixgeelhaar/glossa/platform/internal/context/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/pagination"
)

// keyBatch bounds the keys one Catalog lookup carries.
const keyBatch = 1000

// Port implements app.Catalog on Catalog's service.
type Port struct{ svc *catalogapp.Service }

// New returns the port.
func New(svc *catalogapp.Service) *Port { return &Port{svc: svc} }

var _ app.Catalog = (*Port)(nil)

// projectNotFound maps Catalog's not-found (of the project) to
// Context's.
func projectNotFound(err error) error {
	if errors.Is(err, catalogapp.ErrNotFound) {
		return app.ErrProjectNotFound
	}
	return err
}

// Project implements app.Catalog.
func (p *Port) Project(ctx context.Context, project uuid.UUID) error {
	_, err := p.svc.GetProject(ctx, catalogdomain.ProjectID(project))
	return projectNotFound(err)
}

// Application implements app.Catalog: a project has few applications,
// so it pages through them for the slug.
func (p *Port) Application(ctx context.Context, project uuid.UUID, slug string) (uuid.UUID, error) {
	page := pagination.Page{Size: pagination.MaxPageSize}
	for {
		apps, next, err := p.svc.ListApplications(ctx, catalogdomain.ProjectID(project), page)
		if err != nil {
			return uuid.Nil, projectNotFound(err)
		}
		for _, a := range apps {
			if string(a.Slug) == slug {
				return a.ID.UUID(), nil
			}
		}
		if next == nil {
			return uuid.Nil, app.ErrApplicationNotFound
		}
		page.After = apps[len(apps)-1].ID.String()
	}
}

// MessageIDs implements app.Catalog with Catalog's key lookup, in
// batches. Messages of every state resolve: a usage of an obsolete
// message still names it.
func (p *Port) MessageIDs(ctx context.Context, project uuid.UUID, keys []string) (map[string]uuid.UUID, error) {
	out := make(map[string]uuid.UUID, len(keys))
	for start := 0; start < len(keys); start += keyBatch {
		found, err := p.svc.MessagesByKeys(ctx, catalogdomain.ProjectID(project), keys[start:min(start+keyBatch, len(keys))])
		if err != nil {
			return nil, projectNotFound(err)
		}
		for k, m := range found {
			out[k] = m.ID.UUID()
		}
	}
	return out, nil
}

// ActiveMessages implements app.Catalog with Catalog's read of every
// active message (one transaction, key order).
func (p *Port) ActiveMessages(ctx context.Context, project uuid.UUID) ([]app.MessageRef, error) {
	snap, err := p.svc.ReleaseSource(ctx, catalogdomain.ProjectID(project))
	if err != nil {
		return nil, projectNotFound(err)
	}
	out := make([]app.MessageRef, len(snap.Messages))
	for i, m := range snap.Messages {
		out[i] = app.MessageRef{ID: m.ID.UUID(), Key: string(m.Key)}
	}
	return out, nil
}

// ClosedBranches implements app.Catalog. Catalog has no branches until
// the branch overlay (RFC 0004 §4.1) adds them, so no branch is closed
// yet and retention keeps the latest builds of every branch.
func (p *Port) ClosedBranches(context.Context, uuid.UUID) (map[domain.Branch]time.Time, error) {
	return nil, nil
}
