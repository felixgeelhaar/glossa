package app

import (
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/integration/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/checkpolicy"
)

// Rendering the Glossa check (RFC 0004 §6.4): what it reports, which
// findings become annotations, and what the sticky comment says.
//
// Nothing here talks to GitHub or to a database. It is a pure function
// from what the other contexts said to the check run's output and the
// comment's Markdown, which is what makes the report testable on its
// own and the worker short.

// maxSummaryFindings caps each list in the summary. A pull request that
// breaks two hundred keys is explained by the first twenty, and the
// annotations carry the rest to the diff.
const maxSummaryFindings = 20

// MaxAnnotations caps what one check run is sent. GitHub takes 50 per
// request and the adapter batches them; this bounds the whole report,
// because a check that annotates a thousand lines helps nobody.
const MaxAnnotations = 200

// CheckInput is everything the report is rendered from, for one Git
// connection's project.
type CheckInput struct {
	Policy  checkpolicy.Policy
	Status  BranchStatus
	Quality BranchQuality
	Usages  BranchUsages
}

// CheckReport is the rendered check for one Git connection.
type CheckReport struct {
	Findings         []CheckFinding
	Errors, Warnings int
	Conclusion       string
	Title            string
	Summary          string
	Annotations      []CheckAnnotation
}

// BuildCheckReport turns what the contexts said into the check's
// verdict, summary and annotations.
func BuildCheckReport(in CheckInput) CheckReport {
	r := CheckReport{Findings: findings(in)}
	for _, f := range r.Findings {
		if f.Severity == checkpolicy.Error {
			r.Errors++
		} else {
			r.Warnings++
		}
	}
	r.Conclusion = ConclusionSuccess
	for _, f := range r.Findings {
		if in.Policy.Fails(f.Severity) {
			r.Conclusion = ConclusionFailure
			break
		}
	}
	r.Title = checkTitle(r)
	r.Summary = checkSummary(in, r)
	r.Annotations = annotations(r.Findings)
	return r
}

// findings collects every finding the check reports, in the order the
// summary lists them.
func findings(in CheckInput) []CheckFinding {
	// A key that is not in the catalog is, by definition, unknown to it,
	// so its usages carry the file:line the annotations need. That is
	// what locates an invalid message in the product's own source.
	where := map[string]UnknownKey{}
	for _, u := range in.Usages.Unknown {
		if _, seen := where[u.Key]; !seen {
			where[u.Key] = u
		}
	}
	var out []CheckFinding
	add := func(f CheckFinding) {
		if u, ok := where[f.Key]; ok && !f.Located() {
			f.File, f.Line = u.File, u.Line
		}
		out = append(out, f)
	}
	for _, m := range in.Status.Invalid {
		add(CheckFinding{
			Code: checkpolicy.CodeInvalidMessage, Severity: checkpolicy.Error, Key: m.Key,
			Message: "invalid message (" + m.Code + "): " + m.Detail,
		})
	}
	for _, c := range in.Status.Conflicts {
		add(CheckFinding{
			Code: checkpolicy.CodeKeyConflict, Severity: checkpolicy.Error, Key: c.Key,
			Message: "another open branch proposes this key with different source: " + strings.Join(c.Branches, ", "),
		})
	}
	for _, l := range in.Quality.Locales {
		n := in.Quality.Untranslated[l]
		if n == 0 {
			continue
		}
		add(CheckFinding{
			Code: checkpolicy.CodeMissingTranslation, Severity: in.Policy.Severity(l), Locale: l,
			Message: plural(n, "new key", "new keys") + " untranslated in " + l,
		})
	}
	out = append(out, in.Quality.Findings...)
	for _, u := range in.Usages.Unknown {
		out = append(out, CheckFinding{
			Code: checkpolicy.CodeUnknownKey, Severity: checkpolicy.Warning, Key: u.Key,
			Message: "no message with this key: " + u.Key, File: u.File, Line: u.Line,
		})
	}
	for _, l := range sortedKeys(in.Status.Outdated) {
		n := in.Status.Outdated[l]
		if n == 0 {
			continue
		}
		out = append(out, CheckFinding{
			Code: checkpolicy.CodeOutdatedTranslation, Severity: checkpolicy.Warning, Locale: l,
			Message: plural(n, "translation", "translations") + " in " + l + " will be outdated when this merges",
		})
	}
	return out
}

func checkTitle(r CheckReport) string {
	switch {
	case r.Errors > 0 && r.Warnings > 0:
		return plural(r.Errors, "problem", "problems") + " and " + plural(r.Warnings, "warning", "warnings")
	case r.Errors > 0:
		return plural(r.Errors, "problem", "problems")
	case r.Warnings > 0:
		return plural(r.Warnings, "warning", "warnings")
	}
	return "Localization is ready"
}

