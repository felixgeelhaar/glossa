package domain_test

import (
	"errors"
	"testing"

	identity "github.com/felixgeelhaar/glossa/platform/internal/identity/domain"
	"github.com/felixgeelhaar/glossa/platform/internal/mcp/domain"
)

func TestParseToolset(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    domain.Toolset
		wantErr bool
	}{
		{name: "nothing asked for is a read session", in: "", want: domain.ToolsetRead},
		{name: "read", in: "read", want: domain.ToolsetRead},
		{name: "write", in: "write", want: domain.ToolsetWrite},
		{name: "admin is not a toolset", in: "admin", wantErr: true},
		{name: "unknown", in: "everything", wantErr: true},
		{name: "case matters", in: "Write", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := domain.ParseToolset(tc.in)
			if tc.wantErr {
				if !errors.Is(err, domain.ErrInvalidToolset) {
					t.Fatalf("err = %v, want ErrInvalidToolset", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if got != tc.want {
				t.Errorf("toolset = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestToolsetIncludes(t *testing.T) {
	tests := []struct {
		session, tool domain.Toolset
		want          bool
	}{
		{domain.ToolsetRead, domain.ToolsetRead, true},
		{domain.ToolsetRead, domain.ToolsetWrite, false},
		{domain.ToolsetWrite, domain.ToolsetRead, true},
		{domain.ToolsetWrite, domain.ToolsetWrite, true},
	}
	for _, tc := range tests {
		if got := tc.session.Includes(tc.tool); got != tc.want {
			t.Errorf("%s session includes a %s tool = %t, want %t", tc.session, tc.tool, got, tc.want)
		}
	}
}

// A toolset never asks for admin: MCP exposes no member, token,
// connection or tenant management (RFC 0005 §7.2).
func TestToolsetScopeIsNeverAdmin(t *testing.T) {
	for _, ts := range []domain.Toolset{domain.ToolsetRead, domain.ToolsetWrite} {
		if s := ts.Scope(); s == identity.ScopeAdmin || s == identity.ScopePublish {
			t.Errorf("%s asks for scope %q", ts, s)
		}
	}
	if got := domain.ToolsetWrite.Scope(); got != identity.ScopeWrite {
		t.Errorf("write toolset scope = %q, want %q", got, identity.ScopeWrite)
	}
}
