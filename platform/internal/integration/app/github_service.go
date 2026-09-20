package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/integration/domain"
)

// principalGitHub is the background principal the inbox worker acts as.
const principalGitHub = "integration.github"

// GitHubDeps are the GitHub service's collaborators. Tx, Inbox, GitHub,
// Verifier, Events and Branches are required; the rest take defaults.
//
// Checks and Sources are the PR check's half (RFC 0004 §6.4). They go
// together: without both, the webhook side still runs and the check
// worker does nothing, which is what a deployment that only wants the
// branch view gets.
type GitHubDeps struct {
	Tx       GitHubTransactor
	Inbox    DeliveryInbox
	GitHub   GitHub
	Verifier WebhookVerifier
	Events   WebhookEvents
	Branches Branches
	// Checks is the check queue; Sources is what the report is rendered
	// from.
	Checks  CheckQueue
	Sources CheckSources
	// StudioURL is where the sticky comment's branch link points.
	StudioURL      string
	Metrics        GitHubMetrics
	CheckMetrics   CheckMetrics
	TracerProvider trace.TracerProvider
	Logger         *slog.Logger
	Now            func() time.Time
}

// GitHubService is the GitHub integration's use cases (RFC 0004 §6):
// the install flow with verified ownership, Git connections, and the
// webhook endpoint with its inbox.
//
// It exists only when the deployment configures a GitHub App. Without
// one, the composition root leaves it nil and the HTTP edge answers
// `github_not_configured`; nothing else in the server changes.
type GitHubService struct {
	tx           GitHubTransactor
	inbox        DeliveryInbox
	gh           GitHub
	verifier     WebhookVerifier
	events       WebhookEvents
	branches     Branches
	checks       CheckQueue
	sources      CheckSources
	studioURL    string
	metrics      GitHubMetrics
	checkMetrics CheckMetrics
	tracer       trace.Tracer
	logger       *slog.Logger
	now          func() time.Time
}

// tracerName names the GitHub integration's spans' instrumentation
// scope: one trace per check job (RFC 0004 §11).
const tracerName = "github.com/felixgeelhaar/glossa/platform/internal/integration"

