package sources

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/mcp/tools"
	releaseapp "github.com/felixgeelhaar/glossa/platform/internal/release/app"
	release "github.com/felixgeelhaar/glossa/platform/internal/release/domain"
)

// Releases adapts Release's application service to tools.Releases: the
// three operations of RFC 0005 §7.3, behind the `publish` scope.
//
// Every one of them is the same method the REST endpoint calls, with
// the same arguments, so the environment's policy, the publish gate
// that answers `policy_not_met`, the promotability rules, the
// deployment history a rollback walks and the events the context emits
// are inherited whole. This file translates ids and shapes and does
// nothing else.
type Releases struct{ release *releaseapp.Service }

// NewReleases returns the adapter.
func NewReleases(s *releaseapp.Service) *Releases { return &Releases{release: s} }

var _ tools.Releases = (*Releases)(nil)

// Publish implements tools.Releases.
//
// It passes no force flag because the port has none: a publish the
// environment's policy refuses comes back as the context's own error,
// and overriding it takes a person and a recorded reason.
func (a *Releases) Publish(
	ctx context.Context, project uuid.UUID, in tools.PublishRequest,
) (tools.Published, error) {
	rel, replayed, err := a.release.Publish(ctx, project,
		releaseapp.PublishInput{Environment: in.Environment, Note: in.Note}, in.IdempotencyKey)
	if err != nil {
		return tools.Published{}, notFound(err, releaseNotFound...)
	}
	return tools.Published{Release: releaseOf(rel), Replayed: replayed}, nil
}

// Promote implements tools.Releases.
func (a *Releases) Promote(
	ctx context.Context, project uuid.UUID, environment string, id uuid.UUID,
) (tools.Deployed, error) {
	before := a.serving(ctx, project, environment)
	env, err := a.release.Promote(ctx, project, environment, id)
	if err != nil {
		return tools.Deployed{}, notFound(err, releaseNotFound...)
	}
	return a.deployed(ctx, project, env, before)
}

// Rollback implements tools.Releases. uuid.Nil is "the release before
// the one served now", which is what the context's own nil pointer
// means.
func (a *Releases) Rollback(
	ctx context.Context, project uuid.UUID, environment string, id uuid.UUID,
) (tools.Deployed, error) {
	var want *uuid.UUID
	if id != uuid.Nil {
		want = &id
	}
	before := a.serving(ctx, project, environment)
	env, err := a.release.Rollback(ctx, project, environment, want)
	if err != nil {
		return tools.Deployed{}, notFound(err, releaseNotFound...)
	}
	return a.deployed(ctx, project, env, before)
}

// serving is the release the environment served before the call, so the
// answer can say whether the pointer actually moved. An environment
// that cannot be read yields uuid.Nil and the move is reported as one:
// the pointer operation itself will fail or succeed on its own terms,
// and this read must never be the reason a publish-scoped call is
// refused.
func (a *Releases) serving(ctx context.Context, project uuid.UUID, environment string) uuid.UUID {
	env, err := a.release.GetEnvironment(ctx, project, environment)
	if err != nil {
		return uuid.Nil
	}
	return env.Current
}

// deployed reads back the release the environment now serves. The
// pointer methods answer with the environment, and what an agent needs
// to know is which release that is — including for a rollback, where
// the context chose the release and the caller did not.
//
// Moved compares the pointer with what it was: promoting the release an
// environment already serves changes nothing and is not an error, and
// an agent told "promoted" about a no-op would believe it had changed
// production.
func (a *Releases) deployed(
	ctx context.Context, project uuid.UUID, env release.Environment, before uuid.UUID,
) (tools.Deployed, error) {
	if env.Current == uuid.Nil {
		return tools.Deployed{}, tools.ErrNotFound
	}
	rel, err := a.release.GetRelease(ctx, project, env.Current)
	if err != nil {
		return tools.Deployed{}, notFound(err, releaseNotFound...)
	}
	return tools.Deployed{Environment: env.Name, Release: releaseOf(rel), Moved: env.Current != before}, nil
}

// releaseOf renders a release for a tool's payload.
func releaseOf(rel release.Release) tools.Release {
	out := tools.Release{
		ID: rel.ID.String(), Version: rel.Version, Environment: rel.Environment,
		Digest: rel.Digest, Policy: rel.Policy.States, Branch: rel.Branch,
		Messages: rel.Stats.Messages, Note: rel.Note, Author: rel.Author,
		CreatedAt: rel.CreatedAt.UTC().Format(time.RFC3339),
	}
	for _, l := range rel.Content.Locales {
		out.Locales = append(out.Locales, l.Code)
	}
	return out
}
