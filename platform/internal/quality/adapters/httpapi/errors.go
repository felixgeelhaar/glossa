package httpapi

import (
	"errors"
	"net/http"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/checkpolicy"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/problem"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/app"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
)

// problems maps Quality's errors to the codes documented in
// api/openapi.yaml. A code here is part of the contract: never rename
// one. An empty detail means the error's own text.
var problems = []struct {
	err    error
	status int
	code   problem.Code
	detail string
}{
	{app.ErrProjectNotFound, http.StatusNotFound, problem.CodeNotFound, "no such project"},
	{app.ErrCheckRunNotFound, http.StatusNotFound, problem.CodeNotFound, "no such check run"},
	{app.ErrWaiverNotFound, http.StatusNotFound, problem.CodeNotFound, "no such waiver"},
	{app.ErrCaptureNotFound, http.StatusNotFound, problem.CodeNotFound, "no such capture"},
	{app.ErrNotFound, http.StatusNotFound, problem.CodeNotFound, "no such resource"},
	{app.ErrInvalidQuery, http.StatusBadRequest, "invalid_query", ""},
	{app.ErrTooManyFindings, http.StatusBadRequest, "too_many_findings", ""},
	{app.ErrPreGradedFinding, http.StatusBadRequest, "invalid_finding", ""},
	// A waiver without a reason is a 400 and not a silently accepted
	// blank: the reason is the whole mechanism (RFC 0005 §2.3).
	{domain.ErrReasonRequired, http.StatusBadRequest, "waiver_reason_required",
		"a waiver needs a reason: say why this finding is fine"},
	{domain.ErrReasonTooLong, http.StatusBadRequest, "invalid_waiver", ""},
	{domain.ErrInvalidFingerprint, http.StatusBadRequest, "invalid_fingerprint", ""},
	{domain.ErrInvalidScope, http.StatusBadRequest, "invalid_waiver_scope", ""},
	{domain.ErrBranchRequired, http.StatusBadRequest, "waiver_branch_required", ""},
	{domain.ErrExpiryInThePast, http.StatusBadRequest, "waiver_expiry_in_the_past", ""},
	{app.ErrPolicyVersionNotFound, http.StatusNotFound, problem.CodeNotFound, "no such check-policy version"},
	// A write that lost the race is refused rather than allowed to drop
	// what the winner said: the caller reads the policy again and
	// decides (RFC 0005 §4.3).
	{app.ErrPolicyConflict, http.StatusConflict, "check_policy_conflict", ""},
	// Everything a policy document can be wrong about is one code. The
	// detail says which — an unknown layer, a severity that is not one,
	// a rule raising an opinion to an error, a locale the project does
	// not have — because a caller fixing a policy needs the sentence,
	// not a taxonomy of codes to branch on.
	{domain.ErrPolicyBookkeeping, http.StatusBadRequest, "invalid_check_policy", ""},
	{domain.ErrInvalidGrace, http.StatusBadRequest, "invalid_check_policy", ""},
	{checkpolicy.ErrInvalidDocument, http.StatusBadRequest, "invalid_check_policy", ""},
	{checkpolicy.ErrInvalidSeverity, http.StatusBadRequest, "invalid_check_policy", ""},
	{checkpolicy.ErrInvalidMode, http.StatusBadRequest, "invalid_check_policy", ""},
	{checkpolicy.ErrUnknownLayer, http.StatusBadRequest, "invalid_check_policy", ""},
	{checkpolicy.ErrAdvisoryLayer, http.StatusBadRequest, "invalid_check_policy", ""},
	{checkpolicy.ErrUnknownLocale, http.StatusBadRequest, "invalid_check_policy", ""},
	{checkpolicy.ErrInvalidEnvironment, http.StatusBadRequest, "invalid_check_policy", ""},
}

// invalidPolicy is a policy document the edge itself can refuse,
// before the domain sees it: a require_complete that names none of the
// three modes.
func invalidPolicy(detail string) error {
	return problem.New(http.StatusBadRequest, "invalid_check_policy", detail)
}

// mapError turns Quality's errors into problem details; anything else
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
