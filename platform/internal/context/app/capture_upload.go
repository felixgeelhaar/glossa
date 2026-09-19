package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/context/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/identity/authz"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/outbox"
	"github.com/felixgeelhaar/glossa/platform/internal/kernel/tenancy"
)

// errNoImages means the service was built without WithImages.
var errNoImages = errors.New("context: capture images need object storage (WithImages)")

// ImageParts yields a capture upload's image parts, in the order they
// arrive after the manifest.
type ImageParts interface {
	// Next returns the next part's name and body; the body is valid until
	// the next call. It returns io.EOF after the last part.
	Next() (name string, body io.Reader, err error)
}

// IngestCaptures is the input of IngestCaptures.
type IngestCaptures struct {
	Project uuid.UUID
	// Manifest is the glossa.captures/v1 manifest's bytes.
	Manifest []byte
	// Images are the upload's image parts, each named by the lowercase
	// hex SHA-256 of its bytes as the manifest references it.
	Images ImageParts
}

// CapturesIngested says what IngestCaptures stored.
type CapturesIngested struct {
	// Build is the capture build (source capture).
	Build domain.Build
	// Captures counts the build's captures.
	Captures int
	// ImagesStored counts the images written to object storage;
	// ImagesDeduplicated those whose pixels were stored already. Both
	// are 0 on a replay: its images aren't read.
	ImagesStored       int
	ImagesDeduplicated int
	// UnknownKeys are the region keys the catalog didn't know, in order
	// (stored with a null message ID, like usages).
	UnknownKeys []string
	// Replayed is true when the same manifest was uploaded before for
	// the application and commit: nothing changed and Build is the first
	// upload's.
	Replayed bool
}

// IngestCaptures stores a capture upload (RFC 0004 §3.2–§3.3): a build
// of source capture with the manifest's captures and their regions,
// region keys resolved to message IDs. Every image part is checked
// against its name and its manifest entry, re-encoded and stored
// content-addressed, once per project. The upload is idempotent by
// application, commit and manifest digest: a repeat returns the first
// build without reading its images. Rate-limited per tenant with the
// usage uploads (ErrRateLimited). Needs catalog.write.
func (s *Service) IngestCaptures(ctx context.Context, in IngestCaptures) (CapturesIngested, error) {
	by, err := actor(ctx, authz.CatalogWrite)
	if err != nil {
		return CapturesIngested{}, err
	}
	if s.objects == nil || s.images == nil {
		return CapturesIngested{}, errNoImages
	}
	if err := s.allowUpload(ctx); err != nil {
		return CapturesIngested{}, err
	}
	up, err := domain.ParseCaptures(in.Manifest)
	if err != nil {
		return CapturesIngested{}, err
	}
	b, err := s.captureBuild(ctx, in.Project, up, by)
	if err != nil {
		return CapturesIngested{}, err
	}
	if first, found, err := s.capturesUploaded(ctx, b); err != nil || found {
		s.recordCaptures(ctx, first, 0, 0)
		return first, err
	}
	rec, err := s.receiveImages(ctx, in.Project, up, in.Images)
	var (
		captures []domain.Capture
		out      CapturesIngested
	)
	if err == nil {
		var unknown []string
		if captures, unknown, err = s.newCaptures(ctx, b, up, rec.images); err == nil {
			out, err = s.insertCaptures(ctx, b, captures, unknown)
		}
	}
	if err != nil {
		s.discardImages(ctx, rec.written)
		return CapturesIngested{}, err
	}
	if !out.Replayed {
		out.ImagesStored, out.ImagesDeduplicated = rec.stored, rec.deduplicated
	}
	s.recordCaptures(ctx, out, regionCount(captures), rec.bytes)
	return out, nil
}

func (s *Service) captureBuild(ctx context.Context, project uuid.UUID, up domain.CaptureUpload, by string) (domain.Build, error) {
	defaultBranch, err := s.catalog.DefaultBranch(ctx, project)
	if err != nil {
		return domain.Build{}, err
	}
	application, err := s.catalog.Application(ctx, project, up.Application)
	if err != nil {
		return domain.Build{}, err
	}
	return domain.NewBuild(project, application, up.Upload, domain.SourceCapture, up.Branch.String() == defaultBranch, by, s.now())
}

