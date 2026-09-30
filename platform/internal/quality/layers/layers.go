// Package layers is the QA every surface runs, one package-level type
// per layer behind the Checker port (RFC 0005 §2.1, §3).
//
// The deterministic layers run everywhere, from this code: in `glossa
// check` (offline, over the local catalogs), in the Glossa pull-request
// check, in the server's write-time job and live in Studio. A finding a
// translator sees while typing is the finding that fails the build,
// because there is one implementation and three callers.
//
// A layer is given a Project — the catalog as the caller sees it — and
// the project's check policy, and answers with findings. It never
// reaches for a file, a database or a provider: `glossa check` must be
// cheap, offline-capable, deterministic and free (RFC 0005 §14
// decision 2), and everything a model decides is a job elsewhere.
//
// Project is Quality's own input type rather than the CLI's snapshot,
// so the server can build one from the catalog without the context
// depending on the command.
package layers

import (
	"sort"

	mf "github.com/felixgeelhaar/glossa/messageformat"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/checkpolicy"
	"github.com/felixgeelhaar/glossa/platform/internal/quality/domain"
)

// Invalid is text that doesn't parse, with the kernel's error code.
type Invalid struct {
	Code   string
	Detail string
}

// Locale is one of the project's locales.
type Locale struct {
	Code     string
	IsSource bool
	// File is the local catalog it came from; empty on the server.
	File string
}

// Message is a source message.
type Message struct {
	// ID is the catalog message's ID. Empty where the caller has only
	// keys, which is the case for an offline `glossa check`.
	ID        string
	Key       string
	Namespace string
	Revision  int
	Model     *mf.Message
	Invalid   *Invalid
	File      string
	// Text is the message as it was authored, in its own syntax. It is
	// what a finding's span points into, in bytes; a caller that kept
	// only the model leaves it empty, and its layers report no span
	// rather than one into a string nobody has.
	Text string
	// MaxLength bounds the rendered translation in characters; 0 means
	// the message has no limit. It is the constraint behind
	// `max-length-exceeded`, which the length layer computes where the
	// caller carries it and relays from the stored warning where it does
	// not (RFC 0005 §3.3).
	MaxLength int
}

// Translation is a message's text in one locale.
type Translation struct {
	Key    string
	Locale string
	Model  *mf.Message
	// State is the review state; empty for local files.
	State string
	// Revision is the translation revision's ID, where the caller knows it.
	Revision string
	// SourceRevision is the source revision the translation was made
	// against.
	SourceRevision int
	Outdated       bool
	// Warnings are the findings the server stored with the translation,
	// which only it can compute (max-length-exceeded).
	Warnings []mf.Finding
	Invalid  *Invalid
	File     string
	// Text is the translation as it was authored, in its own syntax, and
	// what a finding's span points into.
	Text string
}

// Region is one rendered message's measured box on a capture: what the
// layout budget of RFC 0005 §3.3 is computed from, without a browser.
//
// The box was measured while the page showed Locale's text, so the two
// together are a font metric — the advance that region's font gave that
// many characters — and that is the metric the length layer predicts
// other locales' widths from. It is why a region carries the locale it
// was measured in, and why a region measured over text nobody has is
// not a metric at all.
type Region struct {
	// Key is the message that rendered in the box.
	Key string
	// Locale is the locale the page was rendered in when it was
	// measured.
	Locale string
	// Capture and ID identify the region for the locus.
	Capture string
	ID      string
	// Width and Height are the box, in CSS pixels and never device
	// pixels (checkpolicy.LengthThresholds says why).
	Width  float64
	Height float64
	// AdvancePerRunePx is the font's measured average advance per rune,
	// where the caller took one directly. Zero means it did not, and the
	// layer derives the metric from the box and the text it held.
	AdvancePerRunePx float64
}

// Project is the catalog a layer checks: its locales, its active source
// messages and their translations.
type Project struct {
	// Origin says where it was read from ("server", "local").
	Origin string
	// SourceLocale is the locale the messages are written in.
	SourceLocale string
	// Locales has the source locale first, then the rest by code.
	Locales []Locale
	// Messages are the active messages.
	Messages []Message
	// Translations maps locale → key → translation.
	Translations map[string]map[string]Translation
	// Regions are the measured boxes of the captures the caller read,
	// which is what the length layer's layout budget needs. A caller
	// with no capture — `glossa check` — carries none, and the budget is
	// simply not computed: a check that guessed at pixels it never
	// measured would be worse than one that says nothing.
	Regions []Region

	index map[string]int
}

