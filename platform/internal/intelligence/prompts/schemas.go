package prompts

// The answer contracts of the prompts, as JSON Schemas in the subset every
// provider's structured output accepts (all properties required, no
// additional properties, no numeric bounds).

// DraftAnswer is the translate and repair answer.
type DraftAnswer struct {
	Message string `json:"message"`
	Notes   string `json:"notes"`
}

// DraftSchema constrains DraftAnswer.
func DraftSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{"type": "string", "description": "The translation in MessageFormat 2 syntax."},
			"notes":   map[string]any{"type": "string", "description": "One short sentence about an ambiguity, or empty."},
		},
		"required":             []any{"message", "notes"},
		"additionalProperties": false,
	}
}

// AssessAnswer is the assess answer.
type AssessAnswer struct {
	Score       float64  `json:"score"`
	FormalityOK bool     `json:"formality_ok"`
	Issues      []string `json:"issues"`
}

// AssessSchema constrains AssessAnswer.
func AssessSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"score":        map[string]any{"type": "number", "description": "Probability in [0, 1] that a reviewer approves without edits."},
			"formality_ok": map[string]any{"type": "boolean"},
			"issues":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required":             []any{"score", "formality_ok", "issues"},
		"additionalProperties": false,
	}
}
