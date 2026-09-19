package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
)

// ContextBuild is what the Context API answers to a usages upload.
type ContextBuild struct {
	ID string `json:"id"`
	// UnknownKeys counts usages whose key the catalog doesn't know.
	UnknownKeys int `json:"unknown_keys"`
	// Replayed is true when the same document was uploaded before.
	Replayed bool `json:"replayed"`
}

// UploadUsages POSTs a glossa.usages/v1 document to the project's
// context builds (RFC 0004 §2.2, §6.3). source is the collector
// (extract, plugin); defaultBranch the repository's default branch, when
// known. The upload is idempotent by document digest, so it is retried
// like a GET.
//
// The route isn't in the generated client yet: this is a thin call
// against POST /v1/tenants/{tenant}/projects/{project}/context-builds,
// to be moved onto the generated client once the operation is in
// platform/api/openapi.yaml.
func (c *Client) UploadUsages(ctx context.Context, s Scope, doc []byte, source, defaultBranch string) (ContextBuild, error) {
	q := url.Values{"source": {source}}
	if defaultBranch != "" {
		q.Set("default_branch", defaultBranch)
	}
	ref := c.path("/v1/tenants/%s/projects/%s/context-builds", s.Tenant, s.Project) + "?" + q.Encode()
	req, err := c.transferRequest(idempotent(ctx), http.MethodPost, ref, bytes.NewReader(doc))
	if err != nil {
		return ContextBuild{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.doer.Do(req)
	if err != nil {
		return ContextBuild{}, &APIError{Method: req.Method, URL: req.URL.String(), Err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return ContextBuild{}, problem(resp.StatusCode, body, req.Method, req.URL.String())
	}
	var out ContextBuild
	_ = json.Unmarshal(body, &out)
	return out, nil
}
