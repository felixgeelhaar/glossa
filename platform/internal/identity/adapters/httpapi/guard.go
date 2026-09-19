package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/felixgeelhaar/glossa/platform/internal/apiv1"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/app"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/problem"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

// Cookie and header names (api/openapi.yaml, securitySchemes).
const (
	// SessionCookie holds the session. The __Host- prefix makes browsers
	// insist on Secure, Path=/ and no Domain, so no subdomain can set it.
	SessionCookie = "__Host-glossa_session"
	// CeremonyCookie holds the key of an in-flight passkey ceremony.
	CeremonyCookie = "__Host-glossa_webauthn"
	// CSRFHeader carries the session's CSRF token on unsafe requests.
	CSRFHeader = "X-CSRF-Token"
)

// caller is what the guard established about the request.
type caller struct {
	authn app.Authn
	// session is the raw session cookie, when the caller used one; it's
	// needed to sign out and to derive the CSRF token.
	session string
}

type callerKey struct{}

func callerFrom(ctx context.Context) (caller, bool) {
	c, ok := ctx.Value(callerKey{}).(caller)
	return c, ok
}

// Guard enforces each operation's security requirement from the
// contract. It runs inside the router (so r.Pattern and r.PathValue are
// set): it authenticates the session cookie (+ CSRF header on unsafe
// methods) or bearer token the operation accepts, and on
// /v1/tenants/{tenant}/… hands over to tenancy.Middleware with this
// package's Resolver, which checks the tenant against the caller.
func (a *API) Guard(next http.Handler) http.Handler {
	tenantScoped := tenancy.Middleware(a, a.logger)(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, ok := a.reqs[r.Pattern]
		if !ok {
			a.errs.write(w, r, fmt.Errorf("httpapi: route %q is not in the contract", r.Pattern))
			return
		}
		if req.Public {
			if unsafeMethod(r.Method) && r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				problem.WriteDetails(w, problem.New(http.StatusForbidden, "cross_site_request",
					"sign-in requests must come from Glossa's own pages"))
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		c, err := a.authenticate(r, req)
		if err != nil {
			a.errs.write(w, r, err)
			return
		}
		ctx := context.WithValue(r.Context(), callerKey{}, c)
		if r.PathValue("tenant") != "" {
			tenantScoped.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		next.ServeHTTP(w, r.WithContext(authz.WithPrincipal(ctx, c.authn.Principal())))
	})
}

var errCSRF = problem.New(http.StatusForbidden, "csrf_invalid",
	"send the session's csrf_token in the X-CSRF-Token header")

func (a *API) authenticate(r *http.Request, req apiv1.Requirement) (caller, error) {
	if h := r.Header.Get("Authorization"); h != "" {
		scheme, cred, _ := strings.Cut(h, " ")
		if !req.Bearer || !strings.EqualFold(scheme, "Bearer") {
			return caller{}, app.ErrUnauthenticated
		}
		authn, err := a.svc.AuthenticateToken(r.Context(), strings.TrimSpace(cred))
		return caller{authn: authn}, err
	}
	cookie, err := r.Cookie(SessionCookie)
	if err != nil || !req.Session {
		return caller{}, app.ErrUnauthenticated
	}
	authn, err := a.svc.AuthenticateSession(r.Context(), cookie.Value)
	if err != nil {
		return caller{}, err
	}
	if req.CSRF && unsafeMethod(r.Method) && !a.validCSRF(cookie.Value, r.Header.Get(CSRFHeader)) {
		return caller{}, errCSRF
	}
	return caller{authn: authn, session: cookie.Value}, nil
}

// ResolveTenant implements tenancy.Resolver. The tenant named in the
// path is only a request: it becomes the request's tenant once the
// session's person is an active member of it, or it is the API token's
// own tenant. The principal and its grant ride on the returned context.
func (a *API) ResolveTenant(r *http.Request) (context.Context, tenancy.ID, error) {
	c, ok := callerFrom(r.Context())
	if !ok {
		return nil, tenancy.ID{}, tenancy.ErrUnauthenticated
	}
	id, err := tenancy.ParseID(r.PathValue("tenant"))
	if err != nil {
		return nil, tenancy.ID{}, tenancy.ErrForbidden
	}
	p, err := a.svc.Authorize(r.Context(), c.authn, id)
	if errors.Is(err, app.ErrForbidden) {
		return nil, tenancy.ID{}, tenancy.ErrForbidden
	}
	if err != nil {
		return nil, tenancy.ID{}, err
	}
	return authz.WithPrincipal(r.Context(), p), id, nil
}

var _ tenancy.Resolver = (*API)(nil)

func unsafeMethod(m string) bool {
	return m != http.MethodGet && m != http.MethodHead && m != http.MethodOptions
}

// csrfToken derives the session's CSRF token: an HMAC of the session
// cookie under a server key. Nothing is stored, it dies with the
// session, and a page on another origin can't compute or read it.
func (a *API) csrfToken(session string) string {
	mac := hmac.New(sha256.New, a.csrfKey)
	mac.Write([]byte("glossa-csrf\x00" + session))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *API) validCSRF(session, got string) bool {
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(a.csrfToken(session))) == 1
}

func sessionCookie(value string, ttl time.Duration) string {
	return (&http.Cookie{
		Name: SessionCookie, Value: value, Path: "/", MaxAge: int(ttl.Seconds()),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	}).String()
}

func clearedSessionCookie() string {
	return (&http.Cookie{
		Name: SessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	}).String()
}

func ceremonyCookie(key string) string {
	return (&http.Cookie{
		Name: CeremonyCookie, Value: key, Path: "/", MaxAge: int(app.CeremonyTTL.Seconds()),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	}).String()
}
