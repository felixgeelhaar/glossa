package sources

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	mf "github.com/felixgeelhaar/glossa/messageformat"

	catalogapp "github.com/felixgeelhaar/glossa/platform/internal/catalog/app"
	catalogdomain "github.com/felixgeelhaar/glossa/platform/internal/catalog/domain"
	contextapp "github.com/felixgeelhaar/glossa/platform/internal/context/app"
	"github.com/felixgeelhaar/glossa/platform/internal/integration/app"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/bcp47"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/checkpolicy"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/pagination"
	knowledgeapp "github.com/felixgeelhaar/glossa/platform/internal/knowledge/app"
	knowledgedomain "github.com/felixgeelhaar/glossa/platform/internal/knowledge/domain"
	localizationapp "github.com/felixgeelhaar/glossa/platform/internal/localization/app"
	releaseapp "github.com/felixgeelhaar/glossa/platform/internal/release/app"
	releasedelivery "github.com/felixgeelhaar/glossa/platform/internal/release/delivery"
)

// Checks implements app.CheckSources: the read model the Glossa PR
// check renders from (RFC 0004 §6.4).
//
// It reaches the other bounded contexts through their application
// services, as RFC 0002 §4 requires, so each keeps checking the
// caller's permissions — the check worker's background principal holds
// exactly catalog.read, translations.read, knowledge.read and
// releases.read, and a project it may not read answers accordingly.
type Checks struct {
	catalog      *catalogapp.Service
	localization *localizationapp.Service
	knowledge    *knowledgeapp.Service
	usages       *contextapp.Service
	release      *releaseapp.Service
	// edgeURL is GLOSSA_EDGE_PUBLIC_URL; without one the comment names
	// no manifest, because there is no address to give.
	edgeURL string
}

// ChecksDeps are the services the check reads.
type ChecksDeps struct {
	Catalog      *catalogapp.Service
	Localization *localizationapp.Service
	Knowledge    *knowledgeapp.Service
	Usages       *contextapp.Service
	Release      *releaseapp.Service
	EdgeURL      string
}

// NewChecks returns the check's read model.
func NewChecks(d ChecksDeps) *Checks {
	return &Checks{
		catalog: d.Catalog, localization: d.Localization, knowledge: d.Knowledge,
		usages: d.Usages, release: d.Release, edgeURL: strings.TrimRight(d.EdgeURL, "/"),
	}
}

var _ app.CheckSources = (*Checks)(nil)

// Policy implements app.CheckSources.
//
// The policy is `glossa check`'s, shared through kernel/checkpolicy so
// the pull request and the terminal never disagree. Its zero value is
// the command's own default with no `glossa.yaml`: every locale must be
// complete, and errors fail. A project that wants another one will
// store it here — this is the seam, and it is the only one, because a
// second policy is the failure mode worth avoiding.
func (c *Checks) Policy(context.Context, uuid.UUID) (checkpolicy.Policy, error) {
	return checkpolicy.Policy{}, nil
}

// BranchStatus implements app.CheckSources.
func (c *Checks) BranchStatus(ctx context.Context, project uuid.UUID, branch string) (app.BranchStatus, error) {
	rep, err := c.catalog.BranchStatus(ctx, catalogdomain.ProjectID(project), branch)
	if errors.Is(err, catalogapp.ErrNotFound) {
		// CI has not pushed this branch yet: an empty status, not a
		// failure. The check then simply waits.
		return app.BranchStatus{Name: branch, Outdated: map[string]int{}}, nil
	}
	if err != nil {
		return app.BranchStatus{}, err
	}
	out := app.BranchStatus{
		Name: string(rep.Branch.Name), PR: rep.Branch.PR, HeadCommit: rep.Branch.HeadCommit,
		State: string(rep.Branch.State), PreviewURL: rep.Branch.PreviewURL,
		NewKeys: keys(rep.NewKeys), SourceProposals: keys(rep.SourceProposals), Removed: keys(rep.Removed),
		Outdated: rep.Outdated,
	}
	for _, m := range rep.Invalid {
		out.Invalid = append(out.Invalid, app.InvalidMessage{Key: m.Key, Code: m.Code, Detail: m.Detail})
	}
	for _, k := range rep.Conflicts {
		conflict := app.KeyConflict{Key: string(k.Key)}
		for _, b := range k.Branches {
			conflict.Branches = append(conflict.Branches, string(b))
		}
		out.Conflicts = append(out.Conflicts, conflict)
	}
	if out.Outdated == nil {
		out.Outdated = map[string]int{}
	}
	return out, nil
}