// checkSummary is the check run's Markdown: what the branch changes, a
// table per locale, and the findings grouped by what they are.
func checkSummary(in CheckInput, r CheckReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**%s** proposes %s and %s; %s.\n\n",
		mdCode(in.Status.Name),
		plural(len(in.Status.NewKeys), "new key", "new keys"),
		plural(len(in.Status.SourceProposals), "source change", "source changes"),
		plural(len(in.Status.Removed), "key is gone", "keys are gone"))
	b.WriteString(localeTable(in))
	writeFindingGroup(&b, "Invalid messages", r.Findings, checkpolicy.CodeInvalidMessage)
	writeFindingGroup(&b, "Key conflicts", r.Findings, checkpolicy.CodeKeyConflict)
	writeFindingGroup(&b, "Unknown keys", r.Findings, checkpolicy.CodeUnknownKey)
	writeQAGroup(&b, r.Findings)
	if r.Errors == 0 && r.Warnings == 0 {
		b.WriteString("\nNothing to report: every new key is translated and nothing is out of place.\n")
	}
	return b.String()
}

// localeTable is the per-locale table RFC 0004 §6.4 asks for: how far
// the branch's new keys have got, and what merging it will make
// outdated.
func localeTable(in CheckInput) string {
	locales := slices.Clone(in.Quality.Locales)
	for l := range in.Status.Outdated {
		if !slices.Contains(locales, l) {
			locales = append(locales, l)
		}
	}
	sort.Strings(locales)
	if len(locales) == 0 {
		return ""
	}
	newKeys := len(in.Status.NewKeys)
	var b strings.Builder
	b.WriteString("| Locale | New keys translated | Untranslated | Will be outdated | Required |\n")
	b.WriteString("| --- | ---: | ---: | ---: | :---: |\n")
	for _, l := range locales {
		missing := in.Quality.Untranslated[l]
		required := "—"
		if in.Policy.Requires(l) {
			required = "yes"
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %s |\n",
			l, newKeys-missing, missing, in.Status.Outdated[l], required)
	}
	b.WriteString("\n")
	return b.String()
}

