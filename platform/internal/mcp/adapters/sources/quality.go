package sources

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/mcp/app"
	"github.com/felixgeelhaar/glossa/platform/internal/mcp/tools"
	qualityapp "github.com/felixgeelhaar/glossa/platform/internal/quality/app"
	quality "github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
)

// Quality adapts Quality's application service to tools.Quality. It
// reads the stored run through the context's own service, never over
// HTTP: MCP is a second façade on the ports (RFC 0005 §7.1).
type Quality struct{ quality *qualityapp.Service }

// NewQuality returns the adapter.
func NewQuality(s *qualityapp.Service) *Quality { return &Quality{quality: s} }

var _ tools.Quality = (*Quality)(nil)

// Findings implements tools.Quality.
func (a *Quality) Findings(
	ctx context.Context, project uuid.UUID, q tools.FindingsQuery,
) (tools.CheckRun, []tools.Finding, string, error) {
	page, err := a.quality.ListFindings(ctx, project, qualityapp.FindingQuery{
		Ref: q.Ref,
		FindingFilter: qualityapp.FindingFilter{
			Layer: quality.Layer(q.Layer), Locale: q.Locale, Severity: quality.Severity(q.Severity),
			MessageKey: q.MessageKey, WaivedOnly: q.WaivedOnly,
		},
		Limit: q.Limit, After: q.After,
	})
	if err != nil {
		if errors.Is(err, qualityapp.ErrInvalidQuery) {
			return tools.CheckRun{}, nil, "", &app.InvalidArgumentError{
				Argument: "arguments", Reason: "the filter is not one Quality accepts",
			}
		}
		return tools.CheckRun{}, nil, "", notFound(err, qualityapp.ErrNotFound)
	}
	run := tools.CheckRun{
		ID: page.Run.ID.String(), Ref: page.Run.Ref, Trigger: string(page.Run.Trigger),
		Conclusion: string(page.Run.Conclusion), PolicyVersion: page.Run.PolicyVersion,
		Errors: page.Run.Counts.Errors, Warnings: page.Run.Counts.Warnings, Waived: page.Run.Counts.Waived,
		StartedAt: page.Run.StartedAt.UTC().Format(time.RFC3339),
	}
	for _, l := range page.Run.Layers {
		run.Layers = append(run.Layers, string(l))
	}
	if page.Run.CompletedAt != nil {
		run.CompletedAt = page.Run.CompletedAt.UTC().Format(time.RFC3339)
	}
	out := make([]tools.Finding, len(page.Findings))
	for i, f := range page.Findings {
		out[i] = tools.Finding{
			ID: f.ID.String(), Fingerprint: f.Fingerprint, Layer: string(f.Layer), Code: f.Code,
			Severity: string(f.Severity), Key: f.Locus.Key, Locale: f.Locus.Locale,
			Namespace: f.Locus.Namespace, File: f.Locus.File, Line: f.Locus.Line,
			Explanation: f.Message, Subject: f.Subject, Waiver: f.Waiver,
		}
	}
	return run, out, page.Next, nil
}
