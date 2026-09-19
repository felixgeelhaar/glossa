package glossa

import (
	"encoding/json"
	"testing"
)

// mutateJSON decodes a JSON object, lets mutate change it and re-encodes it.
func mutateJSON(t *testing.T, raw []byte, mutate func(m map[string]any)) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	mutate(m)
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func manifestWithSignatures(t *testing.T, raw []byte, sigs []any) []byte {
	t.Helper()
	return mutateJSON(t, raw, func(m map[string]any) { m["signatures"] = sigs })
}
