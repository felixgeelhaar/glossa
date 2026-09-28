package tools

import (
	"github.com/felixgeelhaar/glossa/platform/internal/mcp/app"
)

// Read returns the read tools of RFC 0005 §7.3, in the order a client
// sees them. Every one of them is in the read toolset, so a write
// session gets them too and a read session gets nothing else: the
// toolset gate in app.Service is what enforces that, and these tools
// declare only where they belong.
//
// A tool whose port is nil is left out. A deployment that runs without
// a context should not advertise a tool that cannot answer, and an
// agent that never sees a tool wastes no call discovering it is broken.
func Read(s Sources) []app.Tool {
	var out []app.Tool
	if s.Catalog != nil {
		out = append(out, catalogSearch(s.Catalog), messageGet(s.Catalog, s.Usages))
	}
	if s.Translations != nil {
		out = append(out, translationGet(s.Translations))
	}
	if s.Usages != nil {
		out = append(out, usagesGet(s.Usages))
	}
	if s.Knowledge != nil {
		out = append(out, tmSearch(s.Knowledge), termLookup(s.Knowledge), styleRules(s.Knowledge))
	}
	if s.Quality != nil {
		out = append(out, findingsList(s.Quality))
	}
	if s.Delivery != nil && s.Catalog != nil {
		out = append(out, explainDelivery(s.Delivery, s.Catalog))
	}
	return out
}
