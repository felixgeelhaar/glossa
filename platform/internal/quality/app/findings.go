package app

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
)

// Findings listing limits. A stored run can hold thousands of findings,
// so nothing reads them all at once.
const (
	DefaultFindingLimit = 50
	MaxFindingLimit     = 200
)

// Service reads what Quality stored. Running the layers is Run's job
// and needs no state; reading a stored run does, so it lives here.
type Service struct{ tx Transactor }

// NewService returns the service on tx.
func NewService(tx Transactor) *Service { return &Service{tx: tx} }

// FindingQuery asks for a page of a stored run's findings.
type FindingQuery struct {
	// Ref selects the run's ref (a branch, or an environment name);
	// empty takes the project's latest run whatever it checked.
	Ref string
	FindingFilter
	// Limit is the page size (default DefaultFindingLimit, at most
	// MaxFindingLimit); After is the finding id the page continues from.
	Limit int
	After string
}

// FindingsPage is a page of one run's findings, with the run it came
// from: a finding without its run cannot be dated or graded.
type FindingsPage struct {
	Run      RunSummary
	Findings []StoredFinding
	// Next is the id to continue from, or "" on the last page.
	Next string
}

// ListFindings returns a page of the latest check run's findings for a
// project. Needs catalog.read.
//
// It reads the *latest* run rather than every stored one because that
// is what every surface shows (RFC 0005 §2.2 rule 3): a finding from a
// superseded run is history, not a problem anyone can act on.
func (s *Service) ListFindings(ctx context.Context, project uuid.UUID, q FindingQuery) (FindingsPage, error) {
	if err := authz.Require(ctx, authz.CatalogRead); err != nil {
		return FindingsPage{}, err
	}
	if err := q.validate(); err != nil {
		return FindingsPage{}, err
	}
	limit := q.limit()
	var page FindingsPage
	err := s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		run, err := st.LatestRun(ctx, project, q.Ref, true)
		if err != nil {
			return err
		}
		page.Run = run
		// One row more than the page, so the cursor can be issued
		// without a second count.
		found, err := st.Findings(ctx, run.ID, q.FindingFilter, q.After, limit+1)
		if err != nil {
			return err
		}
		if len(found) > limit {
			found = found[:limit]
			page.Next = found[len(found)-1].ID.String()
		}
		page.Findings = found
		return nil
	})
	if err != nil {
		return FindingsPage{}, err
	}
	if page.Findings == nil {
		page.Findings = []StoredFinding{}
	}
	return page, nil
}

// validate rejects a filter outside the closed vocabularies. A layer or
// severity nobody stores would silently return nothing, which reads
// like "clean" and is not.
func (q FindingQuery) validate() error {
	if q.Layer != "" && !q.Layer.Valid() {
		return fmt.Errorf("%w: unknown layer %q", ErrInvalidQuery, q.Layer)
	}
	switch q.Severity {
	case "", domain.Error, domain.Warning, domain.Waived:
	default:
		return fmt.Errorf("%w: unknown severity %q", ErrInvalidQuery, q.Severity)
	}
	if q.Limit < 0 || q.Limit > MaxFindingLimit {
		return fmt.Errorf("%w: limit must be 1 to %d", ErrInvalidQuery, MaxFindingLimit)
	}
	return nil
}

func (q FindingQuery) limit() int {
	if q.Limit <= 0 {
		return DefaultFindingLimit
	}
	return q.Limit
}
