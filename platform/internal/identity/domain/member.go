package domain

import (
	"time"

	authgo "github.com/klarlabs-studio/auth-go/domain"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

// MemberStatus is where a membership is in its life.
type MemberStatus string

const (
	// MemberInvited is an invitation to an email address that no person
	// has accepted yet. It grants nothing.
	MemberInvited MemberStatus = "invited"
	// MemberActive is a person's working membership.
	MemberActive MemberStatus = "active"
)

// Member is the aggregate for a person's place in one tenant: their
// roles and, for translators and reviewers, the locales they work on.
//
// A member is invited by email and becomes active when a person who
// proved they own that address signs in, so inviting never needs to see
// — or reveal — whether the address already has an account.
type Member struct {
	ID       MemberID
	TenantID tenancy.ID
	// PersonID is zero while the invitation is open.
	PersonID PersonID
	Email    authgo.Email
	Roles    Roles
	Locales  LocaleScope
	Status   MemberStatus
	// Version increments with every change; it is the member's ETag.
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewOwner is the first member of a tenant: its creator, active at once.
func NewOwner(tenant tenancy.ID, person PersonID, email authgo.Email, now time.Time) Member {
	return Member{
		ID:        NewMemberID(),
		TenantID:  tenant,
		PersonID:  person,
		Email:     email,
		Roles:     Roles{RoleOwner},
		Status:    MemberActive,
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Invite opens an invitation in an organization. actor is the inviter's
// grant: only an owner may hand out the owner role.
func Invite(tenant tenancy.ID, kind tenancy.Kind, email authgo.Email, roles Roles, locales LocaleScope, actor Grant, now time.Time) (Member, error) {
	if kind == tenancy.KindIndividual {
		return Member{}, ErrIndividualTenant
	}
	if err := checkAccess(roles, locales); err != nil {
		return Member{}, err
	}
	if roles.Has(RoleOwner) && !actor.Allows(PermOwnersManage) {
		return Member{}, ErrOwnerChangeForbidden
	}
	return Member{
		ID:        NewMemberID(),
		TenantID:  tenant,
		Email:     email,
		Roles:     roles,
		Locales:   locales,
		Status:    MemberInvited,
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func checkAccess(roles Roles, locales LocaleScope) error {
	if len(roles) == 0 {
		return ErrNoRoles
	}
	if !locales.All() && !roles.anyLocaleScoped() {
		return ErrLocalesNeedLocaleRole
	}
	return nil
}

// Activate binds an open invitation to the person who accepted it.
func (m *Member) Activate(person PersonID, now time.Time) error {
	if m.Status != MemberInvited {
		return ErrMemberNotInvited
	}
	m.PersonID = person
	m.Status = MemberActive
	m.touch(now)
	return nil
}

// IsOwner reports whether the member holds the owner role.
func (m Member) IsOwner() bool { return m.Roles.Has(RoleOwner) }

// Grant is what the member may do; an open invitation grants nothing.
func (m Member) Grant() Grant {
	if m.Status != MemberActive {
		return Grant{}
	}
	return GrantForMember(m.Roles, m.Locales)
}

// ChangeAccess replaces the member's roles and locales. actor is the
// grant of whoever makes the change; activeOwners counts the tenant's
// active owners, this member included.
func (m *Member) ChangeAccess(roles Roles, locales LocaleScope, actor Grant, activeOwners int, now time.Time) error {
	if err := checkAccess(roles, locales); err != nil {
		return err
	}
	touchesOwner := m.IsOwner() || roles.Has(RoleOwner)
	if touchesOwner && !actor.Allows(PermOwnersManage) {
		return ErrOwnerChangeForbidden
	}
	if m.countsAsOwner() && !roles.Has(RoleOwner) && activeOwners <= 1 {
		return ErrLastOwner
	}
	m.Roles, m.Locales = roles, locales
	m.touch(now)
	return nil
}

// CheckRemoval reports whether actor may remove the member.
func (m Member) CheckRemoval(actor Grant, activeOwners int) error {
	if m.IsOwner() && !actor.Allows(PermOwnersManage) {
		return ErrOwnerChangeForbidden
	}
	if m.countsAsOwner() && activeOwners <= 1 {
		return ErrLastOwner
	}
	return nil
}

func (m Member) countsAsOwner() bool { return m.IsOwner() && m.Status == MemberActive }

func (m *Member) touch(now time.Time) {
	m.Version++
	m.UpdatedAt = now
}
