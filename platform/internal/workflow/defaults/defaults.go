// Package defaults holds the workflow definitions the platform seeds,
// as documents.
//
// The default review workflow is data, not code (RFC 0006 §2.1 rule 2,
// §2.3): review.json is a glossa.workflow/v1 document, seeded per tenant
// on first use by the instance runner (wave 2) and from then on the
// tenant's own, editable and versioned like any other definition.
// Deleting it is allowed and means "no workflow", which is M4's
// behaviour.
//
// It reproduces the project setting `review_required` exactly by adding
// nothing to it: Localization already puts a new translation in
// needs_review when review is required, and only a reviewer's decision
// — through Localization's own ReviewTranslation port, with its own
// permission check — moves it on. The workflow waits for that decision
// and finishes. It sets no review state and creates no new way to
// approve text.
package defaults

import _ "embed"

//go:embed review.json
var review []byte

// Review returns the default review definition's document.
func Review() []byte { return append([]byte(nil), review...) }
