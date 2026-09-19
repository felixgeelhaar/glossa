package app

import (
	"context"
	"errors"
	"time"

	authgo "github.com/klarlabs-studio/auth-go/domain"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

// Store errors. Adapters translate their storage errors into these.
var (
	ErrNotFound     = errors.New("identity: not found")
	ErrEmailTaken   = errors.New("identity: email already registered")
	ErrSlugTaken    = errors.New("identity: slug taken")
	ErrDuplicate    = errors.New("identity: already a member")
	ErrStaleVersion = errors.New("identity: version changed")
)

// Transactor runs units of work in the kernel's two scopes. Calls don't
// nest: never call the Transactor, or an auth-go service backed by it,
// from inside fn.
type Transactor interface {
	// InSystem runs fn in system scope. ctx must not carry a tenant.
	InSystem(ctx context.Context, fn func(context.Context, SystemStore) error) error
	// InTenant runs fn scoped to the tenant on ctx.
	InTenant(ctx context.Context, fn func(context.Context, TenantStore) error) error
}

// PersonRecord is a person as stored, with the credential facts the
// sign-in flows need.
type PersonRecord struct {
	domain.Person
	PasswordHash *authgo.PasswordHash
	TOTPEnabled  bool
}

// MembershipGrant is a person's active membership in one tenant.
type MembershipGrant struct {
	Member  domain.MemberID
	Roles   domain.Roles
	Locales domain.LocaleScope
}

// MembershipView is one of a person's tenants, for GET /v1/me.
type MembershipView struct {
	Member  domain.MemberID
	Tenant  tenancy.Tenant
	Roles   domain.Roles
	Locales domain.LocaleScope
}

// TokenRecord is what bearer authentication needs about a token.
type TokenRecord struct {
	ID         domain.TokenID
	Tenant     tenancy.ID
	Scopes     domain.Scopes
	ExpiresAt  *time.Time
	RevokedAt  *time.Time
	LastUsedAt *time.Time
}

// TOTPRecord is a person's stored authenticator secret.
type TOTPRecord struct {
	Confirmed bool
}

// SystemStore is persistence in system scope: global data, and the
// narrow cross-tenant lookups that happen before a tenant is chosen.
type SystemStore interface {
	CreatePerson(ctx context.Context, p domain.Person, password *authgo.PasswordHash) error
	PersonByEmail(ctx context.Context, email authgo.Email) (PersonRecord, error)
	PersonByID(ctx context.Context, id domain.PersonID) (PersonRecord, error)
	MarkEmailVerified(ctx context.Context, id domain.PersonID, at time.Time) error
	SetPassword(ctx context.Context, id domain.PersonID, hash authgo.PasswordHash, at time.Time) error

	ActiveMembership(ctx context.Context, person domain.PersonID, tenant tenancy.ID) (MembershipGrant, error)
	MembershipsOf(ctx context.Context, person domain.PersonID, after tenancy.ID, limit int) ([]MembershipView, error)
	OpenInvitations(ctx context.Context, email authgo.Email) ([]InvitationRef, error)
	Tenant(ctx context.Context, id tenancy.ID) (tenancy.Tenant, error)

	TokenByHash(ctx context.Context, hash string) (TokenRecord, error)
	TouchToken(ctx context.Context, id domain.TokenID, at time.Time) error

	PendingTOTP(ctx context.Context, person domain.PersonID, secret authgo.TOTPSecret, at time.Time) error
	TOTP(ctx context.Context, person domain.PersonID) (TOTPRecord, error)
	ConfirmTOTP(ctx context.Context, person domain.PersonID, at time.Time) error
	DeleteTOTP(ctx context.Context, person domain.PersonID) error

	AddPasskey(ctx context.Context, person domain.PersonID, c authgo.PasskeyCredential, at time.Time) error
	PersonOfPasskey(ctx context.Context, credentialID []byte) (domain.PersonID, error)
	// PasskeysOf lists a person's passkeys after the cursor, oldest first.
	PasskeysOf(ctx context.Context, person domain.PersonID, after PasskeyCursor, limit int) ([]Passkey, error)
	// DeletePasskeyOf removes the person's passkey (ErrNotFound if they
	// have no such passkey).
	DeletePasskeyOf(ctx context.Context, person domain.PersonID, credentialID []byte) error

	SaveCeremony(ctx context.Context, c Ceremony) error
	// TakeCeremony deletes and returns a ceremony (ErrNotFound if absent).
	TakeCeremony(ctx context.Context, keyHash, purpose string) (Ceremony, error)
}

// Ceremony is WebAuthn ceremony state held server-side between the
// challenge and the response, keyed by the hash of a random key the
// browser keeps in a cookie.
type Ceremony struct {
	KeyHash   string
	Purpose   string
	Person    domain.PersonID // zero for sign-in
	State     []byte
	ExpiresAt time.Time
}

// InvitationRef locates an open invitation.
type InvitationRef struct {
	Member domain.MemberID
	Tenant tenancy.ID
}

// MemberView is a member plus the display name of the person, if any.
type MemberView struct {
	domain.Member
	DisplayName string
}

// TenantStore is persistence scoped to the context's tenant. Row-level
// security, not these methods, keeps it inside that tenant.
type TenantStore interface {
	CreateTenant(ctx context.Context, t tenancy.Tenant) (tenancy.Tenant, error)
	CurrentTenant(ctx context.Context) (tenancy.Tenant, error)

	// InsertMember stores m; inserted is false when a member with m.ID
	// already exists (an idempotent retry).
	InsertMember(ctx context.Context, m domain.Member, by domain.Actor) (inserted bool, err error)
	Member(ctx context.Context, id domain.MemberID) (MemberView, error)
	// LockMember loads m for update.
	LockMember(ctx context.Context, id domain.MemberID) (domain.Member, error)
	// LockActiveOwners locks and counts the tenant's active owners.
	LockActiveOwners(ctx context.Context) (int, error)
	Members(ctx context.Context, after domain.MemberID, limit int) ([]MemberView, error)
	// UpdateMemberAccess saves roles, locales and version; it fails with
	// ErrStaleVersion unless the stored version is m.Version-1.
	UpdateMemberAccess(ctx context.Context, m domain.Member) error
	ActivateMember(ctx context.Context, m domain.Member) error
	DeleteMember(ctx context.Context, id domain.MemberID) error

	InsertToken(ctx context.Context, t domain.APIToken) (inserted bool, err error)
	Token(ctx context.Context, id domain.TokenID) (domain.APIToken, error)
	Tokens(ctx context.Context, after domain.TokenID, limit int) ([]domain.APIToken, error)
	RevokeToken(ctx context.Context, t domain.APIToken, by domain.Actor) error

	Publish(ctx context.Context, e outbox.Event) error
}

// Message is an email to send.
type Message struct {
	To      string
	Subject string
	Text    string
}

// Mailer delivers email. Adapters: a log mailer for development, SMTP.
type Mailer interface {
	Send(ctx context.Context, m Message) error
}

// PasskeyStore is the passkey repository auth-go's WebAuthn adapter reads.
type PasskeyStore = authgo.PasskeyRepository
