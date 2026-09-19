package cli

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"

	"github.com/felixgeelhaar/glossa/platform/internal/cli/credentials"
)

// tenantJSON is a tenant in --json output.
type tenantJSON struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// serverFor is --server, else glossa.yaml's server (if any).
func (inv *invocation) serverFor(flagValue string) (string, error) {
	if flagValue != "" {
		return strings.TrimRight(flagValue, "/"), nil
	}
	if s := inv.env.getenv("GLOSSA_SERVER"); s != "" {
		return strings.TrimRight(s, "/"), nil
	}
	cfg, err := inv.loadConfig()
	if err != nil {
		return "", &Error{Exit: ExitUsage, Code: "no_server", What: "which server?",
			Why: "no --server flag, GLOSSA_SERVER or glossa.yaml", Fix: "pass --server https://glossa.example.com"}
	}
	return strings.TrimRight(cfg.Server, "/"), nil
}

// ── login ───────────────────────────────────────────────────────────

type loginJSON struct {
	Schema   string     `json:"schema"`
	Server   string     `json:"server"`
	StoredIn string     `json:"stored_in"`
	Tenant   tenantJSON `json:"tenant"`
}

func runLogin(ctx context.Context, inv *invocation, args []string) error {
	fs := inv.flags("login [--server URL] [--token-stdin]")
	server := fs.String("server", "", "glossa-server URL (default: glossa.yaml's server)")
	fromStdin := fs.Bool("token-stdin", false, "read the token from stdin (for scripts)")
	if _, err := inv.parse(fs, args); err != nil {
		return err
	}
	srv, err := inv.serverFor(*server)
	if err != nil {
		return err
	}
	tok, err := inv.readToken(*fromStdin)
	if err != nil {
		return err
	}
	if !tokenPattern.MatchString(tok) {
		return &Error{Exit: ExitUsage, Code: "invalid_token", What: "that isn't a Glossa API token",
			Why: "tokens look like glossa_api_ followed by 43 characters", Fix: "copy the token Studio showed when you created it"}
	}
	tenant, err := inv.verifyToken(ctx, srv, tok)
	if err != nil {
		return err
	}
	store, err := inv.credentialStore()
	if err != nil {
		return err
	}
	if err := store.Set(srv, tok); err != nil {
		return &Error{Exit: ExitUsage, Code: "store_failed", What: "can't store the token", Where: store.Name(), Why: err.Error(),
			Fix: "set GLOSSA_TOKEN instead"}
	}
	out := loginJSON{Schema: "glossa.cli.login/v1", Server: srv, StoredIn: store.Name(), Tenant: tenant}
	return inv.emit(out, func(p *printer) {
		p.line("%s Logged in to %s as tenant %s (%s)", p.pass(), srv, tenant.Name, tenant.Slug)
		p.line("  token stored in %s", store.Name())
	})
}

func (inv *invocation) readToken(fromStdin bool) (string, error) {
	if fromStdin || !inv.env.Interactive || inv.env.ReadSecret == nil {
		if inv.env.Interactive && !fromStdin {
			inv.out.line("Paste an API token (create one in Studio → Settings → API tokens):")
		}
		line, err := bufio.NewReader(inv.env.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		if strings.TrimSpace(line) == "" {
			return "", &Error{Exit: ExitUsage, Code: "no_token", What: "no token given",
				Fix: "pipe the token in: echo \"$GLOSSA_TOKEN\" | glossa login --token-stdin"}
		}
		return strings.TrimSpace(line), nil
	}
	tok, err := inv.env.ReadSecret("API token (Studio → Settings → API tokens): ")
	return strings.TrimSpace(tok), err
}

// verifyToken asks the server which tenant the token belongs to.
func (inv *invocation) verifyToken(ctx context.Context, server, tok string) (tenantJSON, error) {
	c, err := newClientWithToken(inv, server, tok)
	if err != nil {
		return tenantJSON{}, err
	}
	tenants, err := c.Tenants(ctx)
	if err != nil {
		return tenantJSON{}, inv.apiError(err, "the server didn't accept the token")
	}
	if len(tenants) == 0 {
		return tenantJSON{}, &Error{Exit: ExitNetwork, Code: "no_tenant", What: "the token has no tenant"}
	}
	t := tenants[0]
	return tenantJSON{ID: t.Id, Slug: t.Slug, Name: t.Name, Kind: string(t.Kind)}, nil
}

// ── logout ──────────────────────────────────────────────────────────

func runLogout(_ context.Context, inv *invocation, args []string) error {
	fs := inv.flags("logout [--server URL]")
	server := fs.String("server", "", "glossa-server URL (default: glossa.yaml's server)")
	if _, err := inv.parse(fs, args); err != nil {
		return err
	}
	srv, err := inv.serverFor(*server)
	if err != nil {
		return err
	}
	store, err := inv.credentialStore()
	if err != nil {
		return err
	}
	err = store.Delete(srv)
	removed := err == nil
	if err != nil && !errors.Is(err, credentials.ErrNotFound) {
		return &Error{Exit: ExitUsage, Code: "store_failed", What: "can't remove the token", Where: store.Name(), Why: err.Error()}
	}
	out := map[string]any{"schema": "glossa.cli.logout/v1", "server": srv, "removed": removed}
	return inv.emit(out, func(p *printer) {
		if removed {
			p.line("%s Removed the token for %s", p.pass(), srv)
		} else {
			p.line("No token was stored for %s", srv)
		}
	})
}

// ── whoami ──────────────────────────────────────────────────────────

type whoamiJSON struct {
	Schema      string       `json:"schema"`
	Server      string       `json:"server"`
	Token       string       `json:"token"`
	TokenSource string       `json:"token_source"`
	Tenant      tenantJSON   `json:"tenant"`
	Project     *projectJSON `json:"project,omitempty"`
}

type projectJSON struct {
	ID           string `json:"id"`
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	SourceLocale string `json:"source_locale"`
}

func runWhoami(ctx context.Context, inv *invocation, args []string) error {
	fs := inv.flags("whoami [--server URL]")
	server := fs.String("server", "", "glossa-server URL (default: glossa.yaml's server)")
	if _, err := inv.parse(fs, args); err != nil {
		return err
	}
	srv, err := inv.serverFor(*server)
	if err != nil {
		return err
	}
	tok, source, err := inv.token(srv)
	if err != nil {
		return err
	}
	tenant, err := inv.verifyToken(ctx, srv, tok)
	if err != nil {
		return err
	}
	out := whoamiJSON{Schema: "glossa.cli.whoami/v1", Server: srv, Token: redact(tok), TokenSource: source, Tenant: tenant}
	if cfg, err := inv.loadConfig(); err == nil && *server == "" {
		if p, err := inv.connectWith(ctx, cfg); err == nil {
			out.Project = &projectJSON{ID: p.info.Id, Slug: p.info.Slug, Name: p.info.Name, SourceLocale: p.info.SourceLocale}
		}
	}
	return inv.emit(out, func(p *printer) {
		p.line("server   %s", srv)
		p.line("tenant   %s (%s, %s)", tenant.Name, tenant.Slug, tenant.ID)
		p.line("token    %s  %s", redact(tok), p.dim("from "+source))
		if out.Project != nil {
			p.line("project  %s (%s, source %s)", out.Project.Name, out.Project.Slug, out.Project.SourceLocale)
		}
	})
}

// redact keeps a token recognizable without revealing it.
func redact(tok string) string {
	const keep = len("glossa_api_") + 4
	if len(tok) <= keep {
		return "…"
	}
	return tok[:keep] + "…"
}
