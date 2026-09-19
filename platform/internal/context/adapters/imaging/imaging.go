// Package imaging implements Context's ImageNormalizer (RFC 0004 §3.3):
// an uploaded capture image is spooled to disk while its SHA-256 is
// checked, its PNG header is read to refuse anything over 40 megapixels
// before a pixel is decoded, and its pixels are decoded with image/png
// and encoded again. Go's PNG encoder writes only the chunks the pixels
// need (IHDR, PLTE, tRNS, IDAT, IEND), so text, EXIF, ICC and every
// other chunk the upload carried is gone, and the same pixels always
// encode to the same bytes: the stored image's digest dedupes across
// builds.
//
// A decoded 40-megapixel image takes up to 320 MB (16-bit RGBA), so
// decoding is a bulkhead: at most concurrency images are decoded at
// once per process; the rest wait. Nothing is held in memory beyond
// the image being re-encoded.
package imaging

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"image/png"
	"io"
	"os"

	"github.com/felixgeelhaar/glossa/platform/internal/context/app"
	"github.com/felixgeelhaar/glossa/platform/internal/context/domain"
)

// DefaultConcurrency is how many images a process decodes at once.
const DefaultConcurrency = 2

// Normalizer implements app.ImageNormalizer.
type Normalizer struct {
	dir   string
	slots chan struct{}
}

var _ app.ImageNormalizer = (*Normalizer)(nil)

// New returns a Normalizer that spools to dir (the system's temporary
// directory when empty) and decodes at most concurrency images at once
// (DefaultConcurrency when below 1).
func New(dir string, concurrency int) *Normalizer {
	if concurrency < 1 {
		concurrency = DefaultConcurrency
	}
	return &Normalizer{dir: dir, slots: make(chan struct{}, concurrency)}
}

// Normalize implements app.ImageNormalizer.
func (n *Normalizer) Normalize(ctx context.Context, part io.Reader, want domain.Digest) (app.NormalizedImage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	upload, err := n.spool(part, want)
	if err != nil {
		return nil, err
	}
	defer remove(upload)
	if err := checkHeader(upload); err != nil {
		return nil, err
	}
	return n.reencode(ctx, upload)
}

// spool copies the part to a temporary file, at most MaxImageBytes of
// it, and checks its digest.
func (n *Normalizer) spool(part io.Reader, want domain.Digest) (*os.File, error) {
	f, err := os.CreateTemp(n.dir, "glossa-capture-upload-*")
	if err != nil {
		return nil, fmt.Errorf("imaging: spool: %w", err)
	}
	h := sha256.New()
	size, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(part, domain.MaxImageBytes+1))
	switch {
	case err != nil:
		remove(f)
		return nil, err
	case size > domain.MaxImageBytes:
		remove(f)
		return nil, fmt.Errorf("%w: image %s is larger than %d bytes", domain.ErrImageTooLarge, want, domain.MaxImageBytes)
	case sum(h) != want:
		remove(f)
		return nil, fmt.Errorf("%w: the part named %s has the SHA-256 %s", domain.ErrInvalidImage, want, sum(h))
	}
	return f, nil
}

// checkHeader reads the PNG header: a PNG of at most MaxImagePixels.
func checkHeader(f *os.File) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	cfg, err := png.DecodeConfig(bufio.NewReader(f))
	if err != nil {
		return fmt.Errorf("%w: not a PNG: %v", domain.ErrInvalidImage, err)
	}
	if int64(cfg.Width)*int64(cfg.Height) > domain.MaxImagePixels {
		return fmt.Errorf("%w: %d×%d pixels is more than %d", domain.ErrImageTooLarge, cfg.Width, cfg.Height, domain.MaxImagePixels)
	}
	return nil
}

// reencode decodes the spooled upload and encodes its pixels again into
// a new temporary file.
func (n *Normalizer) reencode(ctx context.Context, upload *os.File) (app.NormalizedImage, error) {
	select {
	case n.slots <- struct{}{}:
		defer func() { <-n.slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if _, err := upload.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	img, err := png.Decode(bufio.NewReader(upload))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrInvalidImage, err)
	}
	out, err := os.CreateTemp(n.dir, "glossa-capture-image-*")
	if err != nil {
		return nil, fmt.Errorf("imaging: spool: %w", err)
	}
	h := sha256.New()
	w := &counter{w: io.MultiWriter(out, h)}
	buf := bufio.NewWriter(w)
	err = (&png.Encoder{CompressionLevel: png.DefaultCompression}).Encode(buf, img)
	if err == nil {
		err = buf.Flush()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(out.Name())
		return nil, fmt.Errorf("imaging: re-encode: %w", err)
	}
	b := img.Bounds()
	return &spooled{path: out.Name(), size: w.n, image: domain.Image{Digest: sum(h), Width: b.Dx(), Height: b.Dy()}}, nil
}

func sum(h hash.Hash) domain.Digest { return domain.Digest(hex.EncodeToString(h.Sum(nil))) }

func remove(f *os.File) {
	_ = f.Close()
	_ = os.Remove(f.Name())
}

type counter struct {
	w io.Writer
	n int64
}

func (c *counter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// spooled is a re-encoded image in a temporary file.
type spooled struct {
	path  string
	size  int64
	image domain.Image
}

func (s *spooled) Image() domain.Image { return s.image }
func (s *spooled) Size() int64         { return s.size }

func (s *spooled) Open() (io.ReadCloser, error) { return os.Open(s.path) }

func (s *spooled) Close() error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
