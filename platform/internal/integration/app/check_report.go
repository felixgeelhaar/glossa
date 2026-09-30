package app

import (
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/integration/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/checkpolicy"
	quality "github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
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
//
// The policy is the project's, read from the server. There is no local
// override here and there never will be: `glossa.yaml` and the
// command's flags are a developer's own loop, and a pull request grades
// against what the project says (RFC 0005 §4.2, §14 decision 3).
type CheckInput struct {
	Policy  checkpolicy.Policy
	Status  BranchStatus
	Quality BranchQuality
	Usages  BranchUsages
	// OpenedAt is when the pull request was opened, for the grace a
	// stricter policy ships with: a pull request older than the save
	// keeps grading against the version it opened under until the grace
	// runs out (RFC 0005 §4.3). The zero time is "not a pull request, or
	// nobody recorded when", and then the current version grades it.
	OpenedAt time.Time
	// Now is when the check runs. A check that read a clock of its own
	// could grade a re-run differently from the run.
	Now time.Time
}

// CheckReport is the rendered check for one Git connection.
type CheckReport struct {
	Findings         []quality.Finding
	Errors, Warnings int
	// Waived counts the findings a waiver accepted. They are reported
	// and never fail the run (RFC 0005 §2.3).
	Waived     int
	Conclusion string
	Title      string
	Summary    string
	// Layers are the reported findings counted per layer, in report
	// order. It is the breakdown RFC 0005 §12.3 asks the pull request and
	// the terminal to agree on, computed by the Quality context for both
	// of them.
	Layers []quality.LayerCount
	// PolicyVersion is the version of the policy document this check
	// graded itself against, and Pinned says it is not the project's
	// current one because the pull request predates it. CurrentVersion
	// is the project's own. The summary says all three, because a check
	// whose answer changed under someone has to be able to explain
	// itself.
	PolicyVersion  int
	CurrentVersion int
	Pinned         bool
	GraceUntil     time.Time
	Annotations    []CheckAnnotation
}

// BuildCheckReport turns what the contexts said into the check's
// verdict, summary and annotations.
func BuildCheckReport(in CheckInput) CheckReport {
	// The version that grades this pull request, which is the current
	// one unless a grace pins it to the one it was opened under.
	pinned := in.Policy.Pins(in.OpenedAt, in.Now)
	var grace time.Time
	if pinned {
		grace = *in.Policy.GraceUntil
	}
	current := in.Policy.Version
	in.Policy = in.Policy.Effective(in.OpenedAt, in.Now)
	r := CheckReport{
		Findings:      findings(in),
		PolicyVersion: in.Policy.Version, CurrentVersion: current, Pinned: pinned, GraceUntil: grace,
	}
	// The policy stays the evaluator: it grades every finding for this
	// locale and namespace, drops the ones a rule switched off, ignores
	// the ones a rule is still only warning about, and a waived finding
	// is counted on its own and can never fail a run (RFC 0005 §2.3).
	ev := quality.Evaluate(in.Policy, checkEnvironment, r.Findings)
	r.Findings = ev.Findings()
	r.Errors, r.Warnings, r.Waived = ev.Counts.Errors, ev.Counts.Warnings, ev.Counts.Waived
	r.Layers = quality.ByLayer(r.Findings)
	r.Conclusion = string(ev.Conclusion)
	r.Title = checkTitle(r)
	r.Summary = checkSummary(in, r)
	r.Annotations = annotations(r.Findings)
	return r
}

// checkEnvironment is the environment a pull-request check runs in:
// none. A branch is not an environment, so a rule that names one says
// nothing here, and the document's own require_complete applies.
const checkEnvironment = ""

// findings collects every finding the check reports, in the order the
// summary lists them.
//
// The Quality context's findings are the report's spine: the server
// computes them with the same layers `glossa check` links, so a finding
// a translator saw is the finding on the pull request, with its locus,
// its span and its evidence intact. What the check adds is the
// per-locale roll-up for the things the read model only counts — and it
// adds one only where no layered finding already says it, because a
// finding stated twice is counted twice, and a pull request whose
// numbers differ from the terminal's is the one failure this milestone
// exists to prevent (RFC 0005 §12.3).
func findings(in CheckInput) []quality.Finding {
	// A key that is not in the catalog is, by definition, unknown to it,
	// so its usages carry the file:line the annotations need. That is
	// what locates an invalid message in the product's own source.
	where := map[string]UnknownKey{}
	for _, u := range in.Usages.Unknown {
		if _, seen := where[u.Key]; !seen {
			where[u.Key] = u
		}
	}
	said := alreadyReported(in.Quality.Findings)
	var out []quality.Finding
	add := func(f quality.Finding) {
		if u, ok := where[f.Locus.Key]; ok && !f.Located() {
			f.Locus.File, f.Locus.Line = u.File, u.Line
		}
		out = append(out, f)
	}
	for _, m := range in.Status.Invalid {
		if said.key(checkpolicy.CodeInvalidMessage, m.Key) {
			continue
		}
		add(quality.New(quality.Finding{
			Layer: quality.LayerStructure, Code: checkpolicy.CodeInvalidMessage, Severity: checkpolicy.Error,
			Locus: quality.Locus{Key: m.Key}, Detail: m.Code,
			Message: "invalid message (" + m.Code + "): " + m.Detail,
		}))
	}
	for _, c := range in.Status.Conflicts {
		if said.key(checkpolicy.CodeKeyConflict, c.Key) {
			continue
		}
		add(quality.New(quality.Finding{
			Layer: quality.LayerCompleteness, Code: checkpolicy.CodeKeyConflict, Severity: checkpolicy.Error,
			Locus:   quality.Locus{Key: c.Key},
			Message: "another open branch proposes this key with different source: " + strings.Join(c.Branches, ", "),
		}))
	}
	for _, l := range in.Quality.Locales {
		n := in.Quality.Untranslated[l]
		if n == 0 || said.locale(checkpolicy.CodeMissingTranslation, l) {
			continue
		}
		add(quality.New(quality.Finding{
			Layer: quality.LayerCompleteness, Code: checkpolicy.CodeMissingTranslation,
			Severity: in.Policy.Severity(l), Locus: quality.Locus{Locale: l},
			Message: plural(n, "new key", "new keys") + " untranslated in " + l,
		}))
	}
	for _, f := range in.Quality.Findings {
		add(f)
	}
	for _, u := range in.Usages.Unknown {
		if said.key(checkpolicy.CodeUnknownKey, u.Key) {
			continue
		}
		out = append(out, quality.New(quality.Finding{
			Layer: quality.LayerCompleteness, Code: checkpolicy.CodeUnknownKey, Severity: checkpolicy.Warning,
			Locus:   quality.Locus{Key: u.Key, File: u.File, Line: u.Line},
			Message: "no message with this key: " + u.Key,
		}))
	}
	for _, l := range sortedKeys(in.Status.Outdated) {
		n := in.Status.Outdated[l]
		if n == 0 || said.locale(checkpolicy.CodeOutdatedTranslation, l) {
			continue
		}
		out = append(out, quality.New(quality.Finding{
			Layer: quality.LayerCompleteness, Code: checkpolicy.CodeOutdatedTranslation,
			Severity: checkpolicy.Warning, Locus: quality.Locus{Locale: l},
			Message: plural(n, "translation", "translations") + " in " + l + " will be outdated when this merges",
			Fix:     &quality.Fix{Kind: quality.FixAdoptSourceChange},
		}))
	}
	return out
}

// reported is what the layered findings already say, so the check does
// not roll up the same thing a second time.
type reported struct {
	keys    map[string]bool
	locales map[string]bool
}

func alreadyReported(fs []quality.Finding) reported {
	r := reported{keys: map[string]bool{}, locales: map[string]bool{}}
	for _, f := range fs {
		if f.Locus.Key != "" {
			r.keys[f.Code+"\x00"+f.Locus.Key] = true
		}
		if f.Locus.Locale != "" {
			r.locales[f.Code+"\x00"+f.Locus.Locale] = true
		}
	}
	return r
}

func (r reported) key(code, k string) bool { return r.keys[code+"\x00"+k] }

func (r reported) locale(code, l string) bool { return r.locales[code+"\x00"+l] }

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
// table per locale, the counts per layer, and the findings grouped by
// the layer that found them.
//
// Layer is the grouping because layer is what the policy selects on and
// what the terminal prints: a reader who wants to know why the build is
// red, and a reader comparing the pull request with `glossa check`, are
// both asking a per-layer question (RFC 0005 §2.1, §12.3).
func checkSummary(in CheckInput, r CheckReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**%s** proposes %s and %s; %s.\n\n",
		mdCode(in.Status.Name),
		plural(len(in.Status.NewKeys), "new key", "new keys"),
		plural(len(in.Status.SourceProposals), "source change", "source changes"),
		plural(len(in.Status.Removed), "key is gone", "keys are gone"))
	b.WriteString(localeTable(in))
	b.WriteString(layerTable(r))
	writeLayerGroups(&b, r)
	writeWaived(&b, r)
	if len(r.Findings) == 0 {
		b.WriteString("\nNothing to report: every new key is translated and nothing is out of place.\n")
	}
	writePolicyNote(&b, r)
	return b.String()
}

