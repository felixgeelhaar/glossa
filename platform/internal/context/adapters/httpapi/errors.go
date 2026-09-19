package httpapi

import (
	"errors"
	"net/http"

	"github.com/felixgeelhaar/glossa/platform/internal/context/app"
	"github.com/felixgeelhaar/glossa/platform/internal/context/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/problem"
)

// problems maps Context's errors to the codes documented in
// api/openapi.yaml. A code here is part of the contract: never rename
// one. An empty detail means the error's own text.
var problems = []struct {
	err    error
	status int
	code   problem.Code
	detail string
}{
	{app.ErrProjectNotFound, http.StatusNotFound, problem.CodeNotFound, "no such project"},
	{app.ErrMessageNotFound, http.StatusNotFound, problem.CodeNotFound, "no such message"},
	{app.ErrBuildNotFound, http.StatusNotFound, problem.CodeNotFound, "no such build"},
	{app.ErrNotFound, http.StatusNotFound, problem.CodeNotFound, "no such resource"},
	{app.ErrApplicationNotFound, http.StatusBadRequest, "unknown_application", "the project has no application with that slug"},
	{app.ErrRateLimited, http.StatusTooManyRequests, problem.CodeRateLimited, ""},
	{app.ErrInvalidQuery, http.StatusBadRequest, "invalid_query", ""},
	{domain.ErrUploadTooLarge, http.StatusRequestEntityTooLarge, problem.CodePayloadTooLarge, ""},
	{domain.ErrTooManyUsages, http.StatusBadRequest, "too_many_usages", ""},
	{domain.ErrInvalidUpload, http.StatusBadRequest, "invalid_usages", ""},
	{domain.ErrInvalidSource, http.StatusBadRequest, "invalid_source", ""},
	{domain.ErrInvalidBranch, http.StatusBadRequest, "invalid_branch", ""},
}

// mapError turns Context's errors into problem details; anything else
// (authorization, unexpected failures) passes through.
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
