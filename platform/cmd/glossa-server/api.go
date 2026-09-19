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
	catalogapi "github.com/felixgeelhaar/glossa/platform/internal/catalog/adapters/httpapi"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/adapters/httpapi"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/adapters/mail"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/adapters/passkey"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/adapters/postgres"
	identityapp "github.com/felixgeelhaar/glossa/platform/internal/identity/app"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/config"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/db"
	localizationapi "github.com/felixgeelhaar/glossa/platform/internal/localization/adapters/httpapi"
	releaseapi "github.com/felixgeelhaar/glossa/platform/internal/release/adapters/httpapi"
)

// apiServer is the /v1 strict server: every bounded context's handler
// type is embedded here, and together they implement the generated
// apiv1.StrictServerInterface.
type apiServer struct {
	*httpapi.API
	*catalogAPI
	*localizationAPI
	*releaseAPI
}

// Each context names its handler type API; the aliases give the
// embedded fields distinct names.
type (
	catalogAPI      = catalogapi.API
	localizationAPI = localizationapi.API
	releaseAPI      = releaseapi.API
)

var _ apiv1.StrictServerInterface = apiServer{}

// apiRoutes mounts the generated /v1 router. Identity's Guard enforces
// each operation's security requirement for every context, and its
// error hooks render every failure as problem details.
func apiRoutes(identity *httpapi.API, c contexts) func(*http.ServeMux) {
	server := apiServer{API: identity, catalogAPI: c.catalogAPI, localizationAPI: c.localizationAPI, releaseAPI: c.releaseAPI}
	return func(mux *http.ServeMux) {
		strict := apiv1.NewStrictHandlerWithOptions(server, nil, apiv1.StrictHTTPServerOptions{
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
