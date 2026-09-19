package main

import (
	"crypto/hkdf"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/klarlabs-studio/auth-go/aesgcm"

	"github.com/felixgeelhaar/glossa/platform/internal/apiv1"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/adapters/httpapi"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/adapters/mail"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/adapters/passkey"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/adapters/postgres"
	identityapp "github.com/felixgeelhaar/glossa/platform/internal/identity/app"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/config"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/db"
)

// apiServer is the /v1 strict server: every bounded context's handler
// type is embedded here, and together they implement the generated
// apiv1.StrictServerInterface. Identity is the first.
type apiServer struct {
	*httpapi.API
}

// apiRoutes mounts the generated /v1 router. Identity's Guard enforces
// each operation's security requirement for every context, and its
// error hooks render every failure as problem details.
func apiRoutes(identity *httpapi.API) func(*http.ServeMux) {
	return func(mux *http.ServeMux) {
		strict := apiv1.NewStrictHandlerWithOptions(apiServer{identity}, nil, apiv1.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  identity.RequestError,
			ResponseErrorHandlerFunc: identity.ResponseError,
		})
		apiv1.HandlerWithOptions(strict, apiv1.StdHTTPServerOptions{
			BaseRouter:       mux,
			Middlewares:      []apiv1.MiddlewareFunc{identity.Guard},
			ErrorHandlerFunc: identity.ParamError,
		})
	}
}

// newIdentity wires the Identity context: auth-go's services over
// Postgres adapters, a mailer, passkeys when configured, and the HTTP edge.
func newIdentity(cfg config.Identity, logger *slog.Logger, pool *pgxpool.Pool) (*httpapi.API, error) {
	root := cfg.AuthKey()
	csrfKey, err := deriveKey(root, "csrf")
	if err != nil {
		return nil, err
	}
	totpKey, err := deriveKey(root, "totp-seal")
	if err != nil {
		return nil, err
	}
	cipher, err := aesgcm.New(totpKey)
	if err != nil {
		return nil, fmt.Errorf("identity: TOTP cipher: %w", err)
	}
	mailer, err := newMailer(cfg.Mail, logger)
	if err != nil {
		return nil, err
	}

	uow := db.NewUnitOfWork(pool)
	deps := identityapp.Deps{
		Tx:            postgres.NewTransactor(uow, cipher),
		Sessions:      postgres.NewSessionRepo(uow),
		SignInLinks:   postgres.NewLinkRepo(uow, postgres.PurposeSignIn),
		ResetLinks:    postgres.NewLinkRepo(uow, postgres.PurposePasswordReset),
		TOTP:          postgres.NewTOTPRepo(uow, cipher),
		LoginAttempts: postgres.NewLoginAttemptRepo(uow),
		Mailer:        mailer,
		Logger:        logger,
	}
	if cfg.WebAuthn.Enabled() {
		stateKey, err := deriveKey(root, "webauthn-state")
		if err != nil {
			return nil, err
		}
		deps.Passkeys, err = passkey.New(passkey.Config{
			RPID: cfg.WebAuthn.RPID, RPName: cfg.WebAuthn.RPName, Origins: cfg.WebAuthn.Origins, StateKey: stateKey,
		}, postgres.NewPasskeyRepo(uow))
		if err != nil {
			return nil, err
		}
	}
	appCfg := identityapp.DefaultConfig(cfg.StudioURL)
	appCfg.SessionTTL = cfg.SessionTTL
	svc, err := identityapp.New(appCfg, deps)
	if err != nil {
		return nil, err
	}
	return httpapi.New(svc, csrfKey, logger)
}

// deriveKey gives each use of the auth secret its own key (HKDF-SHA256),
// so no two mechanisms ever share key material.
func deriveKey(root []byte, purpose string) ([]byte, error) {
	k, err := hkdf.Key(sha256.New, root, nil, "glossa "+purpose+" v1", 32)
	if err != nil {
		return nil, fmt.Errorf("identity: derive %s key: %w", purpose, err)
	}
	return k, nil
}

func newMailer(cfg config.Mail, logger *slog.Logger) (identityapp.Mailer, error) {
	if cfg.Driver == "log" {
		logger.Warn("GLOSSA_MAIL_DRIVER=log: emails, including sign-in links, are written to the log and not sent")
		return mail.Log{Logger: logger}, nil
	}
	return mail.NewSMTP(mail.SMTPConfig{
		Addr: cfg.SMTPAddr, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword.Reveal(),
		From: cfg.From, AllowPlaintext: cfg.SMTPAllowPlaintext,
	})
}
