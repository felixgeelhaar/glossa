package app

import (
	"context"
	"time"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/pagination"
)

// ListTokens lists the tenant's API tokens, revoked ones included.
func (s *Service) ListTokens(ctx context.Context, page pagination.Page) ([]domain.APIToken, *string, error) {
	if err := authz.Require(ctx, authz.TokensRead); err != nil {
		return nil, nil, err
	}
	after, err := parseAfter(page.After)
	if err != nil {
		return nil, nil, err
	}
	var rows []domain.APIToken
	err = s.tx.InTenant(ctx, func(ctx context.Context, st TenantStore) error {
		rows, err = st.Tokens(ctx, domain.TokenID(after), page.Limit())
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	items, next := pagination.Trim(rows, page, func(t domain.APIToken) string { return t.ID.String() })
	return items, next, nil
}

// GetToken returns one token of the context's tenant.
func (s *Service) GetToken(ctx context.Context, id domain.TokenID) (domain.APIToken, error) {
	if err := authz.Require(ctx, authz.TokensRead); err != nil {
		return domain.APIToken{}, err
	}
	var t domain.APIToken
	err := s.tx.InTenant(ctx, func(ctx context.Context, st TenantStore) error {
		var err error
		t, err = st.Token(ctx, id)
		return err
	})
	return t, err
}

// CreatedToken is a new token; Secret is nil when the create was a
// replay of an earlier request, because secrets are shown once.
type CreatedToken struct {
	Token    domain.APIToken
	Secret   *domain.TokenSecret
	Replayed bool
}

// CreateToken issues an API token in the context's tenant. Its scopes
// may not exceed what the caller may do.
func (s *Service) CreateToken(ctx context.Context, name string, scopes []string, expiresAt *time.Time, idemKey string) (CreatedToken, error) {
	if err := authz.Require(ctx, authz.TokensManage); err != nil {
		return CreatedToken{}, err
	}
	p, _ := authz.From(ctx)
	sc, err := domain.ParseScopes(scopes)
	if err != nil {
		return CreatedToken{}, err
	}
	tok, secret, err := domain.NewAPIToken(p.Tenant, name, sc, expiresAt, p.Actor, s.now())
	if err != nil {
		return CreatedToken{}, err
	}
	if !p.Grant.Covers(tok.Grant()) {
		return CreatedToken{}, domain.ErrScopeExceedsGrant
	}
	if idemKey != "" {
		if err := checkIdempotencyKey(idemKey); err != nil {
			return CreatedToken{}, err
		}
		tok.ID = domain.TokenID(idempotentID("token.create", p.Tenant.String(), p.Actor, idemKey))
	}
	out := CreatedToken{Token: tok, Secret: &secret}
	err = s.tx.InTenant(ctx, func(ctx context.Context, st TenantStore) error {
		inserted, err := st.InsertToken(ctx, tok)
		if err != nil {
			return err
		}
		if !inserted {
			existing, err := st.Token(ctx, tok.ID)
			if err != nil {
				return err
			}
			if existing.Name != tok.Name {
				return ErrIdempotencyReuse
			}
			out = CreatedToken{Token: existing, Replayed: true}
			return nil
		}
		return st.Publish(ctx, outbox.Event{
			Type: domain.EventTokenCreated, AggregateType: domain.AggregateToken, AggregateID: tok.ID.String(),
			Payload: domain.TokenCreated{
				TokenID: tok.ID.String(), Name: tok.Name, Scopes: tok.Scopes.Strings(), CreatedBy: p.Actor.String(),
			},
		})
	})
	if err != nil {
		return CreatedToken{}, err
	}
	return out, nil
}

// RevokeToken revokes a token of the context's tenant. It stops working
// at once: authentication checks revoked_at on every request.
func (s *Service) RevokeToken(ctx context.Context, id domain.TokenID) error {
	if err := authz.Require(ctx, authz.TokensManage); err != nil {
		return err
	}
	p, _ := authz.From(ctx)
	return s.tx.InTenant(ctx, func(ctx context.Context, st TenantStore) error {
		t, err := st.Token(ctx, id)
		if err != nil {
			return err
		}
		if err := t.Revoke(s.now()); err != nil {
			return err
		}
		if err := st.RevokeToken(ctx, t, p.Actor); err != nil {
			return err
		}
		return st.Publish(ctx, outbox.Event{
			Type: domain.EventTokenRevoked, AggregateType: domain.AggregateToken, AggregateID: t.ID.String(),
			Payload: domain.TokenRevoked{TokenID: t.ID.String(), RevokedBy: p.Actor.String()},
		})
	})
}
