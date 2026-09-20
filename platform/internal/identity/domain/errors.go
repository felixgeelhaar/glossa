package domain

import "errors"

// Domain errors. The HTTP adapter maps each to a problem code.
var (
	ErrInvalidLocale         = errors.New("identity: invalid locale")
	ErrInvalidRole           = errors.New("identity: invalid role")
	ErrNoRoles               = errors.New("identity: a member needs at least one role")
	ErrLocalesNeedLocaleRole = errors.New("identity: locale scopes apply only to translators and reviewers")
	ErrInvalidScope          = errors.New("identity: invalid token scope")
	ErrInvalidDisplayName    = errors.New("identity: invalid display name")
	ErrInvalidID             = errors.New("identity: invalid id")

	ErrIndividualTenant     = errors.New("identity: an individual tenant has exactly one member")
	ErrOwnerChangeForbidden = errors.New("identity: only an owner may grant, change or remove the owner role")
	ErrLastOwner            = errors.New("identity: a tenant must keep at least one owner")
	ErrMemberNotInvited     = errors.New("identity: member is not an open invitation")

	ErrInvalidTokenSecret = errors.New("identity: malformed API token")
	ErrInvalidTokenName   = errors.New("identity: token name must be 1-100 characters")
	ErrInvalidTokenExpiry = errors.New("identity: token expiry must be in the future")
	ErrTokenRevoked       = errors.New("identity: token revoked")
	ErrTokenExpired       = errors.New("identity: token expired")
	ErrScopeExceedsGrant  = errors.New("identity: token scopes exceed the creator's permissions")

	ErrInvalidPermission = errors.New("identity: unknown permission")

	// In-context grants and the preview origins they are bound to
	// (RFC 0004 §5.2).
	ErrInvalidOrigin          = errors.New("identity: invalid origin")
	ErrInvalidOriginLabel     = errors.New("identity: origin label must be at most 100 characters")
	ErrOriginRegistered       = errors.New("identity: the origin is already registered for this project")
	ErrTooManyPreviewOrigins  = errors.New("identity: the project has as many preview origins as it may have")
	ErrOriginNotRegistered    = errors.New("identity: the origin is not a registered preview origin of this project")
	ErrNoInContextPermissions = errors.New("identity: you may not edit this project's text in context")
	ErrPersonGrantOnly        = errors.New("identity: only a signed-in person mints an in-context grant")
	ErrOriginNotBound         = errors.New("identity: the grant was minted for another origin")
	ErrGrantProjectMismatch   = errors.New("identity: the grant was minted for another project")

	// The GitHub Actions OIDC exchange (RFC 0004 §6.3).
	ErrInvalidWorkflowRun = errors.New("identity: the ID token does not name a usable workflow run")
	// ErrInvalidIDToken covers every way a presented ID token fails to
	// verify — issuer, audience, signature, expiry, shape. The caller
	// is told one thing, because telling it which would help it guess.
	ErrInvalidIDToken = errors.New("identity: the GitHub Actions ID token did not verify")
	// ErrRepositoryNotConnected means no Git connection names that
	// repository, so there is no project to mint a token for.
	ErrRepositoryNotConnected = errors.New("identity: that repository is not connected to a Glossa project")
	// ErrAmbiguousProject means the repository feeds several projects
	// and the caller named none of them.
	ErrAmbiguousProject = errors.New("identity: that repository feeds several projects; name the one to authenticate for")
	// ErrGitHubOIDCUnavailable means this deployment has no GitHub App,
	// so there is nothing to exchange an ID token against.
	ErrGitHubOIDCUnavailable = errors.New("identity: this deployment has no GitHub App configured")
)
