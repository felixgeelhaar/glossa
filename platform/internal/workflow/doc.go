// Package workflow is the Workflow bounded context (RFC 0006 §2): an
// organisation's localization process as data.
//
// Intent §42 is the constraint the context exists to honour —
// "translation workflows must be configurable; do not encode one
// organisation's localization process into the core domain model" — and
// RFC 0006 §2.1 names what would break it in this codebase:
//
//  1. a fifth review state in localization/domain.ReviewState;
//  2. a process compiled into Go (the default workflow is a document,
//     defaults/review.json, loaded exactly like a tenant's);
//  3. Localization, Release or Intelligence branching on who the tenant
//     is;
//  4. guards or actions that name a process rather than a generic
//     primitive with parameters.
//
// architecture_test.go in this directory is what makes those four
// enforceable rather than a matter of discipline. The layout follows the
// other contexts:
//
//   - domain: the glossa.workflow/v1 document, the closed vocabulary of
//     events, guards and actions, compiling a definition through
//     statekit.FromJSON, lint-on-save, and bindings with the platform's
//     one precedence rule.
//   - app: saving definitions as immutable versions, binding them,
//     resolving the binding for a subject.
//   - adapters/postgres: the workflow_* tables (migration 0040).
//   - defaults: the default definition, as a document.
//
// Workflow depends on the other contexts' ports; none of them depends
// on Workflow.
package workflow
