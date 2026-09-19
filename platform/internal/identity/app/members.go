package app

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/pagination"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/problem"
)

// ListMembers lists the tenant's members and open invitations.
func (s *Service) ListMembers(ctx context.Context, page pagination.Page) ([]MemberView, *string, error) {
	if err := authz.Require(ctx, authz.MembersRead); err != nil {
		return nil, nil, err
	}
	after, err := parseAfter(page.After)
	if err != nil {
		return nil, nil, err
	}
	var rows []MemberView
	err = s.tx.InTenant(ctx, func(ctx context.Context, st TenantStore) error {
		rows, err = st.Members(ctx, domain.MemberID(after), page.Limit())
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	items, next := pagination.Trim(rows, page, func(m MemberView) string { return m.ID.String() })
	return items, next, nil
}

// parseAfter reads a page cursor holding an ID. The kernel decoded the
// token; a cursor that isn't an ID was forged or belongs to another list.
func parseAfter(s string) (uuid.UUID, error) {
	if s == "" {
		return uuid.Nil, nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, problem.New(http.StatusBadRequest, "invalid_page_token", "page_token is not one this list issued")
	}
	return id, nil
}

// AddMember invites email into the tenant with roles and locales.
func (s *Service) AddMember(ctx context.Context, email string, roles, locales []string, idemKey string) (m MemberView, replayed bool, err error) {
	if err := authz.Require(ctx, authz.MembersManage); err != nil {
		return MemberView{}, false, err
	}
	p, _ := authz.From(ctx)
	e, err := parseEmail(email)
	if err != nil {
		return MemberView{}, false, err
	}
	rs, err := domain.ParseRoles(roles)
	if err != nil {
		return MemberView{}, false, err
	}
	ls, err := domain.ParseLocaleScope(locales)
	if err != nil {
		return MemberView{}, false, err
	}
	if idemKey != "" {
		if err := checkIdempotencyKey(idemKey); err != nil {
			return MemberView{}, false, err
		}
	}
	err = s.tx.InTenant(ctx, func(ctx context.Context, st TenantStore) error {
		t, err := st.CurrentTenant(ctx)
		if err != nil {
			return err
		}
		invite, err := domain.Invite(t.ID, t.Kind, e, rs, ls, p.Grant, s.now())
		if err != nil {
			return err
		}
		if idemKey != "" {
			invite.ID = domain.MemberID(idempotentID("member.add", t.ID.String(), p.Actor, idemKey))
		}
		inserted, err := st.InsertMember(ctx, invite, p.Actor)
		if err != nil {
			return err
		}
		if !inserted {
			if m, err = st.Member(ctx, invite.ID); err != nil {
				return err
			}
			if m.Email != invite.Email {
				return ErrIdempotencyReuse
			}
			replayed = true
			return nil
		}
		m = MemberView{Member: invite}
		return st.Publish(ctx, memberAdded(invite, p.Actor))
	})
	return m, replayed, err
}

// GetMember returns one member.
func (s *Service) GetMember(ctx context.Context, id domain.MemberID) (MemberView, error) {
	if err := authz.Require(ctx, authz.MembersRead); err != nil {
		return MemberView{}, err
	}
	var m MemberView
	err := s.tx.InTenant(ctx, func(ctx context.Context, st TenantStore) error {
		var err error
		m, err = st.Member(ctx, id)
		return err
	})
	return m, err
}

// MemberChange is a partial update; nil fields keep their value.
type MemberChange struct {
	Roles   *[]string
	Locales *[]string
}

// UpdateMember changes roles and locales if the member is still at
// version ifMatch.
func (s *Service) UpdateMember(ctx context.Context, id domain.MemberID, ifMatch int, c MemberChange) (MemberView, error) {
	if err := authz.Require(ctx, authz.MembersManage); err != nil {
		return MemberView{}, err
	}
	p, _ := authz.From(ctx)
	var view MemberView
	err := s.tx.InTenant(ctx, func(ctx context.Context, st TenantStore) error {
		// Owners first, then the member: one lock order for every writer.
		owners, err := st.LockActiveOwners(ctx)
		if err != nil {
			return err
		}
		m, err := st.LockMember(ctx, id)
		if err != nil {
			return err
		}
		if m.Version != ifMatch {
			return ErrPreconditionFailed
		}
		roles, locales, err := applyChange(m, c)
		if err != nil {
			return err
		}
		if err := m.ChangeAccess(roles, locales, p.Grant, owners, s.now()); err != nil {
			return err
		}
		if err := st.UpdateMemberAccess(ctx, m); err != nil {
			return err
		}
		if err := st.Publish(ctx, outbox.Event{
			Type: domain.EventMemberAccessChange, AggregateType: domain.AggregateMember, AggregateID: m.ID.String(),
			Payload: domain.MemberAccessChanged{
				MemberID: m.ID.String(), Roles: m.Roles.Strings(), Locales: m.Locales.Strings(), ChangedBy: p.Actor.String(),
			},
		}); err != nil {
			return err
		}
		view, err = st.Member(ctx, id)
		return err
	})
	return view, err
}

func applyChange(m domain.Member, c MemberChange) (domain.Roles, domain.LocaleScope, error) {
	roles, locales := m.Roles, m.Locales
	var err error
	if c.Roles != nil {
		if roles, err = domain.ParseRoles(*c.Roles); err != nil {
			return nil, domain.LocaleScope{}, err
		}
	}
	if c.Locales != nil {
		if locales, err = domain.ParseLocaleScope(*c.Locales); err != nil {
			return nil, domain.LocaleScope{}, err
		}
	}
	return roles, locales, nil
}

// RemoveMember ends a membership or withdraws an invitation. ifMatch, if
// set, must equal the member's version.
func (s *Service) RemoveMember(ctx context.Context, id domain.MemberID, ifMatch *int) error {
	if err := authz.Require(ctx, authz.MembersManage); err != nil {
		return err
	}
	p, _ := authz.From(ctx)
	return s.tx.InTenant(ctx, func(ctx context.Context, st TenantStore) error {
		owners, err := st.LockActiveOwners(ctx)
		if err != nil {
			return err
		}
		m, err := st.LockMember(ctx, id)
		if err != nil {
			return err
		}
		if ifMatch != nil && m.Version != *ifMatch {
			return ErrPreconditionFailed
		}
		if err := m.CheckRemoval(p.Grant, owners); err != nil {
			return err
		}
		if err := st.DeleteMember(ctx, id); err != nil {
			return err
		}
		e := domain.MemberRemoved{MemberID: m.ID.String(), RemovedBy: p.Actor.String()}
		if !m.PersonID.IsZero() {
			e.PersonID = m.PersonID.String()
		}
		return st.Publish(ctx, outbox.Event{
			Type: domain.EventMemberRemoved, AggregateType: domain.AggregateMember, AggregateID: m.ID.String(),
			Payload: e,
		})
	})
}
