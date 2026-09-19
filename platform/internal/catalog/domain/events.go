package domain

// Catalog's domain events (platform/README.md, "Domain events"). Their
// names and payloads are a published contract: a breaking change is a
// new ".v2" type, never an edit.
const (
	AggregateProject     = "project"
	AggregateApplication = "application"
	AggregateMessage     = "message"

	EventProjectCreated = "catalog.project.created"
	EventProjectUpdated = "catalog.project.updated"
	EventProjectDeleted = "catalog.project.deleted"

	EventApplicationCreated = "catalog.application.created"
	EventApplicationUpdated = "catalog.application.updated"
	EventApplicationDeleted = "catalog.application.deleted"

	EventMessageCreated       = "catalog.message.created"
	EventMessageSourceRevised = "catalog.message.source_revised"
	EventMessageUpdated       = "catalog.message.updated"
	EventMessageRenamed       = "catalog.message.renamed"
	EventMessageObsoleted     = "catalog.message.obsoleted"
	EventMessageReactivated   = "catalog.message.reactivated"
)

// ProjectEvent is the payload of every project event.
type ProjectEvent struct {
	ProjectID    string `json:"project_id"`
	Slug         string `json:"slug"`
	SourceLocale string `json:"source_locale"`
	Version      int    `json:"version"`
	By           string `json:"by"`
}

// ProjectEventOf builds the payload for p.
func ProjectEventOf(p Project, by Author) ProjectEvent {
	return ProjectEvent{
		ProjectID: p.ID.String(), Slug: string(p.Slug), SourceLocale: p.SourceLocale.String(),
		Version: p.Version, By: string(by),
	}
}

// ApplicationEvent is the payload of every application event.
type ApplicationEvent struct {
	ApplicationID string `json:"application_id"`
	ProjectID     string `json:"project_id"`
	Slug          string `json:"slug"`
	Platform      string `json:"platform"`
	By            string `json:"by"`
}

// ApplicationEventOf builds the payload for a.
func ApplicationEventOf(a Application, by Author) ApplicationEvent {
	return ApplicationEvent{
		ApplicationID: a.ID.String(), ProjectID: a.ProjectID.String(), Slug: string(a.Slug),
		Platform: string(a.Platform), By: string(by),
	}
}

// MessageSnapshot is the state of a message after the change an event
// reports. Every message event carries one, so a projection can apply
// events in any order and more than once: it keeps the snapshot with the
// highest Version (the outbox guarantees neither order nor uniqueness).
type MessageSnapshot struct {
	MessageID      string `json:"message_id"`
	ProjectID      string `json:"project_id"`
	Key            string `json:"key"`
	Namespace      string `json:"namespace"`
	State          string `json:"state"`
	SourceRevision int    `json:"source_revision"`
	Version        int    `json:"version"`
}

// SnapshotOf takes m's snapshot.
func SnapshotOf(m Message) MessageSnapshot {
	return MessageSnapshot{
		MessageID: m.ID.String(), ProjectID: m.ProjectID.String(), Key: string(m.Key),
		Namespace: string(m.Namespace), State: string(m.State), SourceRevision: m.Revision, Version: m.Version,
	}
}

// MessageEvent is the payload of catalog.message.{created, updated,
// obsoleted, reactivated}.
type MessageEvent struct {
	Message MessageSnapshot `json:"message"`
	By      string          `json:"by"`
}

// SourceRevised is the payload of catalog.message.source_revised.
type SourceRevised struct {
	Message     MessageSnapshot `json:"message"`
	OldRevision int             `json:"old_revision"`
	NewRevision int             `json:"new_revision"`
	By          string          `json:"by"`
}

// MessageRenamed is the payload of catalog.message.renamed.
type MessageRenamed struct {
	Message MessageSnapshot `json:"message"`
	OldKey  string          `json:"old_key"`
	NewKey  string          `json:"new_key"`
	By      string          `json:"by"`
}
