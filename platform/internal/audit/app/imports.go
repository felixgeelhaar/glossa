package app

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// The import of v0.3's history (RFC 0006 §7.2), declared ahead of both
// of its ends so the HTTP route (the wave-4 API slice) and the use case
// (the wave-4 audit-import slice) are built in parallel against one
// shape. `glossa import --from v0 --v0-db` plans these from v0.3's
// audit_log; this is what it sends.

// V0HistoryEntry is one row of v0.3's audit_log. It carries no text: a
// translation's before and after are recorded as SHA-256 digests of
// their UTF-8 bytes, because an audit entry never holds translation
// text (§6.1), and an export that held v0.3's history verbatim would
// carry every string the product ever shipped.
type V0HistoryEntry struct {
	// V0ID is v0.3's audit_log id: the idempotency key, so importing a
	// tenant twice, or once per project, records each row once.
	V0ID string
	// Action is the v0.3 action as the plan names it
	// ("v0.translation.changed").
	Action string
	// Actor is v0:<uuid>, v0:ai:<label>, v0:system:<label> or
	// v0:unknown, as planned. Never a platform actor: v0.3's people are
	// not this platform's members until they accept an invitation.
	Actor      string
	OccurredAt time.Time
	// Project is the platform project the history was imported into.
	Project uuid.UUID
	// Key and Locale locate the translation, when the row still
	// resolves to one; Unresolved says why when it does not.
	Key        string
	Locale     string
	Unresolved string
	// BeforeSHA256 and AfterSHA256 are hex digests, empty for none.
	BeforeSHA256 string
	AfterSHA256  string
}

// V0HistoryImport is one call's worth of rows.
type V0HistoryImport struct {
	// Restore is the dump's name and digest the rows were read from, as
	// the restore marker records them.
	Restore       string
	RestoreSHA256 string
	Entries       []V0HistoryEntry
}

// V0HistoryReport says what an import recorded.
type V0HistoryReport struct {
	Recorded int
	// Existing were recorded by an earlier import (same V0ID).
	Existing int
}

// V0HistoryImporter records v0.3's history as audit entries. The use
// case requires audit.export's sibling the importer holds — see the
// implementing slice — and is idempotent on V0ID.
type V0HistoryImporter interface {
	ImportV0History(ctx context.Context, in V0HistoryImport) (V0HistoryReport, error)
}
