package app

import (
	"context"
	"errors"

	"github.com/felixgeelhaar/glossa/platform/internal/catalog/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/mfcontent"
)

// MaxBatch bounds a bulk request.
const MaxBatch = 500

// UpsertItem is one message of a bulk upsert (CLI push and extract).
// Detail members left nil keep the stored value (or the default for a
// new message).
type UpsertItem struct {
	Key         string
	Namespace   *string
	Description *string
	MaxLength   *int
	Text        string
	Syntax      string
	// BaseRevision, when set, is the source revision the client last
	// saw: if the stored message has moved on, the item fails with
	// source_revision_conflict instead of overwriting someone's edit.
	BaseRevision *int
}

// UpsertStatus is what a bulk upsert did with one item.
type UpsertStatus string

// Upsert statuses.
const (
	UpsertCreated   UpsertStatus = "created"
	UpsertRevised   UpsertStatus = "revised"   // new source revision (details may have changed too)
	UpsertUpdated   UpsertStatus = "updated"   // details or reactivation, same source
	UpsertUnchanged UpsertStatus = "unchanged" // nothing to do: pushing twice is a no-op
	UpsertFailed    UpsertStatus = "failed"
)

// ItemError is a per-item failure with a stable problem code.
type ItemError struct {
	Code   string
	Detail string
}

// UpsertResult reports one item, in request order.
type UpsertResult struct {
	Key     string
	Status  UpsertStatus
	Message *domain.Message
	Error   *ItemError
}

// UpsertMessages creates or revises messages by key in one transaction.
// It is idempotent: pushing the same catalog twice changes nothing the
// second time. Items fail individually (invalid key or source, duplicate
// key in the batch, stale base revision) without failing the batch.
func (s *Service) UpsertMessages(ctx context.Context, project domain.ProjectID, items []UpsertItem) ([]UpsertResult, error) {
	by, err := author(ctx, authz.CatalogWrite)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 || len(items) > MaxBatch {
		return nil, ErrTooManyItems
	}
	var results []UpsertResult
	err = s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		p, err := st.Project(ctx, project)
		if err != nil {
			return err
		}
		keys, prepared := s.prepareUpserts(p, items)
		existing, err := st.LockMessagesByKeys(ctx, project, keys)
		if err != nil {
			return err
		}
		results = make([]UpsertResult, len(items))
		var changed []domain.Message
		for i, it := range prepared {
			if it.err != nil {
				results[i] = UpsertResult{Key: items[i].Key, Status: UpsertFailed, Error: it.err}
				continue
			}
			res, err := s.upsertOne(ctx, st, p, it, existing, by)
			if err != nil {
				return err
			}
			if res.Message != nil {
				existing[res.Message.Key] = *res.Message
			}
			if res.Status == UpsertCreated || res.Status == UpsertRevised || res.Status == UpsertUpdated {
				changed = append(changed, *res.Message)
			}
			results[i] = res
		}
		if s.projection == nil || len(changed) == 0 {
			return nil
		}
		return s.projection.MessagesChanged(ctx, changed)
	})
	return results, err
}

type preparedItem struct {
	UpsertItem
	key     domain.MessageKey
	content mfcontent.Content
	err     *ItemError
}

// prepareUpserts validates items without touching storage.
func (s *Service) prepareUpserts(p domain.Project, items []UpsertItem) ([]domain.MessageKey, []preparedItem) {
	seen := map[domain.MessageKey]bool{}
	var keys []domain.MessageKey
	out := make([]preparedItem, len(items))
	for i, it := range items {
		out[i] = preparedItem{UpsertItem: it}
		key, err := domain.ParseMessageKey(it.Key)
		if err != nil {
			out[i].err = &ItemError{Code: "invalid_message_key", Detail: err.Error()}
			continue
		}
		if seen[key] {
			out[i].err = &ItemError{Code: "duplicate_key", Detail: "the key appears earlier in this batch"}
			continue
		}
		seen[key] = true
		c, err := parseSource(p, it.Syntax, it.Text)
		if err != nil {
			out[i].err = sourceItemError(err)
			continue
		}
		out[i].key, out[i].content = key, c
		keys = append(keys, key)
	}
	return keys, out
}

