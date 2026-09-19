package httpapi

import (
	"errors"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/idempotency"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/problem"
	"github.com/felixgeelhaar/glossa/platform/internal/release/app"
	"github.com/felixgeelhaar/glossa/platform/internal/release/domain"
)

// problems maps Release's errors to the codes documented in
// api/openapi.yaml. A code here is part of the contract: never rename
// one. An empty detail means the error's own text.
var problems = []struct {
	err    error
	status int
	code   problem.Code
	detail string
}{
	{app.ErrNotFound, 404, problem.CodeNotFound, "no such resource"},
	{app.ErrReleaseNotInProject, 404, "release_not_found", "the project has no such release"},
	{app.ErrEnvironmentExists, 409, "environment_exists", "the project already has an environment with that name"},
	{app.ErrStaleVersion, 409, problem.CodeConflict, "the resource changed concurrently; retry"},
	{domain.ErrIneligible, 409, "release_ineligible", ""},
	{domain.ErrNoRollbackTarget, 409, "no_rollback_target", ""},
	{domain.ErrNotInHistory, 409, "not_in_history", ""},
	{domain.ErrKeyRevoked, 409, "key_revoked", ""},
	{app.ErrPreconditionFailed, 412, problem.CodePreconditionFailed, "the resource changed; fetch it and retry with its new ETag"},
	{app.ErrIdempotencyReuse, 422, "idempotency_key_reused", "this Idempotency-Key was used for a different request"},
	{domain.ErrNotReleasable, 422, "not_releasable", ""},
	{app.ErrStorage, 503, "storage_unavailable", "artifact storage is unavailable; retry"},
	{app.ErrInvalidPageToken, 400, "invalid_page_token", "page_token is not one this list issued"},
	{idempotency.ErrInvalidKey, 400, "invalid_idempotency_key", ""},
	{domain.ErrInvalidEnvironment, 400, "invalid_environment", ""},
	{domain.ErrInvalidPolicy, 400, "invalid_policy", ""},
	{domain.ErrInvalidNote, 400, "invalid_note", ""},
	{domain.ErrInvalidKeyName, 400, "invalid_key_name", ""},
}

// mapError turns Release's errors into problem details; anything else
// (authorization, unexpected failures) passes through to the shared
// error hook.
func mapError(err error) error {
	for _, p := range problems {
		if errors.Is(err, p.err) {
			detail := p.detail
			if detail == "" {
				detail = err.Error()
			}
			return problem.New(p.status, p.code, detail)
		}
	}
	return err
}
