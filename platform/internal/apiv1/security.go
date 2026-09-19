package apiv1

import (
	"fmt"
	"strings"
)

// Security scheme names in api/openapi.yaml.
const (
	SchemeSession = "session"
	SchemeCSRF    = "csrf"
	SchemeBearer  = "bearer"
)

// Requirement is what one operation demands of its caller, read from the
// contract's `security` so enforcement can't drift from documentation.
type Requirement struct {
	OperationID string
	// Public operations need no credentials.
	Public bool
	// Session accepts the session cookie; CSRF additionally requires the
	// X-CSRF-Token header with it.
	Session, CSRF bool
	// Bearer accepts an API token.
	Bearer bool
}

// Requirements maps each operation's route pattern, exactly as the
// generated router registers it with http.ServeMux ("PATCH
// /v1/tenants/{tenant}/members/{member}", also r.Pattern at request
// time), to its Requirement. It fails for an operation without explicit
// security or with a combination the server can't enforce.
func Requirements() (map[string]Requirement, error) {
	doc, err := GetSwagger()
	if err != nil {
		return nil, fmt.Errorf("apiv1: load embedded spec: %w", err)
	}
	out := map[string]Requirement{}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			pattern := strings.ToUpper(method) + " " + path
			if op.Security == nil {
				return nil, fmt.Errorf("apiv1: %s has no explicit security", pattern)
			}
			req := Requirement{OperationID: op.OperationID, Public: len(*op.Security) == 0}
			for _, alt := range *op.Security {
				if err := req.add(alt); err != nil {
					return nil, fmt.Errorf("apiv1: %s: %w", pattern, err)
				}
			}
			out[pattern] = req
		}
	}
	return out, nil
}

func (r *Requirement) add(alt map[string][]string) error {
	_, session := alt[SchemeSession]
	_, csrf := alt[SchemeCSRF]
	_, bearer := alt[SchemeBearer]
	switch {
	case len(alt) == 0:
		return fmt.Errorf("an empty security alternative makes the operation public; use security: []")
	case bearer && len(alt) == 1:
		r.Bearer = true
	case session && (len(alt) == 1 || csrf && len(alt) == 2):
		r.Session, r.CSRF = true, csrf
	default:
		return fmt.Errorf("unsupported security alternative %v", alt)
	}
	return nil
}