func keys(ks []catalogdomain.MessageKey) []string {
	out := make([]string, 0, len(ks))
	for _, k := range ks {
		out = append(out, string(k))
	}
	return out
}

// maxQAPages bounds the QA scan. A pull request's check is a report,
// not an audit: the first few hundred translations of the project's
// locales are enough to say what is wrong on the branch, and the
// terminology layer has its own page limit for the same reason.
const maxQAPages = 5

// BranchQuality implements app.CheckSources.
func (c *Checks) BranchQuality(ctx context.Context, project uuid.UUID, branch string, keys []string) (app.BranchQuality, error) {
	out := app.BranchQuality{Untranslated: map[string]int{}}
	locales, err := c.locales(ctx, project)
	if err != nil {
		return out, err
	}
	out.Locales = locales
	ids, err := c.newKeyMessages(ctx, project, branch)
	if err != nil {
		return out, err
	}
	if len(ids) > 0 {
		// CurrentTranslations counts the usable translations of those
		// messages per locale — counts, not text — so what is left is
		// what nobody has written yet.
		counts, err := c.localization.CurrentTranslations(ctx, project, ids)
		if err != nil {
			return out, err
		}
		for _, l := range locales {
			out.Untranslated[l] = len(ids) - counts[l]
		}
	}
	if out.Findings, err = c.qaFindings(ctx, project, locales, keys); err != nil {
		return out, err
	}
	return out, nil
}

// locales lists the project's target locales.
func (c *Checks) locales(ctx context.Context, project uuid.UUID) ([]string, error) {
	tags, err := (&Localization{svc: c.localization}).Locales(ctx, project)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		out = append(out, t.String())
	}
	return out, nil
}

// newKeyMessages are the messages the branch proposes as new keys.
func (c *Checks) newKeyMessages(ctx context.Context, project uuid.UUID, branch string) ([]uuid.UUID, error) {
	o, err := c.catalog.BranchOverlay(ctx, catalogdomain.ProjectID(project), branch)
	if errors.Is(err, catalogapp.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []uuid.UUID
	for _, p := range o.Proposals {
		if p.Kind == catalogdomain.ProposalNewKey {
			out = append(out, p.MessageID.UUID())
		}
	}
	return out, nil
}

// qaFindings collects the QA the server already holds on the branch's
// messages: the structural warnings Localization stored with each text
// (max_length among them) and the terminology findings of M2.
func (c *Checks) qaFindings(ctx context.Context, project uuid.UUID, locales, branchKeys []string) ([]app.CheckFinding, error) {
	if len(locales) == 0 || len(branchKeys) == 0 {
		return nil, nil
	}
	wanted := map[string]bool{}
	for _, k := range branchKeys {
		wanted[k] = true
	}
	out, err := c.storedWarnings(ctx, project, locales, wanted)
	if err != nil {
		return nil, err
	}
	terms, err := c.terminology(ctx, project, locales, wanted)
	if err != nil {
		return nil, err
	}
	return append(out, terms...), nil
}

// storedWarnings reads the structural QA Localization kept with each
// translation when it was written (max-length-exceeded and the compat
// warnings), for the branch's own keys.
func (c *Checks) storedWarnings(ctx context.Context, project uuid.UUID, locales []string, wanted map[string]bool) ([]app.CheckFinding, error) {
	var out []app.CheckFinding
	page := pagination.Page{Size: pagination.MaxPageSize}
	for range maxQAPages {
		rows, next, err := c.localization.ListProjectTranslations(ctx, project,
			localizationapp.TranslationFilter{Locales: locales}, page)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			if !wanted[r.Key] {
				continue
			}
			for _, w := range r.Warnings {
				out = append(out, app.CheckFinding{
					Code: string(w.Code), Severity: severity(w.Severity), Locale: r.Locale.String(),
					Key: r.Key, Message: w.Message,
				})
			}
		}
		if next == nil {
			return out, nil
		}
		if page, err = pagination.Parse(&page.Size, next); err != nil {
			return out, err
		}
	}
	return out, nil
}

