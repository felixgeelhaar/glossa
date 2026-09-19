package glossa

// Explanations (runtimes/SPEC.md §6): why a message rendered the way it
// did, without side effects.

// Source is where rendered content came from.
type Source string

// Sources, in the SPEC §3 reliability order.
const (
	// SourceMemory: the release was already active when the latest load
	// cycle began.
	SourceMemory Source = "memory"
	// SourcePersisted: restored from the cache directory at startup.
	SourcePersisted Source = "persisted"
	// SourceNetwork: activated from the edge by the latest load cycle.
	SourceNetwork Source = "network"
	// SourceBundled: restored from Config.Bundled at startup.
	SourceBundled Source = "bundled"
	// SourceInline: no loaded locale had the message; the inline default
	// or the message ID was rendered.
	SourceInline Source = "inline"
)

// Outcome is what one fallback step found.
type Outcome string

// Step outcomes.
const (
	OutcomeFound   Outcome = "found"
	OutcomeMissing Outcome = "missing"
	// OutcomeNotLoaded: the locale is in the chain but its artifacts
	// weren't loaded, because Config.Locales left it out.
	OutcomeNotLoaded Outcome = "not-loaded"
)

// Step is one locale of the fallback chain and what resolution found there.
type Step struct {
	Locale  string  `json:"locale"`
	Outcome Outcome `json:"outcome"`
}

// Explanation describes how a message resolves (SPEC §6).
type Explanation struct {
	ID string `json:"id"`
	// Requested are the canonicalized requested locales, in priority order.
	Requested []string `json:"requested"`
	// Locale is the active locale negotiated from Requested.
	Locale string `json:"locale"`
	// Chain is the active locale's fallback chain.
	Chain []string `json:"chain"`
	// ResolvedFrom is the locale the message was found in, or nil when the
	// inline default (or the message ID) is used.
	ResolvedFrom *string `json:"resolvedFrom"`
	// Release is the active release, or nil when nothing is loaded.
	Release *ReleaseRef `json:"release"`
	Source  Source      `json:"source"`
	Steps   []Step      `json:"steps"`
}

func (res resolution) explain(snap *snapshot) Explanation {
	e := Explanation{
		ID:        res.id,
		Requested: nonNil(res.requested),
		Locale:    res.locale,
		Chain:     nonNil(res.chain),
		Source:    SourceInline,
		Steps:     nonNil(res.steps),
	}
	if snap.rel != nil {
		ref := snap.rel.manifest.Release
		e.Release = &ReleaseRef{ID: ref.ID, Version: ref.Version}
	}
	if res.resolvedFrom != "" {
		from := res.resolvedFrom
		e.ResolvedFrom, e.Source = &from, snap.source
	}
	return e
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