func (s *Service) recordCaptures(ctx context.Context, out CapturesIngested, regions int, bytes int64) {
	if out.Build.ID == uuid.Nil {
		return
	}
	s.metrics.BuildIngested(domain.SourceCapture, 0, 0, out.Replayed)
	if !out.Replayed {
		t, _ := tenancy.FromContext(ctx)
		s.metrics.CapturesIngested(t, out.Captures, regions, out.ImagesStored, out.ImagesDeduplicated, bytes)
	}
}

func regionCount(cs []domain.Capture) int {
	n := 0
	for _, c := range cs {
		n += len(c.Regions)
	}
	return n
}

// capturesUploaded finds an earlier ingest of b's manifest.
func (s *Service) capturesUploaded(ctx context.Context, b domain.Build) (CapturesIngested, bool, error) {
	var out CapturesIngested
	err := s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		var err error
		out, err = replayCaptures(ctx, st, b)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return CapturesIngested{}, false, nil
	}
	return out, err == nil, err
}

func replayCaptures(ctx context.Context, st Store, b domain.Build) (CapturesIngested, error) {
	first, err := st.BuildByUpload(ctx, b.ApplicationID, b.Commit, b.Source, b.Digest)
	if err != nil {
		return CapturesIngested{}, err
	}
	n, err := st.CountCaptures(ctx, first.ID)
	if err != nil {
		return CapturesIngested{}, err
	}
	unknown, err := st.UnknownRegionKeys(ctx, first.ID)
	if unknown == nil {
		unknown = []string{}
	}
	return CapturesIngested{Build: first, Captures: n, UnknownKeys: unknown, Replayed: true}, err
}

// received is what an upload's image parts stored.
type received struct {
	// images maps each uploaded digest to its stored image.
	images map[domain.Digest]domain.Image
	// seen are the stored images' digests.
	seen map[domain.Digest]bool
	// written are the keys this upload wrote: removed again if the
	// upload fails.
	written []string
	// stored counts the images written, deduplicated those whose pixels
	// were stored already; bytes is what was written.
	stored, deduplicated int
	bytes                int64
}

// receiveImages reads every image part: each names an image of the
// manifest, once, and is the PNG its entry describes. Each is stored as
// soon as it is re-encoded, so an upload holds one image on disk at a
// time, however many it carries.
func (s *Service) receiveImages(ctx context.Context, project uuid.UUID, up domain.CaptureUpload, parts ImageParts) (received, error) {
	want := map[domain.Digest]domain.Image{}
	for _, img := range up.Parts() {
		want[img.Digest] = img
	}
	rec := received{images: map[domain.Digest]domain.Image{}, seen: map[domain.Digest]bool{}}
	for {
		name, body, err := parts.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return rec, err
		}
		d, err := domain.ParseDigest(name)
		entry, referenced := want[d]
		_, twice := rec.images[d]
		switch {
		case err != nil || !referenced:
			return rec, fmt.Errorf("%w: part %q is no image of the manifest", domain.ErrInvalidCaptures, name)
		case twice:
			return rec, fmt.Errorf("%w: image %s is uploaded twice", domain.ErrInvalidCaptures, d)
		}
		if err := s.receiveImage(ctx, project, entry, body, &rec); err != nil {
			return rec, err
		}
	}
	for _, img := range up.Parts() {
		if _, ok := rec.images[img.Digest]; !ok {
			return rec, fmt.Errorf("%w: no part holds image %s", domain.ErrInvalidCaptures, img.Digest)
		}
	}
	return rec, nil
}