func sourceItemError(err error) *ItemError {
	var invalid *mfcontent.InvalidError
	switch {
	case errors.As(err, &invalid):
		return &ItemError{Code: "invalid_message", Detail: string(invalid.Code) + ": " + invalid.Message}
	case errors.Is(err, mfcontent.ErrInvalidSyntax):
		return &ItemError{Code: "invalid_syntax", Detail: err.Error()}
	case errors.Is(err, mfcontent.ErrTooLong):
		return &ItemError{Code: "message_too_long", Detail: err.Error()}
	}
	return &ItemError{Code: "invalid_message", Detail: err.Error()}
}

func (it preparedItem) details(base domain.Details) (domain.Details, *ItemError) {
	d := base
	if it.Namespace != nil {
		ns, err := domain.ParseNamespace(*it.Namespace)
		if err != nil {
			return d, &ItemError{Code: "invalid_namespace", Detail: err.Error()}
		}
		d.Namespace = ns
	}
	if it.Description != nil {
		d.Description = *it.Description
	}
	if it.MaxLength != nil {
		d.MaxLength = it.MaxLength
	}
	if err := d.Validate(); err != nil {
		return d, &ItemError{Code: "invalid_details", Detail: err.Error()}
	}
	return d, nil
}

func (s *Service) upsertOne(ctx context.Context, st Store, p domain.Project, it preparedItem,
	existing map[domain.MessageKey]domain.Message, by domain.Author,
) (UpsertResult, error) {
	res := UpsertResult{Key: it.Key}
	m, found := existing[it.key]
	if !found {
		if it.BaseRevision != nil && *it.BaseRevision != 0 {
			res.Status, res.Error = UpsertFailed, &ItemError{Code: "source_revision_conflict", Detail: "the message doesn't exist"}
			return res, nil
		}
		return s.createFromUpsert(ctx, st, p, it, by)
	}
	if it.BaseRevision != nil && *it.BaseRevision != m.Revision {
		res.Status, res.Error = UpsertFailed, &ItemError{Code: "source_revision_conflict",
			Detail: "the message is at a newer source revision than base_revision"}
		return res, nil
	}
	d, ierr := it.details(m.Details)
	if ierr != nil {
		res.Status, res.Error = UpsertFailed, ierr
		return res, nil
	}
	expected, oldRev, now := m.Version, m.Revision, s.now()
	reactivated := m.Reactivate(now)
	detailsChanged, _ := m.ChangeDetails(d, now)
	rev, revised, err := m.ReviseSource(it.content, by, now)
	if err != nil {
		return res, err
	}
	res.Message, res.Status = &m, UpsertUnchanged
	if !reactivated && !detailsChanged && !revised {
		return res, nil
	}
	if err := st.UpdateMessage(ctx, m, expected); err != nil {
		return res, err
	}
	if reactivated {
		if err := st.Publish(ctx, messageEvent(domain.EventMessageReactivated, m, by)); err != nil {
			return res, err
		}
	}
	if detailsChanged {
		if err := st.Publish(ctx, messageEvent(domain.EventMessageUpdated, m, by)); err != nil {
			return res, err
		}
	}
	res.Status = UpsertUpdated
	if revised {
		res.Status = UpsertRevised
		if err := st.AppendSourceRevision(ctx, rev); err != nil {
			return res, err
		}
		if err := st.Publish(ctx, sourceRevisedEvent(m, oldRev, by)); err != nil {
			return res, err
		}
	}
	return res, nil
}

func (s *Service) createFromUpsert(ctx context.Context, st Store, p domain.Project, it preparedItem, by domain.Author) (UpsertResult, error) {
	res := UpsertResult{Key: it.Key}
	d, ierr := it.details(domain.Details{Namespace: domain.DefaultNamespace})
	if ierr != nil {
		res.Status, res.Error = UpsertFailed, ierr
		return res, nil
	}
	m, first, err := domain.NewMessage(p.ID, it.key, d, it.content, by, s.now())
	if err != nil {
		return res, err
	}
	if _, err := st.InsertMessage(ctx, m, first, by); err != nil {
		return res, err
	}
	if err := st.Publish(ctx, messageEvent(domain.EventMessageCreated, m, by)); err != nil {
		return res, err
	}
	res.Status, res.Message = UpsertCreated, &m
	return res, nil
}
