// Package messageformat is Glossa's MessageFormat kernel for Go.
//
// The canonical representation of every message is the Unicode
// MessageFormat 2 data model (RFC 0002 §5). This package parses ICU
// MessageFormat 1 and MessageFormat 2 source into that model, derives
// the metadata the platform needs (arguments and their types,
// selectors, markup), checks that a translation is structurally
// compatible with its source, and formats messages for the Go runtime.
//
// Third-party engines sit behind this package's API so they can be
// replaced; the conformance suite in testdata/ proves any replacement.
package messageformat
