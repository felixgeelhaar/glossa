package capture

import (
	"github.com/felixgeelhaar/glossa/platform/internal/cli/extract"
)

// Schema is the document type Document carries.
const Schema = "glossa.captures/v1"

// Document is a glossa.captures/v1 document
// (runtimes/testdata/schemas/captures.v1.schema.json): one build's
// captures, each a page image and the regions where messages were
// rendered on it.
type Document struct {
	Schema      string       `json:"schema"`
	Application string       `json:"application"`
	Commit      string       `json:"commit"`
	Branch      string       `json:"branch"`
	Tool        extract.Tool `json:"tool"`
	Captures    []Capture    `json:"captures"`
}

// Capture is one (route, viewport, locale).
type Capture struct {
	Route    string   `json:"route"`
	URL      string   `json:"url"`
	Viewport Viewport `json:"viewport"`
	Locale   string   `json:"locale"`
	Image    Image    `json:"image"`
	Renders  []Render `json:"renders"`
	Regions  []Region `json:"regions"`
}

// Viewport is the CSS viewport; the image is its pixels times
// DeviceScaleFactor (omitted when 1).
type Viewport struct {
	Width             int     `json:"width"`
	Height            int     `json:"height"`
	DeviceScaleFactor float64 `json:"deviceScaleFactor,omitempty"`
}

// Image is the full-page PNG: its lowercase hex SHA-256 names its
// multipart part and its file.
type Image struct {
	SHA256 string `json:"sha256"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// Render is an entry of the page's render log: what a t() marker's index
// rendered.
type Render struct {
	Index  int    `json:"index"`
	Key    string `json:"key"`
	Locale string `json:"locale"`
}

// Region is where a rendered message is on the page: a component host
// (Key) or a marked t() string (Index into the renders).
type Region struct {
	Key   string `json:"key,omitempty"`
	Index *int   `json:"index,omitempty"`
	// Kind is element, text or attribute; Attribute names the attribute.
	Kind      string `json:"kind"`
	Attribute string `json:"attribute,omitempty"`
	Box       Box    `json:"box"`
	// Visible is false when the message rendered zero-size, off-screen,
	// hidden, or under a redaction.
	Visible bool `json:"visible"`
}

// Box is in CSS pixels from the top-left of the full page.
type Box struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// NewDocument returns the document for h and captures, in the plan's
// order.
func NewDocument(h extract.Header, captures []Capture) Document {
	if captures == nil {
		captures = []Capture{}
	}
	return Document{Schema: Schema, Application: h.Application, Commit: h.Commit, Branch: h.Branch, Tool: h.Tool, Captures: captures}
}

// VisibleKeys are the keys with at least one visible region in doc.
func (d Document) VisibleKeys() map[string]bool {
	seen := map[string]bool{}
	for _, c := range d.Captures {
		keys := make(map[int]string, len(c.Renders))
		for _, r := range c.Renders {
			keys[r.Index] = r.Key
		}
		for _, r := range c.Regions {
			switch {
			case !r.Visible:
			case r.Key != "":
				seen[r.Key] = true
			case r.Index != nil && keys[*r.Index] != "":
				seen[keys[*r.Index]] = true
			}
		}
	}
	return seen
}
