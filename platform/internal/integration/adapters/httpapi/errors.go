package httpapi

import (
	"net/http"

	"errors"

	"github.com/felixgeelhaar/glossa/platform/internal/integration/app"
	"github.com/felixgeelhaar/glossa/platform/internal/integration/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/idempotency"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/problem"
)

// problems maps Integration's errors to the codes documented in
// api/openapi.yaml. A code here is part of the contract: never rename
// one. An empty detail means the error's own text.
var problems = []struct {
	err    error
	status int
	code   problem.Code
	detail string
}{
	{app.ErrNotFound, http.StatusNotFound, problem.CodeNotFound, "no such job"},
	{app.ErrProjectNotFound, http.StatusNotFound, problem.CodeNotFound, "no such project"},
	{app.ErrLocaleNotFound, http.StatusNotFound, "locale_not_found", ""},
	{app.ErrIdempotencyReuse, http.StatusUnprocessableEntity, "idempotency_key_reused", "this Idempotency-Key was used for a different request"},
	{app.ErrUploadTooLarge, http.StatusRequestEntityTooLarge, "file_too_large", ""},
	{app.ErrEmptyUpload, http.StatusBadRequest, "empty_file", ""},
	{app.ErrUploadInterrupted, http.StatusBadRequest, "upload_interrupted", ""},
	{app.ErrStorage, http.StatusServiceUnavailable, "storage_unavailable", "file storage is unavailable; retry"},
	{app.ErrInvalidQuery, http.StatusBadRequest, "invalid_query", ""},
	{domain.ErrInvalidFormat, http.StatusBadRequest, "invalid_format", ""},
	{domain.ErrNotExportable, http.StatusBadRequest, "invalid_format", "gettext PO is import only"},
	{domain.ErrInvalidMode, http.StatusBadRequest, "invalid_mode", ""},
	{domain.ErrInvalidOptions, http.StatusBadRequest, "invalid_options", ""},
	{domain.ErrProjectRequired, http.StatusBadRequest, "project_required", ""},
	{domain.ErrNotCancellable, http.StatusConflict, "job_not_cancellable", ""},
	{domain.ErrUploadNotExpected, http.StatusConflict, "upload_not_expected", ""},
	{domain.ErrNotReady, http.StatusConflict, "export_not_ready", ""},
	{domain.ErrFileExpired, http.StatusGone, "file_expired", ""},
	{idempotency.ErrInvalidKey, http.StatusBadRequest, "invalid_idempotency_key", ""},
}

// mapError turns Integration's errors into problem details; anything
// else (authorization, problems already, unexpected failures) passes
// through.
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
