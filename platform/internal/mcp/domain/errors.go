package domain

import "errors"

var (
	// ErrUnauthenticated means the request carried no usable tenant API
	// token.
	ErrUnauthenticated = errors.New("mcp: unauthenticated")
	// ErrCredentialNotAccepted means the bearer authenticates elsewhere
	// but is not a credential MCP accepts: a CI token (glossa_ci_…) or
	// an in-context grant (glossa_ctx_…). Each is minted for one job or
	// one origin, and lending either to a long-lived agent session would
	// widen it (RFC 0005 §7.2).
	ErrCredentialNotAccepted = errors.New("mcp: credential not accepted")
	// ErrWriteNotGranted means a write session was asked for on a token
	// that does not carry the write scope.
	ErrWriteNotGranted = errors.New("mcp: the token does not carry the write scope")
	// ErrToolNotInSession means the tool exists but belongs to a toolset
	// this session did not open — the second of the two locks on a write
	// (RFC 0005 §7.2).
	ErrToolNotInSession = errors.New("mcp: the tool is not in this session's toolset")
	// ErrToolNotFound means no tool of that name is registered.
	ErrToolNotFound = errors.New("mcp: no such tool")
	// ErrInvalidToolset means the client asked for a toolset that does
	// not exist.
	ErrInvalidToolset = errors.New("mcp: invalid toolset")
)
