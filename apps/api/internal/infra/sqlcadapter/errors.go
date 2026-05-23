package sqlcadapter

import "errors"

// ErrNotFound is returned when a lookup misses. Use cases map this
// to a 404 at the HTTP boundary.
var ErrNotFound = errors.New("sqlcadapter: not found")
