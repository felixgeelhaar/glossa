package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/bcp47"
)

// Capture limits (RFC 0004 §3.3, §10).
const (
	// MaxCapturesPerBuild bounds the captures of one build.
	MaxCapturesPerBuild = 500
	// MaxRegionsPerCapture bounds the regions of one capture.
	MaxRegionsPerCapture = 10_000
	// MaxImagePixels bounds a capture's image (40 megapixels).
	MaxImagePixels = 40_000_000
	// MaxViewportSide bounds each side of a viewport, in CSS pixels.
	MaxViewportSide = 10_000
)

// Viewport is the browser viewport a capture was taken at, in CSS
// pixels (default 1280×800 and 390×844).
type Viewport struct {
	Width  int
	Height int
}

func (v Viewport) valid() bool {
	return v.Width >= 1 && v.Width <= MaxViewportSide && v.Height >= 1 && v.Height <= MaxViewportSide
}

// Image is a capture's page image: content-addressed by the digest of
// its re-encoded PNG (RFC 0004 §3.3), so builds share identical pixels.
type Image struct {
	Digest Digest
	Width  int
	Height int
}

func (i Image) validate() error {
	if _, err := ParseDigest(string(i.Digest)); err != nil {
		return err
	}
	if i.Width < 1 || i.Height < 1 || int64(i.Width)*int64(i.Height) > MaxImagePixels {
		return fmt.Errorf("%w: the image must be 1 to %d pixels", ErrInvalidCapture, MaxImagePixels)
	}
	return nil
}

// RegionKind says how a region's box was found (RFC 0004 §3.1).
type RegionKind string

// Region kinds.
const (
	// RegionElement is a component-rendered message: the host element's
	// box.
	RegionElement RegionKind = "element"
	// RegionText is a marked range of text: its client rects' union.
	RegionText RegionKind = "text"
	// RegionAttribute is a message in an attribute (placeholder, title,
	// aria-label): the element's box.
	RegionAttribute RegionKind = "attribute"
)

// Box is a rectangle on the page image, in CSS pixels from the page's
// top left. An off-screen box may lie at negative coordinates.
type Box struct {
	X      int
	Y      int
	Width  int
	Height int
}

// Region is where one rendered message appears on a capture. The
// message is referenced by key and resolved to its ID at ingest, as a
// usage is. Visible is false for a marked message that rendered with
// zero size or off-screen, so the gap shows instead of being hidden.
type Region struct {
	Key       string
	MessageID *uuid.UUID
	Kind      RegionKind
	Box       Box
	Visible   bool
}

func (r Region) validate() error {
	switch {
	case !textWithin(r.Key, 1, MaxKeyLen):
		return fmt.Errorf("key must be 1–%d characters", MaxKeyLen)
	case r.Kind != RegionElement && r.Kind != RegionText && r.Kind != RegionAttribute:
		return fmt.Errorf("kind must be element, text or attribute, not %q", r.Kind)
	case r.Box.Width < 0 || r.Box.Height < 0:
		return fmt.Errorf("a box has no negative size")
	}
	return nil
}

// CaptureInput is what a capture upload says.
type CaptureInput struct {
	// Route is the route pattern the page was captured at.
	Route    string
	Viewport Viewport
	Locale   bcp47.Tag
	Image    Image
	Regions  []Region
}

// Capture is one (route, viewport, locale) screenshot of a build: the
// page image plus the regions of the messages on it (RFC 0004 §3.2).
// It is immutable, and it follows its build's retention.
type Capture struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	BuildID   uuid.UUID
	Route     string
	Viewport  Viewport
	Locale    bcp47.Tag
	Image     Image
	Regions   []Region
	CreatedBy string
	CreatedAt time.Time
}

// NewCapture validates a capture of build.
func NewCapture(project, build uuid.UUID, in CaptureInput, by string, now time.Time) (Capture, error) {
	switch {
	case !textWithin(in.Route, 1, MaxRouteLen):
		return Capture{}, fmt.Errorf("%w: route must be 1–%d characters", ErrInvalidCapture, MaxRouteLen)
	case !in.Viewport.valid():
		return Capture{}, fmt.Errorf("%w: viewport sides must be 1–%d pixels", ErrInvalidCapture, MaxViewportSide)
	case in.Locale.IsZero():
		return Capture{}, fmt.Errorf("%w: locale is required", ErrInvalidCapture)
	case len(in.Regions) > MaxRegionsPerCapture:
		return Capture{}, ErrTooManyRegions
	}
	if err := in.Image.validate(); err != nil {
		return Capture{}, err
	}
	for i, r := range in.Regions {
		if err := r.validate(); err != nil {
			return Capture{}, fmt.Errorf("%w: regions[%d]: %v", ErrInvalidRegion, i, err)
		}
	}
	return Capture{
		ID: uuid.Must(uuid.NewV7()), ProjectID: project, BuildID: build, Route: in.Route, Viewport: in.Viewport,
		Locale: in.Locale, Image: in.Image, Regions: in.Regions, CreatedBy: by, CreatedAt: now,
	}, nil
}

// SameShot reports whether c and o capture the same route, viewport and
// locale in the same pixels: a repeated upload of one screenshot.
func (c Capture) SameShot(o Capture) bool {
	return c.Route == o.Route && c.Viewport == o.Viewport && c.Locale == o.Locale && c.Image == o.Image
}

// CheckCaptureLimit refuses another capture for a build that already
// holds existing ones.
func CheckCaptureLimit(existing int) error {
	if existing >= MaxCapturesPerBuild {
		return ErrTooManyCaptures
	}
	return nil
}
