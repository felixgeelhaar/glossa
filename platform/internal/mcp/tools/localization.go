package tools

import (
	"context"
	"encoding/json"
	"fmt"

	identity "github.com/felixgeelhaar/glossa/platform/internal/identity/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/mcp/app"
	"github.com/felixgeelhaar/glossa/platform/internal/mcp/domain"
)

// TranslationGetName is the translation read tool's wire name.
const TranslationGetName = "translation_get"

type translationGetArgs struct {
	Project string `json:"project"`
	Key     string `json:"key"`
	Locale  string `json:"locale"`
}

// translationGet reads one locale's text for a message, with its
// provenance, review state and outdated flag (RFC 0005 §7.3).
func translationGet(t Translations) app.Tool {
	return app.Tool{
		Name:  TranslationGetName,
		Title: "Read a translation",
		Description: "Read a message's translation in one locale: the text, its review state, who " +
			"and what wrote it (human, ai, translation-memory, import), the source revision it was " +
			"made against, and whether the source has moved on since (outdated).",
		Toolset:     domain.ToolsetRead,
		Permission:  identity.PermTranslationsRead,
		Selectors:   selectors,
		ReadOnly:    true,
		InputSchema: json.RawMessage(translationGetSchema),
		Handler: func(ctx context.Context, _ app.Session, raw json.RawMessage) (app.Result, error) {
			var a translationGetArgs
			if err := decode(raw, &a); err != nil {
				return app.Result{}, err
			}
			project, err := projectOf(a.Project)
			if err != nil {
				return app.Result{}, err
			}
			if err := required("key", a.Key); err != nil {
				return app.Result{}, err
			}
			if err := required("locale", a.Locale); err != nil {
				return app.Result{}, err
			}
			tr, err := t.Translation(ctx, project, a.Key, a.Locale)
			if err != nil {
				return app.Result{}, err
			}
			state := tr.State
			if tr.Outdated {
				state += ", outdated"
			}
			return app.Result{
				Explanation: fmt.Sprintf("%s in %s is %s (written by %s).", tr.Key, tr.Locale, state, tr.Origin),
				Data:        tr,
			}, nil
		},
	}
}

const translationGetSchema = `{
  "type": "object",
  "properties": {
    "project": {"type": "string", "format": "uuid", "description": "The project's id."},
    "key": {"type": "string", "maxLength": 200, "description": "The message key."},
    "locale": {"type": "string", "maxLength": 35, "description": "A BCP 47 locale, e.g. \"de-AT\"."}
  },
  "required": ["project", "key", "locale"],
  "additionalProperties": false
}`
