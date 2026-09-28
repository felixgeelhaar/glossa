// Package tools declares MCP's read tools (RFC 0005 §7.3) and the
// narrow ports they call.
//
// Every tool here is a thin call into another context's application
// service, never a second implementation of a rule (RFC 0005 §7.1). The
// contexts are reached through the ports in ports.go, whose adapters
// live in internal/mcp/adapters/sources; this package therefore imports
// no other context and can be tested with fakes and no database.
//
// Three rules hold for every tool in this package.
//
// # Tenancy is never an argument
//
// A session is bound to its token's tenant when it opens
// (app.Session.Context), and no tool takes a tenant. A project, message
// or release id from another tenant is simply not there: row-level
// security scopes every read to the session's tenant, so the query
// finds nothing and the tool answers "not found". It never answers
// "forbidden", because a forbidden-with-detail is itself the leak — it
// confirms the id exists somewhere.
//
// # Every list is bounded
//
// Each list tool takes `limit`, defaulting to DefaultLimit and capped
// at MaxLimit, and returns `next_cursor` wherever the underlying query
// is keyset-paginated. Where it is not — a message's usages, a TM
// lookup, the recognized terms of a text — the result carries
// `truncated` instead, and the tool's description says so. No tool
// returns an unbounded catalog.
//
// # Results are shapes, not rows
//
// Each tool answers with the ids an agent needs to ask its next
// question and the few fields it needs to decide. What each shape
// leaves out, and why, is documented on the shape itself. Nothing here
// returns a stored row: timestamps, versions, ETags, provenance blobs
// and internal counters are the REST API's business, and an agent that
// needs them can follow the ids.
package tools
