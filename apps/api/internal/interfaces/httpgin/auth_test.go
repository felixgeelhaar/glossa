package httpgin

import "testing"

func TestBearerToken(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantTok  string
		wantOK   bool
	}{
		{"happy", "Bearer glossa_abc", "glossa_abc", true},
		{"trims trailing space", "Bearer glossa_abc ", "glossa_abc", true},
		{"empty header", "", "", false},
		{"missing prefix", "glossa_abc", "", false},
		{"prefix only", "Bearer ", "", false},
		{"wrong prefix case", "bearer glossa_abc", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tok, ok := bearerToken(tc.input)
			if ok != tc.wantOK || tok != tc.wantTok {
				t.Errorf("bearerToken(%q) = (%q,%v) want (%q,%v)", tc.input, tok, ok, tc.wantTok, tc.wantOK)
			}
		})
	}
}
