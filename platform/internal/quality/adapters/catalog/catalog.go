// Package catalog adapts Catalog's application service to Quality's
// Catalog port: the only way Quality learns that a project exists
// (RFC 0002 §4, contexts reach each other through application ports).
// The call is an ordinary authorized Catalog use case, so Quality
// learns nothing its caller couldn't read through the API.
package catalog

import (
	"context"
	"errors"

	"github.com/google/uuid"

	catalogapp "github.com/felixgeelhaar/glossa/platform/internal/catalog/app"
	catalogdomain "github.com/felixgeelhaar/glossa/platform/internal/catalog/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/app"
)

// Port implements app.Catalog on Catalog's service.
type Port struct{ svc *catalogapp.Service }

// New returns the port.
func New(svc *catalogapp.Service) *Port { return &Port{svc: svc} }

var _ app.Catalog = (*Port)(nil)

// Project implements app.Catalog.
func (p *Port) Project(ctx context.Context, project uuid.UUID) error {
	_, err := p.svc.GetProject(ctx, catalogdomain.ProjectID(project))
	if errors.Is(err, catalogapp.ErrNotFound) {
		return app.ErrProjectNotFound
	}
	return err
}
