//go:build system

package m5_test

import (
	"bytes"
	"context"
	"strings"

	"github.com/felixgeelhaar/glossa/platform/internal/cli"
	"github.com/felixgeelhaar/glossa/platform/internal/cli/credentials"
)

// memStore is an in-memory credentials.Store: the exit test never
// touches the developer's keychain.
type memStore struct{ tokens map[string]string }

func (m *memStore) Get(s string) (string, error) {
	if t, ok := m.tokens[s]; ok {
		return t, nil
	}
	return "", credentials.ErrNotFound
}
func (m *memStore) Set(s, t string) error { m.tokens[s] = t; return nil }
func (m *memStore) Delete(s string) error { delete(m.tokens, s); return nil }
func (m *memStore) Name() string          { return "memory" }

// cliResult is one CLI run.
type cliResult struct {
	code           int
	stdout, stderr string
}

func (r cliResult) String() string {
	out := strings.TrimSpace(r.stderr)
	if out == "" {
		out = strings.TrimSpace(r.stdout)
	}
	return lastLines(out, 4)
}

// glossa runs the CLI in process — the code path the binary's main
// takes — in dir, with env as its whole environment.
func glossa(dir string, env map[string]string, args ...string) cliResult {
	var out, errb bytes.Buffer
	code := cli.Main(context.Background(), args, cli.Env{
		Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb,
		Getenv:      func(k string) string { return env[k] },
		Dir:         dir,
		Credentials: &memStore{tokens: map[string]string{}},
		Version:     "0.0.0-m5",
	})
	return cliResult{code: code, stdout: out.String(), stderr: errb.String()}
}
