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
)