// layerTable is the per-layer breakdown, with the run's totals under
// it. It is the number RFC 0005 §12.3 makes the milestone's exit
// criterion — the same commit's `glossa check` prints the same
// arithmetic — so it is a table a person and a test can both read, and
// the totals row is there because a breakdown that doesn't add up is
// how the two surfaces would drift apart unnoticed.
func layerTable(r CheckReport) string {
	if len(r.Layers) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("**Findings by layer**\n\n")
	b.WriteString("| Layer | Errors | Warnings | Waived |\n")
	b.WriteString("| --- | ---: | ---: | ---: |\n")
	for _, l := range r.Layers {
		fmt.Fprintf(&b, "| %s | %d | %d | %d |\n", l.Layer, l.Counts.Errors, l.Counts.Warnings, l.Counts.Waived)
	}
	fmt.Fprintf(&b, "| **Total** | **%d** | **%d** | **%d** |\n\n", r.Errors, r.Warnings, r.Waived)
	return b.String()
}

// writeLayerGroups lists each layer's findings under its own heading.
// Waived findings are not here: they have their own section, because
// mixing an accepted finding into the layer that failed the build is
// how a waiver starts looking like a failure.
func writeLayerGroups(b *strings.Builder, r CheckReport) {
	for _, l := range r.Layers {
		var group []quality.Finding
		for _, f := range r.Findings {
			if f.Layer == l.Layer && f.Severity != quality.Waived {
				group = append(group, f)
			}
		}
		if len(group) == 0 {
			continue
		}
		// The tally counts the group, not the layer: the waived findings
		// are counted in the table above and listed in their own
		// section, and a heading promising three findings over a list of
		// two is a heading nobody trusts again.
		var c quality.Counts
		for _, f := range group {
			c.Count(f)
		}
		fmt.Fprintf(b, "**%s** (%s)\n\n", layerHeading(l.Layer), layerTally(c))
		writeFindings(b, group)
	}
}

