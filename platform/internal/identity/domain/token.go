package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	authgo "github.com/klarlabs-studio/auth-go/domain"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

// TokenPrefix starts every API token. It exists for secret scanners:
// the full pattern is glossa_api_[A-Za-z0-9_-]{43}. A future publishable
// delivery key (M1) gets its own prefix, so a leaked key's kind is
// obvious from the string alone.
const TokenPrefix = "glossa_api_"

const (
	tokenBodyLen = 43 // auth-go's 256-bit token, base64url without padding
	hintBodyLen  = 4
	maxTokenName = 100
)

// TokenSecret is the raw bearer credential. It is shown exactly once, at
// creation; only its hash is stored.
type TokenSecret struct{ v string }

// NewTokenSecret returns a fresh secret: the prefix plus 256 random bits
// from auth-go's token generator.
func NewTokenSecret() (TokenSecret, error) {
	raw, err := authgo.NewToken()
	if err != nil {
		return TokenSecret{}, fmt.Errorf("identity: generate token: %w", err)
	}
	return TokenSecret{v: TokenPrefix + raw.String()}, nil
}

// ParseTokenSecret validates an inbound bearer credential's shape before
// anything is hashed or looked up.
func ParseTokenSecret(s string) (TokenSecret, error) {
	body, ok := strings.CutPrefix(s, TokenPrefix)
	if !ok || len(body) != tokenBodyLen || strings.IndexFunc(body, notBase64URL) >= 0 {
		return TokenSecret{}, ErrInvalidTokenSecret
	}
	return TokenSecret{v: s}, nil
}

func notBase64URL(r rune) bool {
	return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_')
}

// String returns the secret. Never log it.
func (s TokenSecret) String() string { return s.v }

// Hash is the at-rest lookup key: auth-go's hex SHA-256 of the whole
// secret. A fast hash is right here — the secret has 256 bits of
// entropy, so there is nothing to brute-force.
func (s TokenSecret) Hash() string {
	tok, err := authgo.TokenFromString(s.v)
	if err != nil {
		return ""
	}
	return authgo.HashToken(tok)
}

// Hint is the prefix plus four characters: enough for a person to tell
// tokens apart in a list, too little to matter as a secret.
func (s TokenSecret) Hint() string {
	return s.v[:len(TokenPrefix)+hintBodyLen]
}

// ActorKind says what kind of principal acted.
type ActorKind string

const (
	ActorPerson ActorKind = "person"
	ActorToken  ActorKind = "token"
)

// Actor identifies who did something: a person or an API token.
type Actor struct {
	Kind ActorKind
	ID   uuid.UUID
}

// PersonActor names a person.
func PersonActor(id PersonID) Actor { return Actor{Kind: ActorPerson, ID: id.UUID()} }

// TokenActor names an API token.
func TokenActor(id TokenID) Actor { return Actor{Kind: ActorToken, ID: id.UUID()} }

// String renders "person:<id>" or "token:<id>".
func (a Actor) String() string { return string(a.Kind) + ":" + a.ID.String() }

// ParseActor parses Actor.String's output.
func ParseActor(s string) (Actor, error) {
	kind, id, _ := strings.Cut(s, ":")
	u, err := parseID(id)
	if err != nil || (ActorKind(kind) != ActorPerson && ActorKind(kind) != ActorToken) {
		return Actor{}, fmt.Errorf("%w: actor %q", ErrInvalidID, s)
	}
	return Actor{Kind: ActorKind(kind), ID: u}, nil
}

// APIToken is the aggregate for a tenant-owned bearer credential used by
// the CLI, CI and agents. Tokens belong to the tenant, not to the person
// who created them, and are revoked rather than deleted so their use
// stays auditable. Project scoping is a later, additive restriction.
type APIToken struct {
	ID         TokenID
	TenantID   tenancy.ID
	Name       string
	Scopes     Scopes
	Hint       string
	Hash       string
	CreatedBy  Actor
	CreatedAt  time.Time
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}

// NewAPIToken creates a token and its one-time secret. expiresAt is
// optional; when set it must be in the future.
func NewAPIToken(tenant tenancy.ID, name string, scopes Scopes, expiresAt *time.Time, by Actor, now time.Time) (APIToken, TokenSecret, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxTokenName {
		return APIToken{}, TokenSecret{}, ErrInvalidTokenName
	}
	if len(scopes) == 0 {
		return APIToken{}, TokenSecret{}, ErrInvalidScope
	}
	if expiresAt != nil && !expiresAt.After(now) {
		return APIToken{}, TokenSecret{}, ErrInvalidTokenExpiry
	}
	secret, err := NewTokenSecret()
	if err != nil {
		return APIToken{}, TokenSecret{}, err
	}
	return APIToken{
		ID:        NewTokenID(),
		TenantID:  tenant,
		Name:      name,
		Scopes:    scopes,
		Hint:      secret.Hint(),
		Hash:      secret.Hash(),
		CreatedBy: by,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}, secret, nil
}

// CheckUsable returns ErrTokenRevoked or ErrTokenExpired, or nil.
func (t APIToken) CheckUsable(now time.Time) error {
	if t.RevokedAt != nil {
		return ErrTokenRevoked
	}
	if t.ExpiresAt != nil && !now.Before(*t.ExpiresAt) {
		return ErrTokenExpired
	}
	return nil
}

// Grant is what the token may do in its tenant.
func (t APIToken) Grant() Grant { return GrantForScopes(t.Scopes) }

// Revoke ends the token for good.
func (t *APIToken) Revoke(now time.Time) error {
	if t.RevokedAt != nil {
		return ErrTokenRevoked
	}
	t.RevokedAt = &now
	return nil
}