// RegionsFor are the measured regions of key, in a stable order.
func (p *Project) RegionsFor(key string) []Region {
	var out []Region
	for _, r := range p.Regions {
		if r.Key == key {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Locale != out[j].Locale {
			return out[i].Locale < out[j].Locale
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Message returns the source message with key.
func (p *Project) Message(key string) (*Message, bool) {
	if p.index == nil {
		p.index = make(map[string]int, len(p.Messages))
		for i, m := range p.Messages {
			p.index[m.Key] = i
		}
	}
	i, ok := p.index[key]
	if !ok {
		return nil, false
	}
	return &p.Messages[i], true
}

// Identify gives findings measured outside the layers the catalog
// identity their fingerprint is hashed over: the message ID of the key
// each one names, where this project knows one (RFC 0005 §2.1).
//
// Two layers need it. The visual layer's measurements are taken in a
// browser, which holds keys and not the catalog, so a probe finding
// arrives with a key alone; the terminology layer's come from the
// server's termbase check, which reports keys as well. A fingerprint
// over a key is not the one the server computes for the same finding,
// which would leave a waiver made in the terminal not matching the pull
// request. Both of the server's own paths resolve the ID before they
// mint — app.CaptureFindings.findings at the capture ingest, the
// integration context's terminology findings for the PR check — and this
// is how the CLI resolves it.
//
// A finding that already names a message is left alone, and so is one
// whose key this project has never seen: that one keeps falling back to
// the key, which is what domain.Fingerprint does with an empty message.
//
// Findings are copied, never edited in place. A finding that was already
// minted is re-minted, because its identity moved with its locus; one
// that carries no fingerprint yet is left for its layer to mint (the
// visual layer's probe findings, which PromoteVisual seals first).
func (p *Project) Identify(fs []domain.Finding) []domain.Finding {
	out := make([]domain.Finding, 0, len(fs))
	for _, f := range fs {
		if f.Locus.Message == "" && f.Locus.Key != "" {
			if m, ok := p.Message(f.Locus.Key); ok && m.ID != "" {
				f.Locus.Message = m.ID
				if f.Fingerprint != "" {
					f = domain.New(f)
				}
			}
		}
		out = append(out, f)
	}
	return out
}

// TargetLocales are the locales other than the source.
func (p *Project) TargetLocales() []Locale {
	var out []Locale
	for _, l := range p.Locales {
		if !l.IsSource {
			out = append(out, l)
		}
	}
	return out
}

// SortedTranslations are a locale's translations by key, so a layer's
// findings come out in a stable order whatever the map iteration did.
func (p *Project) SortedTranslations(locale string) []Translation {
	trs := p.Translations[locale]
	out := make([]Translation, 0, len(trs))
	for _, t := range trs {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// Checker is one layer of QA.
type Checker interface {
	// Layer names the layer, which is what a policy and a command line
	// select on.
	Layer() domain.Layer
	// Check answers with the layer's findings, in a stable order.
	Check(p *Project, policy checkpolicy.Policy) []domain.Finding
}

// Default is the deterministic QA a check always runs: every layer that
// needs nothing but the project it is handed — no network, no database,
// no browser and no provider.
//
// A layer with nothing to read is silent, not absent. The length layer
// with no measured regions computes no layout budget, and the locale
// layer in a locale whose conventions it has no data for reports
// nothing — the honest answer, and not a green one, because the run
// still names the layer in Report.Layers and a reader can tell "clean"
// from "not looked at".
func Default() []Checker {
	return []Checker{Structure{}, Parity{}, Completeness{}, Length{}, LocaleLayer{}}
}

// Precomputed is a Checker reporting findings computed elsewhere, such
// as the terminology layer, which asks the server.
func Precomputed(layer domain.Layer, fs []domain.Finding) Checker {
	return precomputed{layer: layer, findings: fs}
}

type precomputed struct {
	layer    domain.Layer
	findings []domain.Finding
}

func (c precomputed) Layer() domain.Layer { return c.layer }

func (c precomputed) Check(*Project, checkpolicy.Policy) []domain.Finding { return c.findings }

// sourceRevision is a pointer to t's source revision, which is what a
// waiver is measured against. A translation the caller knows nothing
// about carries none.
func sourceRevision(t Translation) *int {
	if t.SourceRevision == 0 {
		return nil
	}
	rev := t.SourceRevision
	return &rev
}