func severity(s mf.Severity) checkpolicy.Severity {
	if s == mf.SeverityError {
		return checkpolicy.Error
	}
	return checkpolicy.Warning
}

// terminology asks Knowledge for the branch's terminology findings.
func (c *Checks) terminology(ctx context.Context, project uuid.UUID, locales []string, wanted map[string]bool) ([]app.CheckFinding, error) {
	if c.knowledge == nil {
		return nil, nil
	}
	tags := make([]bcp47.Tag, 0, len(locales))
	for _, l := range locales {
		t, err := bcp47.Parse(l)
		if err != nil {
			continue
		}
		tags = append(tags, t)
	}
	if len(tags) == 0 {
		return nil, nil
	}
	var out []app.CheckFinding
	page := pagination.Page{Size: pagination.MaxPageSize}
	for range maxQAPages {
		rep, err := c.knowledge.CheckProjectTerminology(ctx, project,
			knowledgeapp.ProjectTermCheck{Locales: tags, Page: page})
		if err != nil {
			return nil, err
		}
		for _, item := range rep.Items {
			if !wanted[item.Key] {
				continue
			}
			for _, f := range item.Findings {
				out = append(out, app.CheckFinding{
					Code: string(f.Code), Severity: termSeverity(f.Severity), Locale: item.Locale.String(),
					Key: item.Key, Message: f.Message,
				})
			}
		}
		if rep.Next == nil {
			return out, nil
		}
		if page, err = pagination.Parse(&page.Size, rep.Next); err != nil {
			return out, err
		}
	}
	return out, nil
}

func termSeverity(s knowledgedomain.Severity) checkpolicy.Severity {
	if s == knowledgedomain.SeverityError {
		return checkpolicy.Error
	}
	return checkpolicy.Warning
}

// maxUnknownPages bounds the unknown-key scan. The query already keeps
// only the unknown ones, so this is a cap on a pathological branch, not
// on the report.
const maxUnknownPages = 5

// BranchUsages implements app.CheckSources.
func (c *Checks) BranchUsages(ctx context.Context, project uuid.UUID, branch string) (app.BranchUsages, error) {
	var out app.BranchUsages
	builds, err := c.usages.CurrentBuilds(ctx, project, branch)
	if err != nil {
		return out, err
	}
	out.Builds = len(builds)
	for _, b := range builds {
		out.Commits = append(out.Commits, string(b.Commit))
	}
	seen := map[string]bool{}
	page := pagination.Page{Size: pagination.MaxPageSize}
	for range maxUnknownPages {
		p, err := c.usages.ListUsages(ctx, project, branch, contextapp.UsageFilter{Unknown: true}, page)
		if err != nil {
			return out, err
		}
		for _, u := range p.Usages {
			if seen[u.Key] {
				continue
			}
			seen[u.Key] = true
			out.Unknown = append(out.Unknown, app.UnknownKey{Key: u.Key, File: u.File, Line: u.Line})
		}
		if p.Next == nil {
			break
		}
		if page, err = pagination.Parse(&page.Size, p.Next); err != nil {
			return out, err
		}
	}
	cov, err := c.usages.CaptureCoverage(ctx, project, branch)
	if err != nil {
		return out, err
	}
	out.Captured, out.NotCaptured = cov.Captured, cov.NotCaptured()
	return out, nil
}

// ManifestURL implements app.CheckSources: where the branch
// environment's manifest is served at the edge.
//
// It needs three things to exist — the environment, a delivery key that
// may read branch environments, and an announced edge — and answers ""
// rather than an error when any is missing: a comment with one link
// fewer is not a failed check.
func (c *Checks) ManifestURL(ctx context.Context, project uuid.UUID, branch string, pr int) (string, error) {
	if c.release == nil || c.edgeURL == "" {
		return "", nil
	}
	name := releasedelivery.BranchEnvironmentName(branch, pr)
	if _, err := c.release.GetEnvironment(ctx, project, name); err != nil {
		if errors.Is(err, releaseapp.ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	keys, _, err := c.release.ListDeliveryKeys(ctx, project, pagination.Page{Size: pagination.MaxPageSize})
	if err != nil {
		return "", err
	}
	for _, k := range keys {
		if k.RevokedAt == nil && k.Scope.Allows(name) {
			return fmt.Sprintf("%s/v1/%s/%s/manifest.json", c.edgeURL, k.Key, name), nil
		}
	}
	return "", nil
}
