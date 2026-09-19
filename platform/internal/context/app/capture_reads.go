package app

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/context/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/objectstore"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

// CaptureImage is a capture's stored image.
type CaptureImage struct {
	domain.Image
	// Open reads the re-encoded PNG: ErrCaptureNotFound when retention
	// deleted it meanwhile, ErrStorageUnavailable when storage fails.
	Open func() (io.ReadCloser, error)
}

// CaptureImage returns a capture's image, read through the API: the
// object store never hands out a URL (RFC 0004 §3.3). The image is
// content-addressed, so its digest is a strong validator. Needs
// catalog.read.
func (s *Service) CaptureImage(ctx context.Context, project, capture uuid.UUID) (CaptureImage, error) {
	if _, err := s.readView(ctx, project, ""); err != nil {
		return CaptureImage{}, err
	}
	if s.objects == nil {
		return CaptureImage{}, errNoImages
	}
	var c domain.Capture
	err := s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		var err error
		c, err = st.Capture(ctx, capture)
		return err
	})
	if errors.Is(err, ErrNotFound) || (err == nil && c.ProjectID != project) {
		return CaptureImage{}, ErrCaptureNotFound
	}
	if err != nil {
		return CaptureImage{}, err
	}
	t, _ := tenancy.FromContext(ctx)
	key := domain.ImageKey(t, project, c.Image.Digest)
	return CaptureImage{Image: c.Image, Open: func() (io.ReadCloser, error) {
		r, err := s.objects.Open(ctx, key)
		switch {
		case errors.Is(err, objectstore.ErrNotFound):
			return nil, ErrCaptureNotFound
		case err != nil:
			return nil, fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
		}
		return r, nil
	}}, nil
}

// MessageCapturesPage is a message's current captures, up to a limit.
type MessageCapturesPage struct {
	MessageID uuid.UUID
	// Captures show the message; each holds only its regions.
	Captures []CaptureView
	// Truncated says more captures exist than the limit.
	Truncated bool
}

// CapturesOfKey returns the current captures that show the message a
// key names now (ErrMessageNotFound), with its regions on each: the
// default branch first, then by application, route, locale and the
// widest viewport. q.Branch selects a branch view, as for usages.
// Needs catalog.read.
func (s *Service) CapturesOfKey(ctx context.Context, project uuid.UUID, key string, q UsageQuery) (MessageCapturesPage, error) {
	id, err := s.messageOf(ctx, project, key)
	if err != nil {
		return MessageCapturesPage{}, err
	}
	view, err := s.readView(ctx, project, q.Branch)
	if err != nil {
		return MessageCapturesPage{}, err
	}
	limit := q.limit()
	out := MessageCapturesPage{MessageID: id, Captures: []CaptureView{}}
	err = s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		current, err := currentBuilds(ctx, st, project, view)
		if err != nil || len(current) == 0 {
			return err
		}
		got, err := st.MessageCaptures(ctx, id, current, limit+1)
		if err != nil {
			return err
		}
		if len(got) > limit {
			got, out.Truncated = got[:limit], true
		}
		out.Captures = append(out.Captures, got...)
		return nil
	})
	return out, err
}

// measureCaptureCoverage records the share of a project's active
// messages with a visible region on a current default-branch capture.
func (s *Service) measureCaptureCoverage(ctx context.Context, project uuid.UUID) error {
	if err := authz.Require(ctx, authz.CatalogRead); err != nil {
		return err
	}
	active, err := s.catalog.ActiveMessages(ctx, project)
	if err != nil {
		return err
	}
	captured := map[uuid.UUID]bool{}
	err = s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		current, err := currentBuilds(ctx, st, project, "")
		if err != nil || len(current) == 0 {
			return err
		}
		ids, err := st.CapturedMessages(ctx, current)
		for _, id := range ids {
			captured[id] = true
		}
		return err
	})
	if err != nil {
		return err
	}
	n := 0
	for _, m := range active {
		if captured[m.ID] {
			n++
		}
	}
	t, _ := tenancy.FromContext(ctx)
	s.metrics.CaptureCoverage(t, project, len(active), n)
	return nil
}
