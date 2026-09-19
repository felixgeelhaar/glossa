// Package problem writes RFC 9457 problem details, the error body of
// every glossa-server HTTP response.
package problem

import (
	"encoding/json"
	"net/http"
)

// ContentType is the RFC 9457 media type.
const ContentType = "application/problem+json"

// Details is the RFC 9457 body. Type defaults to about:blank, so Title
// is the status text unless a more specific one is given.
type Details struct {
	Type   string `json:"type,omitempty"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Write sends a problem response with the given status and detail.
func Write(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Details{
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
	})
}