// receiveImage re-encodes one part and stores it content-addressed,
// unless the project (or this upload) stored the same pixels already.
func (s *Service) receiveImage(ctx context.Context, project uuid.UUID, entry domain.Image, body io.Reader, rec *received) error {
	n, err := s.images.Normalize(ctx, body, entry.Digest)
	if err != nil {
		return err
	}
	defer func() { _ = n.Close() }()
	img := n.Image()
	if img.Width != entry.Width || img.Height != entry.Height {
		return fmt.Errorf("%w: image %s is %d×%d pixels; the manifest says %d×%d", domain.ErrInvalidImage, entry.Digest,
			img.Width, img.Height, entry.Width, entry.Height)
	}
	rec.images[entry.Digest] = img
	stored := rec.seen[img.Digest]
	rec.seen[img.Digest] = true
	t, _ := tenancy.FromContext(ctx)
	key := domain.ImageKey(t, project, img.Digest)
	if !stored {
		if stored, err = s.objects.Exists(ctx, key); err != nil {
			return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
		}
	}
	if stored {
		rec.deduplicated++
		return nil
	}
	r, err := n.Open()
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	size, err := s.objects.PutStream(ctx, key, r, "image/png")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	rec.written = append(rec.written, key)
	rec.stored++
	rec.bytes += size
	return nil
}

// discardImages removes the images a failed upload wrote. Best effort:
// what can't be removed stays unreferenced, and a retry of the same
// pixels reuses it.
func (s *Service) discardImages(ctx context.Context, keys []string) {
	for _, key := range keys {
		if err := s.objects.Delete(context.WithoutCancel(ctx), key); err != nil {
			s.logger.WarnContext(ctx, "context: image of a failed upload not removed", slog.String("key", key),
				slog.Any("error", err))
		}
	}
}

// newCaptures builds the captures of b: each on its stored image, with
// its region keys resolved to message IDs. It returns the keys the
// catalog didn't know, in order.
func (s *Service) newCaptures(ctx context.Context, b domain.Build, up domain.CaptureUpload, images map[domain.Digest]domain.Image) ([]domain.Capture, []string, error) {
	ids, err := s.catalog.MessageIDs(ctx, b.ProjectID, up.Keys())
	if err != nil {
		return nil, nil, err
	}
	out := make([]domain.Capture, len(up.Captures))
	unknown := map[string]bool{}
	for i, in := range up.Captures {
		in.Image = images[in.Image.Digest]
		in.Regions = slices.Clone(in.Regions)
		c, err := domain.NewCapture(b.ProjectID, b.ID, in, b.CreatedBy, b.CreatedAt)
		if err != nil {
			return nil, nil, err
		}
		domain.ResolveRegions(c.Regions, ids)
		for _, r := range c.Regions {
			if r.MessageID == nil {
				unknown[r.Key] = true
			}
		}
		out[i] = c
	}
	keys := make([]string, 0, len(unknown))
	for k := range unknown {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return out, keys, nil
}

// insertCaptures stores b with its captures in one unit of work and
// publishes context.build.ingested and a context.capture.ingested per
// capture. A concurrent upload of the same manifest that won makes this
// one its replay.
func (s *Service) insertCaptures(ctx context.Context, b domain.Build, captures []domain.Capture, unknown []string) (CapturesIngested, error) {
	var out CapturesIngested
	err := s.tx.InTenant(ctx, func(ctx context.Context, st Store) error {
		inserted, err := st.InsertBuild(ctx, b)
		if err != nil {
			return err
		}
		if !inserted {
			out, err = replayCaptures(ctx, st, b)
			return err
		}
		for _, c := range captures {
			ok, err := st.InsertCapture(ctx, c)
			if err != nil {
				return err
			}
			if !ok { // the manifest holds each shot once; a new build none
				return ErrCaptureConflict
			}
			if err := st.Publish(ctx, outbox.Event{
				Type: domain.EventCaptureIngested, AggregateType: domain.AggregateCapture, AggregateID: c.ID.String(),
				Payload: domain.CaptureIngestedOf(c, b), OccurredAt: c.CreatedAt,
			}); err != nil {
				return err
			}
		}
		out = CapturesIngested{Build: b, Captures: len(captures), UnknownKeys: unknown}
		return st.Publish(ctx, outbox.Event{
			Type: domain.EventBuildIngested, AggregateType: domain.AggregateBuild, AggregateID: b.ID.String(),
			Payload: domain.BuildIngestedOf(b, 0), OccurredAt: b.CreatedAt,
		})
	})
	return out, err
}
