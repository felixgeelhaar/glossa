// Package user models the admin / translator identity. Lives
// behind a tenant — every read/write is RLS-scoped.
package user

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleAdmin      Role = "admin"
	RoleTranslator Role = "translator"
)

// User is the read-model. PasswordHash is loaded only on the login
// path; everywhere else handlers should never see it.
type User struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	Email        string
	PasswordHash []byte
	Role         Role
	Locales      []string // BCP-47 codes; empty = all locales (admins)
	CreatedAt    time.Time
}

var (
	ErrInvalidEmail = errors.New("user: invalid email")
	ErrInvalidRole  = errors.New("user: invalid role")
)

// Local-part chars per RFC 5322 simplified; domain check via the
// presence of an `@` and a `.` somewhere after. Good enough for our
// scope — proper email validation lives in the SMTP exchange.
var emailRE = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

func NormalizeEmail(s string) (string, error) {
	v := strings.TrimSpace(strings.ToLower(s))
	if !emailRE.MatchString(v) {
		return "", ErrInvalidEmail
	}
	return v, nil
}

func ParseRole(s string) (Role, error) {
	switch Role(s) {
	case RoleAdmin, RoleTranslator:
		return Role(s), nil
	default:
		return "", ErrInvalidRole
	}
}

// IsAdmin returns true if the user holds the admin role.
func (u User) IsAdmin() bool { return u.Role == RoleAdmin }

// CanEditLocale enforces translator scoping: admins edit any
// locale; translators only the codes listed in Locales (empty
// list = scoped to nothing, which is the safe default for a
// freshly created translator).
func (u User) CanEditLocale(code string) bool {
	if u.IsAdmin() {
		return true
	}
	for _, l := range u.Locales {
		if l == code {
			return true
		}
	}
	return false
}
