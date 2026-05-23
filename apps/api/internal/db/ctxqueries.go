// Package db is sqlc-generated. This file is hand-written: a context
// helper that lets the RLS middleware bind a tx-scoped Queries on
// the request and have repositories read it back implicitly.
//
// Pattern: rlsTxMiddleware opens `BEGIN; SET LOCAL app.current_tenant
// = '...';` per request, builds a [Queries] off the tx, and stuffs
// it into the context. Every repository method does
// `q := db.QueriesFromContext(ctx, r.fallback)` and the tx-scoped
// queries flow naturally; ifsomehow no middleware ran, the fallback
// (pool-direct queries) is used so unauth paths like project-create
// still work.
package db

import "context"

type ctxKey struct{}

// WithQueries returns a derived context that carries the given
// Queries. Lookup is by the package-private [ctxKey] so callers
// can't typo-shadow it.
func WithQueries(ctx context.Context, q *Queries) context.Context {
	return context.WithValue(ctx, ctxKey{}, q)
}

// QueriesFromContext extracts the Queries set by [WithQueries] or
// returns the supplied fallback. The fallback covers unauthed paths
// (project create) where no tx + SET LOCAL setup is needed.
func QueriesFromContext(ctx context.Context, fallback *Queries) *Queries {
	if q, ok := ctx.Value(ctxKey{}).(*Queries); ok && q != nil {
		return q
	}
	return fallback
}
