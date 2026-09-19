package httpserver

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/felixgeelhaar/glossa/platform/internal/kernel/problem"
)

// recoverPanics turns a handler panic into a logged 500 so one bad
// request can't take the connection down with a half-written response.
// http.ErrAbortHandler is re-panicked: it's net/http's deliberate abort.
func recoverPanics(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				p := recover()
				if p == nil {
					return
				}
				if p == http.ErrAbortHandler { // identity comparison, as net/http itself does
					panic(p)
				}
				logger.ErrorContext(r.Context(), "handler panicked",
					slog.String("panic", fmt.Sprint(p)), slog.String("stack", string(debug.Stack())))
				problem.Write(w, http.StatusInternalServerError, "internal error")
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// limitBody caps request bodies: a declared oversize body is refused
// up front; an undeclared one fails with *http.MaxBytesError on read.
func limitBody(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > limit {
				problem.Write(w, http.StatusRequestEntityTooLarge,
					fmt.Sprintf("request body exceeds %d bytes", limit))
				return
			}
			if r.Body != nil && r.Body != http.NoBody {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// securityHeaders sets headers every API response should carry.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
