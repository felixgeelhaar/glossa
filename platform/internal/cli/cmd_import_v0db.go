package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/felixgeelhaar/glossa/platform/internal/cli/v0"
)

// importV0DB is `glossa import --from v0 --v0-db DSN` (RFC 0006 §7.2):
// the import of a restored v0.3 backup. It writes what the platform's
// API takes — locales, messages with their descriptions, translations
// with v0.3's provenance — and reports, without acting on them, the
// plans other waves complete: users as invitations (Identity, wave 3)
// and history as audit entries (Audit, wave 4).
func (inv *invocation) importV0DB(ctx context.Context, fs *flag.FlagSet, f importFlags) error {
	for _, name := range []string{"v0-url", "v0-key-env"} {
		if isSet(fs, name) {
			return usageError(inv.name, "--%s belongs to an API import; --v0-db reads a restored backup instead", name)
		}
	}
	only, err := localeSet(inv, f.locales)
	if err != nil {
		return err
	}
	p, err := inv.connect(ctx)
	if err != nil {
		return err
	}
	f.project = orDefault(f.project, p.cfg.Project)
	snap, err := v0.ReadRestore(ctx, f.db, f.tenant, f.project)
	if err != nil {
		return v0DBError(err, f.db)
	}
	plan, err := v0.BuildDBPlan(p.info.SourceLocale, snap, only)
	if err != nil {
		return &Error{Exit: ExitUsage, Code: "source_locale_missing", What: "can't import this v0.3 project", Why: err.Error(),
			Fix: "import into a project whose source locale the v0.3 project has"}
	}
	restore := plan.Restore
	out := importJSON{Schema: "glossa.cli.import/v1", From: "v0", DryRun: f.dryRun, LocalesAdded: []string{},
		Source: plan.ReportSource(f.db), Restore: &restore, Locales: plan.LocaleInfo, Invitations: plan.Invitations,
		AuditEntries: plan.AuditEntries, NotCarried: plan.NotCarried, Warnings: plan.Warnings}
	if f.dryRun {
		out.Items = planItems(plan.Plan)
	} else if out, err = inv.applyImport(ctx, p, plan.Plan, plan.OriginDetail(), out); err != nil {
		return err
	}
	out.Summary = importSummary(out.Items)
	if err := inv.emit(out, func(pr *printer) { printImport(pr, out) }); err != nil {
		return err
	}
	if out.Summary["message"]["failed"]+out.Summary["translation"]["failed"] > 0 {
		return silentExit(ExitPartial, "partial_failure")
	}
	return nil
}

// v0DBError explains a restore the importer can't or won't read.
func v0DBError(err error, dsn string) error {
	where := v0.DescribeDSN(dsn)
	var (
		nr  *v0.NotARestoreError
		se  *v0.SchemaError
		rs  *v0.RowSecurityError
		pnf *v0.ProjectNotFoundError
		amb *v0.AmbiguousProjectError
	)
	switch {
	case errors.As(err, &nr):
		return &Error{Exit: ExitUsage, Code: "v0_not_a_restore", What: "refusing to read this database: it is not a marked v0.3 restore",
			Where: where, Why: nr.Reason, Err: err,
			Fix: "restore the v0.3 backup into a new database with " + v0.RestoreScript + " and point --v0-db at that database — never at v0.3's own"}
	case errors.As(err, &se):
		return &Error{Exit: ExitUsage, Code: "v0_schema_mismatch", What: "the restore is not Glossa v0.3's schema", Where: where, Why: se.Error(), Err: err,
			Fix: "restore a backup of v0.3 at apps/api migration 0006 (the deployed schema)"}
	case errors.As(err, &rs):
		return &Error{Exit: ExitUsage, Code: "v0_row_security", What: "can't read every v0.3 row as this role", Where: where, Why: rs.Error(), Err: err,
			Fix: "connect as the role v0-restore.sh ran as"}
	case errors.As(err, &pnf):
		return &Error{Exit: ExitUsage, Code: "v0_project_not_found", What: pnf.Error(), Where: where, Err: err,
			Fix: "check --v0-project (and --v0-tenant)"}
	case errors.As(err, &amb):
		return &Error{Exit: ExitUsage, Code: "v0_project_ambiguous", What: amb.Error(), Where: where, Err: err,
			Fix: "add --v0-tenant " + amb.Tenants[0] + " (or " + strings.Join(amb.Tenants[1:], ", ") + ")"}
	}
	return &Error{Exit: ExitNetwork, Code: "v0_db_unreachable", What: "can't read the restored v0.3 database", Where: where, Why: err.Error(), Err: err,
		Fix: "check --v0-db (credentials are best passed in PGPASSWORD, not the DSN)"}
}

// printV0Plans prints a --v0-db import's restore, locales, plans and
// what it didn't carry.
func printV0Plans(p *printer, out importJSON) {
	if out.Restore == nil {
		return
	}
	r := out.Restore
	p.line("%s restore %s of %s (sha256 %s…), restored %s", p.pass(), r.Database, r.DumpName, r.DumpSHA256[:min(12, len(r.DumpSHA256))],
		r.RestoredAt.Format("2006-01-02 15:04 MST"))
	var labels []string
	for _, l := range out.Locales {
		s := fmt.Sprintf("%s %q", l.Code, l.Label)
		if !l.Enabled {
			s += " (disabled)"
		}
		labels = append(labels, s)
	}
	p.line("  locales: %s", strings.Join(labels, ", "))
	held := 0
	for _, i := range out.Invitations {
		if i.Status == "held" {
			held++
		}
	}
	p.line("%s %d invitations planned, %d held — not sent: Identity sends them (RFC 0006 wave 3)", p.pass(), len(out.Invitations)-held, held)
	for _, i := range out.Invitations {
		if i.Status == "held" {
			p.line("  %s %s: %s", p.caution(), i.Email, i.Reason)
		}
	}
	unresolved := 0
	for _, e := range out.AuditEntries {
		if e.Unresolved != "" {
			unresolved++
		}
	}
	p.line("%s %d history entries planned as imported audit entries (%d without a translation) — not written: Audit imports them (wave 4)",
		p.pass(), len(out.AuditEntries), unresolved)
	for _, w := range out.Warnings {
		p.line("%s %s", p.caution(), w)
	}
	var not []string
	for _, n := range out.NotCarried {
		not = append(not, n.What)
	}
	p.line("  not carried (see --json for why): %s", strings.Join(not, "; "))
}
