package domain

import (
	"fmt"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/domain"
)

// Toolset is the set of tools a session registered. It is the *second*
// lock on a write (RFC 0005 §7.2): the token must carry the `write`
// scope, and the client must also have opened the session asking for
// the write toolset. A token is long-lived and an agent is not a
// person; one accidental tool call should not be able to rewrite a
// catalog.
type Toolset string

const (
	// ToolsetRead is the default: a session that can only read.
	ToolsetRead Toolset = "read"
	// ToolsetWrite registers the read tools and the write tools. It is
	// opened explicitly (`?toolset=write`, `--allow-write` on the stdio
	// proxy) and only on a token that carries the write scope.
	ToolsetWrite Toolset = "write"
)

// ParseToolset reads the toolset a client asked for. Empty means read,
// so a client that says nothing gets the safe session.
func ParseToolset(s string) (Toolset, error) {
	switch Toolset(s) {
	case "":
		return ToolsetRead, nil
	case ToolsetRead:
		return ToolsetRead, nil
	case ToolsetWrite:
		return ToolsetWrite, nil
	}
	return "", fmt.Errorf("%w: %q (read or write)", ErrInvalidToolset, s)
}

// String returns the toolset name.
func (t Toolset) String() string { return string(t) }

// Includes reports whether a session with this toolset may call a tool
// belonging to want. A write session includes the read tools; a read
// session includes only its own.
func (t Toolset) Includes(want Toolset) bool {
	return want == ToolsetRead || t == ToolsetWrite
}

// Scope is the token scope a toolset needs. `admin` is never a toolset:
// MCP exposes no member, token, connection or tenant management, and no
// delete tool of any kind (RFC 0005 §7.2).
func (t Toolset) Scope() domain.Scope {
	if t == ToolsetWrite {
		return domain.ScopeWrite
	}
	return domain.ScopeRead
}

// Outcome is how a tool call ended. The vocabulary is closed: it is a
// metric label and an audited column, so a new value is a deliberate
// change in both places.
type Outcome string

const (
	// OutcomeOK is a call that ran and answered.
	OutcomeOK Outcome = "ok"
	// OutcomeDenied is a call refused by authorization or by the
	// session's toolset.
	OutcomeDenied Outcome = "denied"
	// OutcomeInvalid is a call whose arguments were rejected.
	OutcomeInvalid Outcome = "invalid"
	// OutcomeError is a call that failed for any other reason.
	OutcomeError Outcome = "error"
)

// Outcomes lists every outcome, for the metrics adapter's zero-valued
// series and for tests.
func Outcomes() []Outcome { return []Outcome{OutcomeOK, OutcomeDenied, OutcomeInvalid, OutcomeError} }

// String returns the outcome name.
func (o Outcome) String() string { return string(o) }
