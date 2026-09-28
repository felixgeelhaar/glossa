package tools

import (
	"context"
	"encoding/json"
	"fmt"

	identity "github.com/felixgeelhaar/glossa/platform/internal/identity/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/mcp/app"
	"github.com/felixgeelhaar/glossa/platform/internal/mcp/domain"
)

// FindingsListName is the findings tool's wire name.
const FindingsListName = "findings_list"

type findingsListArgs struct {
	Project  string `json:"project"`
	Ref      string `json:"ref"`
	Layer    string `json:"layer"`
	Locale   string `json:"locale"`
	Severity string `json:"severity"`
	Key      string `json:"key"`
	Waived   bool   `json:"waived"`
	Limit    *int   `json:"limit"`
	Cursor   string `json:"cursor"`
}

// FindingsListResult is a page of one check run's findings.
type FindingsListResult struct {
	Run      CheckRun  `json:"run"`
	Findings []Finding `json:"findings"`
	// NextCursor continues the page; "" on the last one.
	NextCursor string `json:"next_cursor,omitempty"`
}

// findingsList reads the stored findings of a project's latest check
// run (RFC 0005 §7.3). It reads them through Quality's application
// service, never over HTTP: MCP is a second façade on the ports, not a
// client of the REST API (RFC 0005 §7.1).
func findingsList(q Quality) app.Tool {
	return app.Tool{
		Name:  FindingsListName,
		Title: "List quality findings",
		Description: "The stored findings of a project's latest completed check run, filtered by " +
			"layer, locale, severity, message key or waived. The run itself is returned with them: " +
			"which ref it checked, which policy version graded it, which layers it computed — a " +
			"layer that is not in that list was not looked at, which is not the same as clean. " +
			"Paginated: pass the returned next_cursor to continue.",
		Toolset:     domain.ToolsetRead,
		Permission:  identity.PermCatalogRead,
		Selectors:   selectors,
		ReadOnly:    true,
		InputSchema: json.RawMessage(findingsListSchema),
		Handler: func(ctx context.Context, _ app.Session, raw json.RawMessage) (app.Result, error) {
			var a findingsListArgs
			if err := decode(raw, &a); err != nil {
				return app.Result{}, err
			}
			project, err := projectOf(a.Project)
			if err != nil {
				return app.Result{}, err
			}
			limit, err := limitOf(a.Limit, MaxLimit)
			if err != nil {
				return app.Result{}, err
			}
			run, found, next, err := q.Findings(ctx, project, FindingsQuery{
				Ref: a.Ref, Layer: a.Layer, Locale: a.Locale, Severity: a.Severity,
				MessageKey: a.Key, WaivedOnly: a.Waived, Limit: limit, After: a.Cursor,
			})
			if err != nil {
				return app.Result{}, err
			}
			if found == nil {
				found = []Finding{}
			}
			return app.Result{
				Explanation: fmt.Sprintf("%s from the %s run of %s (%d error(s), %d warning(s), %d waived).",
					plural(len(found), "finding", "findings"), run.Conclusion, run.Ref,
					run.Errors, run.Warnings, run.Waived),
				Data: FindingsListResult{Run: run, Findings: found, NextCursor: next},
			}, nil
		},
	}
}

const findingsListSchema = `{
  "type": "object",
  "properties": {
    "project": {"type": "string", "format": "uuid", "description": "The project's id."},
    "ref": {"type": "string", "maxLength": 255, "description": "The branch or environment the run checked; the project's latest run when absent."},
    "layer": {"type": "string", "enum": ["structure", "parity", "completeness", "terminology", "style", "length", "locale", "source", "visual", "linguistic"]},
    "locale": {"type": "string", "maxLength": 35},
    "severity": {"type": "string", "enum": ["error", "warning", "waived"]},
    "key": {"type": "string", "maxLength": 200, "description": "Only findings about this message key."},
    "waived": {"type": "boolean", "description": "Only findings a waiver accepted."},
    "limit": {"type": "integer", "minimum": 1, "maximum": 100, "description": "Page size; 20 by default."},
    "cursor": {"type": "string", "description": "The next_cursor of the previous page."}
  },
  "required": ["project"],
  "additionalProperties": false
}`
