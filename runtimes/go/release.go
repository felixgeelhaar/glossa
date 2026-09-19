package glossa

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"

	"github.com/felixgeelhaar/glossa/messageformat"
)

// An activated release and how one is assembled (runtimes/SPEC.md §3):
// verify the manifest, then load and verify every artifact the client
// needs, from the cheapest source that has it. Only a completely assembled
// release is ever activated.

// release is immutable once assembled.
type release struct {
	manifest *manifest
	raw      []byte
	etag     string
	catalogs map[string]catalog // locale → messages of all its namespaces
	bySHA    map[string]catalog // artifact digest → its messages, for reuse
}

// blobSource yields artifact bytes by reference. remote sources are the
// edge: their failures are real errors, while a local source that lacks
// or has corrupted an artifact just defers to the next source.
type blobSource struct {
	remote bool
	get    func(ctx context.Context, ref artifactRef) ([]byte, error)
}

// loadError attaches the release a load failure belongs to.
type loadError struct {
	releaseID string
	err       error
}

func (e *loadError) Error() string { return e.err.Error() }
func (e *loadError) Unwrap() error { return e.err }

func releaseOf(err error) string {
	var le *loadError
	if errors.As(err, &le) {
		return le.releaseID
	}
	return ""
}

// assemble builds a release from manifest bytes. prev, when not nil, lends
// its already parsed artifacts.
func (c *Client) assemble(ctx context.Context, raw []byte, etag string, sources []blobSource, prev *release) (*release, error) {
	m, err := c.verifiedManifest(raw)
	if err != nil {
		return nil, err
	}
	rel := &release{manifest: m, raw: raw, etag: etag, catalogs: map[string]catalog{}, bySHA: map[string]catalog{}}
	for _, locale := range neededLocales(m, c.cfg.Locales) {
		if err := c.loadLocale(ctx, rel, locale, sources, prev); err != nil {
			return nil, &loadError{releaseID: m.Release.ID, err: err}
		}
	}
	return rel, nil
}

func (c *Client) verifiedManifest(raw []byte) (*manifest, error) {
	m, err := parseManifest(raw)
	if err != nil {
		return nil, err
	}
	if c.cfg.Environment != "" && m.Environment != c.cfg.Environment {
		err := fmt.Errorf("%w: manifest is for environment %q, not %q", errSchema, m.Environment, c.cfg.Environment)
		return nil, &loadError{releaseID: m.Release.ID, err: err}
	}
	if err := verifySignatures(raw, m, c.cfg.PublicKeys); err != nil {
		return nil, &loadError{releaseID: m.Release.ID, err: err}
	}
	return m, nil
}

// neededLocales are the locales whose artifacts a client loads: every
// locale by default, since a backend renders for whoever it serves, or the
// fallback chains of the configured locales.
func neededLocales(m *manifest, configured []string) []string {
	if len(configured) == 0 {
		return m.localeCodes()
	}
	var out []string
	for _, tag := range canonicalizeAll(configured) {
		for _, l := range m.chain(m.negotiate([]string{tag})) {
			if !slices.Contains(out, l) {
				out = append(out, l)
			}
		}
	}
	return out
}

func (c *Client) loadLocale(ctx context.Context, rel *release, locale string, sources []blobSource, prev *release) error {
	merged := catalog{}
	namespaces := rel.manifest.Artifacts[locale]
	for _, ns := range slices.Sorted(maps.Keys(namespaces)) {
		ref := namespaces[ns]
		cat, err := c.loadArtifact(ctx, rel.manifest.Release.ID, ref, locale, ns, sources, prev)
		if err != nil {
			return err
		}
		rel.bySHA[ref.SHA256] = cat
		maps.Copy(merged, cat)
	}
	rel.catalogs[locale] = merged
	return nil
}

func (c *Client) loadArtifact(ctx context.Context, releaseID string, ref artifactRef, locale, ns string, sources []blobSource, prev *release) (catalog, error) {
	if prev != nil {
		if cat, ok := prev.bySHA[ref.SHA256]; ok {
			return cat, nil
		}
	}
	body, fromRemote, err := fetchVerified(ctx, ref, sources)
	if err != nil {
		return nil, err
	}
	cat, bad, err := parseArtifact(body, locale, ns)
	if err != nil {
		return nil, err
	}
	for _, b := range bad {
		c.reporter.report(Error{Type: ErrorSchema, Detail: b.err.Error(), MessageID: b.id, Locale: locale, ReleaseID: releaseID})
	}
	if fromRemote {
		if err := c.store.saveArtifact(ref.SHA256, body); err != nil {
			c.cfg.Logger.Warn("glossa: caching an artifact failed", "sha256", ref.SHA256, "error", err)
		}
	}
	return cat, nil
}

// fetchVerified returns the first bytes for ref that match its digest.
func fetchVerified(ctx context.Context, ref artifactRef, sources []blobSource) ([]byte, bool, error) {
	for _, src := range sources {
		body, err := src.get(ctx, ref)
		if err == nil {
			err = verifyArtifact(body, ref)
		}
		switch {
		case err == nil:
			return body, src.remote, nil
		case src.remote:
			return nil, true, err
		}
	}
	return nil, false, fmt.Errorf("%w: artifact %s is not available", errNetwork, ref.SHA256)
}

// resolution is the outcome of resolving one message (SPEC §4.3).
type resolution struct {
	id           string
	requested    []string
	locale       string
	chain        []string
	steps        []Step
	resolvedFrom string // "" when no locale in the chain has the message
	message      messageformat.Message
}

// resolve walks the fallback chain for id. rel may be nil (nothing loaded).
func (rel *release) resolve(id string, requested []string) resolution {
	res := resolution{id: id, requested: requested}
	if rel == nil {
		if len(requested) > 0 {
			res.locale = requested[0]
		}
		return res
	}
	res.locale = rel.manifest.negotiate(requested)
	res.chain = rel.manifest.chain(res.locale)
	for _, l := range res.chain {
		cat, loaded := rel.catalogs[l]
		msg, ok := cat[id]
		switch {
		case !loaded:
			res.steps = append(res.steps, Step{Locale: l, Outcome: OutcomeNotLoaded})
		case !ok:
			res.steps = append(res.steps, Step{Locale: l, Outcome: OutcomeMissing})
		default:
			res.steps = append(res.steps, Step{Locale: l, Outcome: OutcomeFound})
			res.resolvedFrom, res.message = l, msg
			return res
		}
	}
	return res
}

// fsSource reads artifacts from a bundled file system, laid out like the
// edge: a/<sha256>.json.
func fsSource(fsys fs.FS) blobSource {
	return blobSource{get: func(_ context.Context, ref artifactRef) ([]byte, error) {
		return fs.ReadFile(fsys, "a/"+ref.SHA256+".json")
	}}
}

func storeSource(s *store) blobSource {
	return blobSource{get: func(_ context.Context, ref artifactRef) ([]byte, error) {
		return s.artifact(ref.SHA256)
	}}
}

func edgeSource(e *edge) blobSource {
	return blobSource{remote: true, get: e.artifact}
}