// writeWaived lists the findings a waiver accepted, on their own and in
// plain sight. A waived finding is still computed and still reported:
// hiding it would make the number go down without the product getting
// better, which is the failure mode of every suppression system
// (RFC 0005 §2.3).
func writeWaived(b *strings.Builder, r CheckReport) {
	var group []quality.Finding
	for _, f := range r.Findings {
		if f.Severity == quality.Waived {
			group = append(group, f)
		}
	}
	if len(group) == 0 {
		return
	}
	fmt.Fprintf(b, "**Accepted by a waiver** (%d)\n\n", len(group))
	b.WriteString("Still found, still reported, and not counted against this check.\n\n")
	writeFindings(b, group)
}

// layerHeading is a layer's name as a heading: the layer's own
// spelling, which is what the policy and `--layer` use, with a capital.
func layerHeading(l quality.Layer) string {
	s := string(l)
	if s == "" {
		return "Other"
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// layerTally is a group's counts in words, leaving out the zeroes.
func layerTally(c quality.Counts) string {
	var parts []string
	if c.Errors > 0 {
		parts = append(parts, plural(c.Errors, "error", "errors"))
	}
	if c.Warnings > 0 {
		parts = append(parts, plural(c.Warnings, "warning", "warnings"))
	}
	if c.Waived > 0 {
		parts = append(parts, plural(c.Waived, "waived", "waived"))
	}
	if len(parts) == 0 {
		return "0"
	}
	return strings.Join(parts, ", ")
}

// writeFindings is one group's list, capped.
func writeFindings(b *strings.Builder, group []quality.Finding) {
	for i, f := range group {
		if i == maxSummaryFindings {
			fmt.Fprintf(b, "- … and %d more\n", len(group)-i)
			break
		}
		// A finding about a whole locale has no key, and "—" in front of
		// its locale reads as a missing one rather than as "this is
		// about the locale".
		var label string
		if f.Locus.Key != "" {
			label = mdCode(f.Locus.Key)
		}
		if f.Locus.Locale != "" {
			if label != "" {
				label += " "
			}
			label += "_" + mdEscape(f.Locus.Locale) + "_"
		}
		if label == "" {
			label = "—"
		}
		b.WriteString("- " + label + " — " + mdCode(f.Code) + ": " + mdEscape(f.Message))
		if f.Located() {
			b.WriteString(" (" + mdCode(f.Locus.File+":"+strconv.Itoa(f.Locus.Line)) + ")")
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

// writePolicyNote says which version of the check policy graded this
// run, and — where the pull request predates a stricter version — that
// it is being graded against the version it was opened under, and until
// when (RFC 0005 §4.3).
//
// A policy with no version says nothing, which is every policy stored
// before M4: a project that has not written a document is graded
// exactly as it always was, and the comment does not grow a line about
// machinery it does not use.
func writePolicyNote(b *strings.Builder, r CheckReport) {
	if r.PolicyVersion == 0 {
		return
	}
	fmt.Fprintf(b, "\nGraded against the project's check policy v%d", r.PolicyVersion)
	if r.Pinned {
		fmt.Fprintf(b, " — the version this pull request was opened under. v%d is the project's current"+
			" policy and takes this pull request over on %s",
			r.CurrentVersion, r.GraceUntil.UTC().Format("2006-01-02"))
	}
	b.WriteString(".\n")
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

// annotations turns the located findings into GitHub annotations,
// capped. A finding whose locus carries a file and a line goes on the
// diff; the rest are summary-only, because an annotation has to point
// at a line of the product's source and a finding about a translation
// often has none.
//
// A waived finding is annotated too, at `notice`: it is accepted, so it
// must not look like a live warning, and it is not hidden, so it must
// not vanish from the one place a reviewer is actually looking.
func annotations(fs []quality.Finding) []CheckAnnotation {
	var out []CheckAnnotation
	for _, f := range fs {
		if !f.Located() || len(out) == MaxAnnotations {
			continue
		}
		level := "warning"
		switch f.Severity {
		case checkpolicy.Error:
			level = "failure"
		case quality.Waived:
			level = "notice"
		}
		title := f.Code
		if f.Locus.Key != "" {
			title = f.Code + ": " + f.Locus.Key
		}
		message := f.Message
		if f.Severity == quality.Waived {
			message = "waived: " + message
		}
		out = append(out, CheckAnnotation{
			Path: f.Locus.File, StartLine: f.Locus.Line, EndLine: f.Locus.Line, Level: level,
			Title: title, Message: message,
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
