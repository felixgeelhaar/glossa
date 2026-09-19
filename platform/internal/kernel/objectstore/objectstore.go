// Package objectstore is the port to object storage (RFC 0002 §6): an
// S3-compatible bucket in deployments (MinIO on k3s, Hetzner Object
// Storage in production), a directory or memory for single-node
// development and tests. Release artifacts and manifests live here, and
// glossa-edge serves them from here and nowhere else (RFC 0002 §3).
//
// Objects are small, whole values: they are written and read in one
// piece, never streamed or appended. Keys are slash-separated relative
// paths ("v1/projects/…/manifest.json").
package objectstore

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrNotFound means no object has the key.
var ErrNotFound = errors.New("objectstore: no such object")

// ErrInvalidKey means a key is not a clean relative path.
var ErrInvalidKey = errors.New("objectstore: invalid key")

// ErrTooLarge means an object exceeds the size a read allows.
var ErrTooLarge = errors.New("objectstore: object is larger than allowed")

// Reader reads objects.
type Reader interface {
	// Get returns an object's bytes; ErrNotFound if it doesn't exist.
	// Objects larger than maxBytes fail with ErrTooLarge.
	Get(ctx context.Context, key string, maxBytes int64) ([]byte, error)
	// Exists reports whether an object exists.
	Exists(ctx context.Context, key string) (bool, error)
}

// Writer writes objects.
type Writer interface {
	// Put stores body under key, replacing any object there. Readers
	// see either the old or the new bytes, never a mix.
	Put(ctx context.Context, key string, body []byte, contentType string) error
	// Delete removes an object; deleting a missing one is no error.
	Delete(ctx context.Context, key string) error
}

// Store reads and writes objects.
type Store interface {
	Reader
	Writer
}

var segment = regexp.MustCompile(`^[A-Za-z0-9_.=-]+$`)

// MaxKeyLen bounds a key (S3 allows 1024 bytes).
const MaxKeyLen = 900

// CheckKey validates key: 1..MaxKeyLen bytes of slash-separated segments
// of [A-Za-z0-9_.=-], none of them "." or "..", no leading or trailing
// slash. Every adapter enforces it, so a key never escapes its bucket or
// directory.
func CheckKey(key string) error {
	if key == "" || len(key) > MaxKeyLen {
		return fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	for _, s := range strings.Split(key, "/") {
		if s == "." || s == ".." || !segment.MatchString(s) {
			return fmt.Errorf("%w: %q", ErrInvalidKey, key)
		}
	}
	return nil
}
