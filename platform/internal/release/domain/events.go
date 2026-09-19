package domain

import "time"

// Release's domain events (platform/README.md, "Domain events"). The
// Release aggregate shares the context's name, so its events are
// release.<verb>, as runtimes/SPEC.md §3 names release.published.
const (
	AggregateRelease     = "release"
	AggregateEnvironment = "environment"
	AggregateDeliveryKey = "delivery_key"

	EventPublished  = "release.published"
	EventPromoted   = "release.promoted"
	EventRolledBack = "release.rolled_back"

	EventEnvironmentCreated       = "release.environment.created"
	EventEnvironmentPolicyChanged = "release.environment.policy_changed"
	// EventEnvironmentDestroyed: a branch environment was destroyed; its
	// manifest is removed, so the edge answers 404 (RFC 0004 §4.2).
	EventEnvironmentDestroyed = "release.environment.destroyed"
	// EventPublishRequested: a branch environment is to be published
	// again once the request is due (debounced).
	EventPublishRequested = "release.environment.publish_requested"

	EventDeliveryKeyCreated = "release.delivery_key.created"
	// EventDeliveryKeyScopeChanged: what the key reads changed
	// (RFC 0004 §4.3); its index object is written again.
	EventDeliveryKeyScopeChanged = "release.delivery_key.scope_changed"
	EventDeliveryKeyRevoked      = "release.delivery_key.revoked"
)

// Published is the payload of release.published.
type Published struct {
	ReleaseID      string `json:"release_id"`
	ProjectID      string `json:"project_id"`
	Version        int    `json:"version"`
	Environment    string `json:"environment"`
	ParentID       string `json:"parent_id,omitempty"`
	ManifestDigest string `json:"manifest_digest"`
	Messages       int    `json:"messages"`
	By             string `json:"by"`
}

// PointerMoved is the payload of release.promoted and release.rolled_back:
// environment now serves release_id instead of previous_release_id.
type PointerMoved struct {
	ReleaseID         string `json:"release_id"`
	ProjectID         string `json:"project_id"`
	Version           int    `json:"version"`
	Environment       string `json:"environment"`
	PreviousReleaseID string `json:"previous_release_id,omitempty"`
	By                string `json:"by"`
}

// EnvironmentChanged is the payload of the environment events.
type EnvironmentChanged struct {
	ProjectID   string `json:"project_id"`
	Environment string `json:"environment"`
	// Kind is standard or branch; Branch names a branch environment's
	// branch.
	Kind            string   `json:"kind,omitempty"`
	Branch          string   `json:"branch,omitempty"`
	States          []string `json:"states"`
	IncludeOutdated bool     `json:"include_outdated"`
	Version         int      `json:"version"`
	By              string   `json:"by"`
}

// DeliveryKeyChanged is the payload of the delivery key events. It never
// carries the key itself: public or not, keys stay out of event logs.
type DeliveryKeyChanged struct {
	KeyID        string   `json:"key_id"`
	ProjectID    string   `json:"project_id"`
	Name         string   `json:"name"`
	Environments []string `json:"environments"`
	Branches     bool     `json:"branches"`
	By           string   `json:"by"`
}

// PublishRequested is the payload of release.environment.publish_requested.
type PublishRequested struct {
	ProjectID   string    `json:"project_id"`
	Environment string    `json:"environment"`
	Branch      string    `json:"branch"`
	RequestID   string    `json:"request_id"`
	NotBefore   time.Time `json:"not_before"`
	By          string    `json:"by"`
}
