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
	contextapi "github.com/felixgeelhaar/glossa/platform/internal/context/adapters/httpapi"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/adapters/httpapi"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/adapters/mail"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/adapters/passkey"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/adapters/postgres"
	identityapp "github.com/felixgeelhaar/glossa/platform/internal/identity/app"
	integrationapi "github.com/felixgeelhaar/glossa/platform/internal/integration/adapters/httpapi"
	intelligenceapi "github.com/felixgeelhaar/glossa/platform/internal/intelligence/adapters/httpapi"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/config"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/db"
	knowledgeapi "github.com/felixgeelhaar/glossa/platform/internal/knowledge/adapters/httpapi"
	localizationapi "github.com/felixgeelhaar/glossa/platform/internal/localization/adapters/httpapi"
	previewapi "github.com/felixgeelhaar/glossa/platform/internal/preview/adapters/httpapi"
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
	*knowledgeAPI
	*intelligenceAPI
	*integrationAPI
	*contextAPI
	*previewAPI
	*metaAPI
}

// Each context names its handler type API; the aliases give the
// embedded fields distinct names.
type (
	catalogAPI      = catalogapi.API
	localizationAPI = localizationapi.API
	releaseAPI      = releaseapi.API
	knowledgeAPI    = knowledgeapi.API
	intelligenceAPI = intelligenceapi.API
	integrationAPI  = integrationapi.API
	contextAPI      = contextapi.API
	previewAPI      = previewapi.API
)

var _ apiv1.StrictServerInterface = apiServer{}

// apiRoutes mounts the generated /v1 router. Identity's Guard enforces
// each operation's security requirement for every context, and its
// error hooks render every failure as problem details. Its CORS
// middleware sits outside the Guard so a refused request still carries
// the headers the browser needs to show the problem, and its Preflight
// handler answers OPTIONS, which the generated router doesn't register
// (RFC 0004 §5.2).
func apiRoutes(identity *httpapi.API, meta *metaAPI, c contexts) func(*http.ServeMux) {
	server := apiServer{API: identity, catalogAPI: c.catalogAPI, localizationAPI: c.localizationAPI, releaseAPI: c.releaseAPI,
		knowledgeAPI: c.knowledgeAPI, intelligenceAPI: c.intelligenceAPI, integrationAPI: c.integrationAPI,
		previewAPI: c.previewAPI, contextAPI: c.contextAPI, metaAPI: meta}
	return func(mux *http.ServeMux) {
		strict := apiv1.NewStrictHandlerWithOptions(server, nil, apiv1.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  identity.RequestError,
			ResponseErrorHandlerFunc: identity.ResponseError,
		})
		mux.HandleFunc("OPTIONS /v1/", identity.Preflight)
		apiv1.HandlerWithOptions(strict, apiv1.StdHTTPServerOptions{
			BaseRouter:       mux,
			Middlewares:      []apiv1.MiddlewareFunc{identity.CORS, identity.Guard},
			ErrorHandlerFunc: identity.ParamError,
		})
	}
}

// newIdentity wires the Identity context: auth-go's services over
// Postgres adapters, a mailer and passkeys when configured, and the HTTP
// edge.
func newIdentity(cfg config.Identity, logger *slog.Logger, pool *pgxpool.Pool) (*httpapi.API, *identityapp.Service, error) {
	root := cfg.AuthKey()
	csrfKey, err := deriveKey(root, "csrf")
	if err != nil {
		return nil, nil, err
	}
	totpKey, err := deriveKey(root, "totp-seal")
	if err != nil {
		return nil, nil, err
	}
	cipher, err := aesgcm.New(totpKey)
	if err != nil {
		return nil, nil, fmt.Errorf("identity: TOTP cipher: %w", err)
	}
	mailer, err := newMailer(cfg.Mail, logger)
	if err != nil {
		return nil, nil, err
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
			return nil, nil, err
		}
		deps.Passkeys, err = passkey.New(passkey.Config{
			RPID: cfg.WebAuthn.RPID, RPName: cfg.WebAuthn.RPName, Origins: cfg.WebAuthn.Origins, StateKey: stateKey,
		}, postgres.NewPasskeyRepo(uow))
		if err != nil {
			return nil, nil, err
		}
	}
	appCfg := identityapp.DefaultConfig(cfg.StudioURL)
	appCfg.SessionTTL = cfg.SessionTTL
	svc, err := identityapp.New(appCfg, deps)
	if err != nil {
		return nil, nil, err
	}
	api, err := httpapi.New(svc, csrfKey, logger)
	return api, svc, err
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

// newMailer returns nil when the deployment sends no email: Identity then
// refuses the email flows and lets passwords work unverified.
func newMailer(cfg config.Mail, logger *slog.Logger) (identityapp.Mailer, error) {
	switch cfg.Driver {
	case "none":
		logger.Info("GLOSSA_MAIL_DRIVER=none: no email is sent; magic links and password reset by email are off, password accounts work unverified")
		return nil, nil //nolint:nilnil // no mailer is a valid configuration
	case "log":
		logger.Warn("GLOSSA_MAIL_DRIVER=log: emails, including sign-in links, are written to the log and not sent")
		return mail.Log{Logger: logger}, nil
	}
	return mail.NewSMTP(mail.SMTPConfig{
		Addr: cfg.SMTPAddr, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword.Reveal(),
		From: cfg.From, AllowPlaintext: cfg.SMTPAllowPlaintext,
	})
}