// NewGitHubService returns the service, or an error naming what is
// missing.
func NewGitHubService(d GitHubDeps) (*GitHubService, error) {
	var errs []error
	for name, ok := range map[string]bool{
		"Tx": d.Tx != nil, "Inbox": d.Inbox != nil, "GitHub": d.GitHub != nil,
		"Verifier": d.Verifier != nil, "Events": d.Events != nil, "Branches": d.Branches != nil,
	} {
		if !ok {
			errs = append(errs, fmt.Errorf("integration: GitHubDeps.%s is required", name))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	if (d.Checks == nil) != (d.Sources == nil) {
		return nil, errors.New("integration: GitHubDeps.Checks and .Sources go together")
	}
	s := &GitHubService{
		tx: d.Tx, inbox: d.Inbox, gh: d.GitHub, verifier: d.Verifier, events: d.Events,
		branches: d.Branches, checks: d.Checks, sources: d.Sources,
		studioURL: strings.TrimRight(d.StudioURL, "/"),
		metrics:   d.Metrics, checkMetrics: d.CheckMetrics, logger: d.Logger, now: d.Now,
		tracer: noop.NewTracerProvider().Tracer(tracerName),
	}
	if d.TracerProvider != nil {
		s.tracer = d.TracerProvider.Tracer(tracerName)
	}
	if s.metrics == nil {
		s.metrics = NoGitHubMetrics{}
	}
	if s.checkMetrics == nil {
		s.checkMetrics = NoCheckMetrics{}
	}
	if s.logger == nil {
		s.logger = slog.New(slog.DiscardHandler)
	}
	if s.now == nil {
		s.now = func() time.Time { return time.Now().UTC() }
	}
	return s, nil
}

// ── the install flow ─────────────────────────────────────────────────

// StartInstall issues a single-use state bound to this tenant, this
// person and a short expiry, and says where to send them (RFC 0004
// §6.1). The state is random and unguessable; only its SHA-256 is
// stored, so a reader of the database cannot replay one.
func (s *GitHubService) StartInstall(ctx context.Context) (InstallIntent, error) {
	by, err := gitHubActor(ctx, authz.IntegrationManage)
	if err != nil {
		return InstallIntent{}, err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return InstallIntent{}, fmt.Errorf("integration: generate the install state: %w", err)
	}
	state := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(state))
	now := s.now()
	out := InstallIntent{
		ID: uuid.Must(uuid.NewV7()), TenantID: tenantOf(ctx), Person: by, State: state,
		InstallURL: s.gh.InstallURL(state), CreatedAt: now, ExpiresAt: now.Add(domain.InstallIntentTTL),
	}
	err = s.tx.InGitHub(ctx, func(ctx context.Context, st GitHubStore) error {
		return st.InsertInstallIntent(ctx, out.ID, by, sum[:], out.CreatedAt, out.ExpiresAt)
	})
	if err != nil {
		return InstallIntent{}, err
	}
	return out, nil
}

// CompleteInstall is the callback GitHub sends the person back through.
type CompleteInstall struct {
	// State is what StartInstall issued.
	State string
	// Code is GitHub's one-time OAuth code, redeemed for the user token
	// the ownership check needs and then dropped.
	Code string
	// InstallationID is the installation GitHub says was made.
	InstallationID int64
	// SetupAction is GitHub's `setup_action`: `install` when an
	// installation happened, `request` when the person could only ask
	// their organization for one.
	SetupAction string
}

// CompleteInstall verifies the state, then verifies through the
// person's own OAuth token that they can actually see the installation
// they claimed, and only then maps it to this tenant (RFC 0004 §6.1).
//
// The state is burned first, before the GitHub calls: it is single-use,
// so a failed attempt cannot be retried with the same state. Starting
// over is one click. The user token is used for that one call and
// never stored.
func (s *GitHubService) CompleteInstall(ctx context.Context, in CompleteInstall) (domain.Installation, error) {
	by, err := gitHubActor(ctx, authz.IntegrationManage)
	if err != nil {
		return domain.Installation{}, err
	}
	if in.SetupAction == "request" {
		// The person asked their organization to install the App; there
		// is no installation to claim yet.
		return domain.Installation{}, fmt.Errorf("%w: GitHub reported setup_action=request, so no installation was made",
			ErrInstallStateInvalid)
	}
	if in.InstallationID <= 0 || in.Code == "" {
		return domain.Installation{}, fmt.Errorf("%w: the callback needs an installation_id and a code", ErrInstallStateInvalid)
	}
	sum := sha256.Sum256([]byte(in.State))
	var intent RedeemedIntent
	if err := s.tx.InGitHub(ctx, func(ctx context.Context, st GitHubStore) error {
		var err error
		intent, err = st.RedeemInstallIntent(ctx, sum[:], s.now())
		return err
	}); err != nil {
		return domain.Installation{}, err
	}
	if intent.TenantID != tenantOf(ctx) {
		// Row-level security already scoped the redemption to this
		// tenant, so this is belt and braces.
		return domain.Installation{}, ErrInstallStateInvalid
	}
	if intent.Person != by {
		return domain.Installation{}, ErrInstallStateNotYours
	}

	userToken, err := s.gh.ExchangeUserCode(ctx, in.Code)
	if err != nil {
		return domain.Installation{}, err
	}
	seen, err := s.gh.VerifyInstallationOwner(ctx, userToken, in.InstallationID)
	userToken = "" //nolint:ineffassign,wastedassign // the token is done with; do not keep it alive
	if err != nil {
		return domain.Installation{}, err
	}

	now := s.now()
	inst, err := domain.NewInstallation(domain.InstallationInput{
		GitHubID: seen.ID, AccountID: seen.AccountID, AccountLogin: seen.AccountLogin, AccountType: seen.AccountType,
	}, by, now)
	if err != nil {
		return domain.Installation{}, err
	}
	err = s.tx.InGitHub(ctx, func(ctx context.Context, st GitHubStore) error {
		// Re-connecting one we already hold is the same installation,
		// not a conflict: refresh it instead.
		if held, err := st.InstallationByGitHubID(ctx, seen.ID); err == nil {
			if err := st.SetInstallationState(ctx, seen.ID, domain.InstallationActive, seen.AccountLogin, now); err != nil {
				return err
			}
			held.State, held.AccountLogin, held.UpdatedAt = domain.InstallationActive, seen.AccountLogin, now
			inst = held
			return nil
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		return st.InsertInstallation(ctx, inst)
	})
	if err != nil {
		return domain.Installation{}, err
	}
	s.logger.InfoContext(ctx, "integration: a GitHub installation was connected",
		slog.String("tenant_id", tenantOf(ctx).String()), slog.Int64("installation_id", inst.GitHubID),
		slog.String("account", inst.AccountLogin))
	return inst, nil
}

// InstallationView is an installation with the repositories the App can
// see through it.
type InstallationView struct {
	domain.Installation
	Repositories []GitHubRepository
	// RepositoriesUnavailable is true when GitHub could not be reached
	// for this installation: the installation still lists, with no
	// repositories, rather than failing the whole page.
	RepositoriesUnavailable bool
}

// Installations lists the tenant's installations with their
// repositories. A suspended or revoked installation lists without
// repositories, because GitHub would refuse the call anyway.
func (s *GitHubService) Installations(ctx context.Context) ([]InstallationView, error) {
	if err := authz.Require(ctx, authz.IntegrationRead); err != nil {
		return nil, err
	}
	var stored []domain.Installation
	if err := s.tx.InGitHub(ctx, func(ctx context.Context, st GitHubStore) error {
		var err error
		stored, err = st.Installations(ctx)
		return err
	}); err != nil {
		return nil, err
	}
	tenant := tenantOf(ctx)
	out := make([]InstallationView, 0, len(stored))
	for _, i := range stored {
		v := InstallationView{Installation: i}
		if i.Usable() {
			repos, err := s.gh.Repositories(ctx, GitHubTarget{TenantID: tenant, InstallationID: i.GitHubID})
			if err != nil {
				s.logger.WarnContext(ctx, "integration: listing an installation's repositories failed",
					slog.Int64("installation_id", i.GitHubID), slog.Any("error", err))
				v.RepositoriesUnavailable = true
			}
			v.Repositories = repos
		}
		out = append(out, v)
	}
	return out, nil
}

// ForgetInstallation removes an installation and its Git connections
// from Glossa. It does not uninstall the App: only GitHub can do that,
// on the account's settings page.
func (s *GitHubService) ForgetInstallation(ctx context.Context, id uuid.UUID) error {
	if _, err := gitHubActor(ctx, authz.IntegrationManage); err != nil {
		return err
	}
	return s.tx.InGitHub(ctx, func(ctx context.Context, st GitHubStore) error {
		return st.DeleteInstallation(ctx, id)
	})
}

// ── Git connections ──────────────────────────────────────────────────

// ConnectRepository is a Git connection as asked for.
type ConnectRepository struct {
	// Installation is Glossa's installation row.
	Installation uuid.UUID
	domain.ConnectionInput
}

// Connect ties one of an installation's repositories to a project and
// application, optionally at a monorepo path. The repository must be
// one the installation can actually see, and the application one the
// project actually has, so a connection that could never work is
// refused at the source.
func (s *GitHubService) Connect(ctx context.Context, in ConnectRepository) (domain.GitConnection, error) {
	by, err := gitHubActor(ctx, authz.IntegrationManage)
	if err != nil {
		return domain.GitConnection{}, err
	}
	// The checks call GitHub and Catalog, so they happen before the
	// transaction opens: a unit of work is never nested, and a database
	// transaction never waits on the network.
	var inst domain.Installation
	if err := s.tx.InGitHub(ctx, func(ctx context.Context, st GitHubStore) error {
		var err error
		inst, err = st.Installation(ctx, in.Installation)
		return err
	}); err != nil {
		return domain.GitConnection{}, err
	}
	if err := s.checkRepository(ctx, inst, &in.ConnectionInput); err != nil {
		return domain.GitConnection{}, err
	}
	if err := s.checkApplication(ctx, in.ProjectID, in.ApplicationID); err != nil {
		return domain.GitConnection{}, err
	}
	out, err := domain.NewGitConnection(inst, in.ConnectionInput, by, s.now())
	if err != nil {
		return domain.GitConnection{}, err
	}
	if err := s.tx.InGitHub(ctx, func(ctx context.Context, st GitHubStore) error {
		return st.InsertGitConnection(ctx, out)
	}); err != nil {
		return domain.GitConnection{}, err
	}
	return out, nil
}

// checkRepository refuses a repository the installation cannot see, and
// fills in the label and the default branch GitHub reports when the
// caller left them out.
func (s *GitHubService) checkRepository(ctx context.Context, inst domain.Installation, in *domain.ConnectionInput) error {
	if !inst.Usable() {
		return domain.ErrInstallationRevoked
	}
	repos, err := s.gh.Repositories(ctx, GitHubTarget{TenantID: tenantOf(ctx), InstallationID: inst.GitHubID})
	if err != nil {
		return err
	}
	for _, r := range repos {
		if r.ID != in.RepositoryID {
			continue
		}
		in.RepositoryName = r.FullName
		if in.DefaultBranch == "" {
			in.DefaultBranch = r.DefaultBranch
		}
		return nil
	}
	return ErrRepositoryNotVisible
}

func (s *GitHubService) checkApplication(ctx context.Context, project, application uuid.UUID) error {
	ok, err := s.branches.ApplicationExists(ctx, project, application)
	if err != nil {
		return err
	}
	if !ok {
		return ErrApplicationNotFound
	}
	return nil
}

// Connections lists the tenant's Git connections.
func (s *GitHubService) Connections(ctx context.Context, f ConnectionFilter) ([]domain.GitConnection, error) {
	if err := authz.Require(ctx, authz.IntegrationRead); err != nil {
		return nil, err
	}
	var out []domain.GitConnection
	err := s.tx.InGitHub(ctx, func(ctx context.Context, st GitHubStore) error {
		var err error
		out, err = st.GitConnections(ctx, f)
		return err
	})
	return out, err
}

// Connection returns one Git connection.
func (s *GitHubService) Connection(ctx context.Context, id uuid.UUID) (domain.GitConnection, error) {
	if err := authz.Require(ctx, authz.IntegrationRead); err != nil {
		return domain.GitConnection{}, err
	}
	var out domain.GitConnection
	err := s.tx.InGitHub(ctx, func(ctx context.Context, st GitHubStore) error {
		var err error
		out, err = st.GitConnection(ctx, id)
		return err
	})
	return out, err
}

// ChangeConnection edits a connection's project, application, default
// branch or path. The repository is fixed: pointing a connection at
// another repository is a different connection.
func (s *GitHubService) ChangeConnection(ctx context.Context, id uuid.UUID, in domain.ConnectionInput) (domain.GitConnection, error) {
	if _, err := gitHubActor(ctx, authz.IntegrationManage); err != nil {
		return domain.GitConnection{}, err
	}
	// As in Connect, Catalog is asked before the transaction opens.
	var c domain.GitConnection
	if err := s.tx.InGitHub(ctx, func(ctx context.Context, st GitHubStore) error {
		var err error
		c, err = st.GitConnection(ctx, id)
		return err
	}); err != nil {
		return domain.GitConnection{}, err
	}
	if err := s.checkApplication(ctx, in.ProjectID, in.ApplicationID); err != nil {
		return domain.GitConnection{}, err
	}
	if err := c.Change(in, s.now()); err != nil {
		return domain.GitConnection{}, err
	}
	if err := s.tx.InGitHub(ctx, func(ctx context.Context, st GitHubStore) error {
		return st.SaveGitConnection(ctx, c)
	}); err != nil {
		return domain.GitConnection{}, err
	}
	return c, nil
}

// Disconnect removes a Git connection.
func (s *GitHubService) Disconnect(ctx context.Context, id uuid.UUID) error {
	if _, err := gitHubActor(ctx, authz.IntegrationManage); err != nil {
		return err
	}
	return s.tx.InGitHub(ctx, func(ctx context.Context, st GitHubStore) error {
		return st.DeleteGitConnection(ctx, id)
	})
}

// ── helpers ──────────────────────────────────────────────────────────

func gitHubActor(ctx context.Context, perm authz.Permission) (string, error) {
	if err := authz.Require(ctx, perm); err != nil {
		return "", err
	}
	return actorOf(ctx)
}
