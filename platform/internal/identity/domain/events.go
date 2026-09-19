package domain

// Domain events Identity publishes through the outbox. Names follow the
// platform convention "<context>.<aggregate>.<past-tense verb>" (see
// platform/README.md): the aggregate type is the middle segment, the
// payload is JSON with snake_case keys, and a breaking payload change
// ships as a new name with a ".v2" suffix, never in place.
const (
	EventTenantCreated      = "identity.tenant.created"
	EventMemberAdded        = "identity.member.added"
	EventMemberActivated    = "identity.member.activated"
	EventMemberAccessChange = "identity.member.access_changed"
	EventMemberRemoved      = "identity.member.removed"
	EventTokenCreated       = "identity.token.created"
	EventTokenRevoked       = "identity.token.revoked"
)

// Aggregate types, the middle segment of the event names.
const (
	AggregateTenant = "tenant"
	AggregateMember = "member"
	AggregateToken  = "token"
)

// TenantCreated is published when an individual or organization tenant
// comes into existence.
type TenantCreated struct {
	TenantID  string `json:"tenant_id"`
	Kind      string `json:"kind"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	CreatedBy string `json:"created_by"`
}

// MemberAdded is published for every new membership, invited or active.
type MemberAdded struct {
	MemberID string   `json:"member_id"`
	PersonID string   `json:"person_id,omitempty"`
	Email    string   `json:"email"`
	Status   string   `json:"status"`
	Roles    []string `json:"roles"`
	Locales  []string `json:"locales"`
	AddedBy  string   `json:"added_by"`
}

// MemberActivated is published when an invitation is accepted.
type MemberActivated struct {
	MemberID string `json:"member_id"`
	PersonID string `json:"person_id"`
}

// MemberAccessChanged is published when roles or locales change.
type MemberAccessChanged struct {
	MemberID  string   `json:"member_id"`
	Roles     []string `json:"roles"`
	Locales   []string `json:"locales"`
	ChangedBy string   `json:"changed_by"`
}

// MemberRemoved is published when a membership ends.
type MemberRemoved struct {
	MemberID  string `json:"member_id"`
	PersonID  string `json:"person_id,omitempty"`
	RemovedBy string `json:"removed_by"`
}

// TokenCreated is published when an API token is issued. It never
// carries the secret or its hash.
type TokenCreated struct {
	TokenID   string   `json:"token_id"`
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	CreatedBy string   `json:"created_by"`
}

// TokenRevoked is published when an API token is revoked.
type TokenRevoked struct {
	TokenID   string `json:"token_id"`
	RevokedBy string `json:"revoked_by"`
}