func writeFindingGroup(b *strings.Builder, heading string, fs []CheckFinding, code string) {
	var group []CheckFinding
	for _, f := range fs {
		if f.Code == code {
			group = append(group, f)
		}
	}
	if len(group) == 0 {
		return
	}
	fmt.Fprintf(b, "**%s** (%d)\n\n", heading, len(group))
	for i, f := range group {
		if i == maxSummaryFindings {
			fmt.Fprintf(b, "- … and %d more\n", len(group)-i)
			break
		}
		b.WriteString("- " + mdCode(f.Key) + " — " + mdEscape(f.Message))
		if f.Located() {
			b.WriteString(" (" + mdCode(f.File+":"+strconv.Itoa(f.Line)) + ")")
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

// writeQAGroup lists the QA findings (terminology, max_length), which
// are the ones the other groups did not claim.
func writeQAGroup(b *strings.Builder, fs []CheckFinding) {
	claimed := map[string]bool{
		checkpolicy.CodeInvalidMessage: true, checkpolicy.CodeKeyConflict: true,
		checkpolicy.CodeUnknownKey: true, checkpolicy.CodeMissingTranslation: true,
		checkpolicy.CodeOutdatedTranslation: true,
	}
	var group []CheckFinding
	for _, f := range fs {
		if !claimed[f.Code] {
			group = append(group, f)
		}
	}
	if len(group) == 0 {
		return
	}
	fmt.Fprintf(b, "**Quality findings** (%d)\n\n", len(group))
	for i, f := range group {
		if i == maxSummaryFindings {
			fmt.Fprintf(b, "- … and %d more\n", len(group)-i)
			break
		}
		b.WriteString("- " + mdCode(f.Key))
		if f.Locale != "" {
			b.WriteString(" _" + mdEscape(f.Locale) + "_")
		}
		b.WriteString(" — " + mdEscape(f.Message) + "\n")
	}
	b.WriteString("\n")
}

// annotations turns the located findings into GitHub annotations,
// capped. A finding without a location stays in the summary only: an
// annotation has to point at a line of the product's source.
func annotations(fs []CheckFinding) []CheckAnnotation {
	var out []CheckAnnotation
	for _, f := range fs {
		if !f.Located() || len(out) == MaxAnnotations {
			continue
		}
		level := "warning"
		if f.Severity == checkpolicy.Error {
			level = "failure"
		}
		title := f.Code
		if f.Key != "" {
			title = f.Code + ": " + f.Key
		}
		out = append(out, CheckAnnotation{
			Path: f.File, StartLine: f.Line, EndLine: f.Line, Level: level,
			Title: title, Message: f.Message,
		})
	}
	return out
}

// UnsentAnnotations keeps the annotations this check run has not been
// sent yet, and their fingerprints.
//
// This is the whole of the annotation rule: GitHub *appends* what a
// PATCH carries rather than replacing it, so sending the same
// annotation twice shows it twice. The check run is the unit — a new
// commit or a rerequest makes a new run, whose ledger starts empty —
// and within one run every annotation goes exactly once, however often
// the job runs.
func UnsentAnnotations(t domain.CheckTarget, all []CheckAnnotation) (send []CheckAnnotation, fingerprints []string) {
	for _, a := range all {
		f := domain.AnnotationFingerprint(a.Path, a.StartLine, a.Message)
		if t.Sent(f) || slices.Contains(fingerprints, f) {
			continue
		}
		send = append(send, a)
		fingerprints = append(fingerprints, f)
	}
	return send, fingerprints
}

// ── a pull request from a fork ───────────────────────────────────────

// ForkCheckTitle and forkCheckSummary are what the check says about a
// pull request whose head lives in another repository (RFC 0004 §6.3).
//
// The wait for CI is not mentioned, because there is none: a fork's
// workflow gets no OIDC token and no secrets, so `glossa push` and the
// usages upload cannot run at all, however long anyone waits. Saying
// only that would leave a maintainer stuck, so the summary names the
// two ways forward.
const ForkCheckTitle = "A pull request from a fork has no Glossa CI token"

const forkCheckSummary = "A pull request from a fork gets no Glossa CI token, so its workflow cannot " +
	"upload this commit's messages or usages and there is nothing for Glossa to check.\n\n" +
	"A maintainer can re-run this work from a branch in this repository, where CI does get a token, " +
	"or merge the pull request and let the default branch's push check it.\n"

// ForkCheckReport is the check a fork's pull request gets: `neutral`,
// straight away, with the explanation above.
func ForkCheckReport() CheckReport {
	return CheckReport{Conclusion: ConclusionNeutral, Title: ForkCheckTitle, Summary: forkCheckSummary}
}

// ForkComment is the sticky comment for a fork's pull request. It says
// what the check run says and nothing else: the per-locale table and
// the capture counts describe a branch Glossa is tracking, and a fork's
// branch is not one.
func ForkComment(project string, r CheckReport) string {
	return "### Glossa — " + mdEscape(project) + "\n\n⚪ " + r.Title + ".\n\n" + r.Summary
}

// ── the sticky comment ───────────────────────────────────────────────

// CommentLinks are the places the sticky comment points at.
type CommentLinks struct {
	// Studio is the branch's view in Studio.
	Studio string
	// Preview is the product's own preview deployment, if CI registered
	// one (`glossa preview register --url`).
	Preview string
	// Manifest is the branch environment's manifest at the edge.
	Manifest string
}

// StickyComment renders the one comment a pull request gets (RFC 0004
// §6.4): where to look, what CI captured, and the per-locale table.
// Several Git connections in one repository share it, so it is written
// per project section.
func StickyComment(project string, links CommentLinks, in CheckInput, r CheckReport) string {
	var b strings.Builder
	b.WriteString("### Glossa — " + mdEscape(project) + "\n\n")
	b.WriteString(verdictLine(r) + "\n\n")
	if line := linkLine(links); line != "" {
		b.WriteString(line + "\n\n")
	}
	fmt.Fprintf(&b, "%s of the branch's messages are captured, %d are not.\n\n",
		plural(in.Usages.Captured, "message", "messages"), in.Usages.NotCaptured)
	if in.Usages.Builds == 0 {
		b.WriteString("_No Glossa CI run has uploaded usages for this commit yet._\n\n")
	}
	b.WriteString(localeTable(in))
	return b.String()
}

func verdictLine(r CheckReport) string {
	switch r.Conclusion {
	case ConclusionFailure:
		return "❌ " + checkTitle(r) + "."
	case ConclusionNeutral:
		return "⚪ No Glossa CI run for this commit."
	}
	if r.Warnings > 0 {
		return "✅ Ready, with " + plural(r.Warnings, "warning", "warnings") + "."
	}
	return "✅ Ready."
}

func linkLine(l CommentLinks) string {
	var parts []string
	for _, p := range [][2]string{{"Branch in Studio", l.Studio}, {"Preview", l.Preview}, {"Manifest", l.Manifest}} {
		if p[1] != "" {
			parts = append(parts, "["+p[0]+"]("+p[1]+")")
		}
	}
	return strings.Join(parts, " · ")
}

// StudioBranchURL is where a person opens the branch in Studio. The
// branch rides as a query parameter on the project's workspace, which
// is where the branch view lives.
func StudioBranchURL(base string, tenant, project uuid.UUID, branch string) string {
	base = strings.TrimRight(base, "/")
	if base == "" {
		return ""
	}
	return base + "/t/" + tenant.String() + "/p/" + project.String() + "/translate?branch=" +
		url.QueryEscape(branch)
}

// ── small helpers ────────────────────────────────────────────────────

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// mdCode renders s as inline code, or a dash when it is empty. A key
// may hold a backtick, so the fence grows past the longest run in it.
func mdCode(s string) string {
	if s == "" {
		return "—"
	}
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			longest = max(longest, run)
			continue
		}
		run = 0
	}
	fence := strings.Repeat("`", longest+1)
	pad := ""
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		pad = " "
	}
	return fence + pad + s + pad + fence
}

// mdEscape keeps text the server did not write from becoming Markdown:
// a detail or a locale is quoted content, not formatting.
func mdEscape(s string) string {
	r := strings.NewReplacer(
		"\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]",
		"<", "&lt;", ">", "&gt;", "|", "\\|", "\n", " ", "\r", " ",
	)
	return r.Replace(s)
}
