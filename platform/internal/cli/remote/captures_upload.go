package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"sort"
)

// UploadedCaptures is the answer to a captures upload.
type UploadedCaptures struct {
	// Build is the ID of the context build the captures belong to.
	Build              string
	Captures           int
	ImagesStored       int
	ImagesDeduplicated int
	UnknownKeys        int
	// Replayed is true when the same upload was made before (200).
	Replayed bool
}

// capturesAnswer is the response body; build is the build's ID or the build.
type capturesAnswer struct {
	Build              json.RawMessage `json:"build"`
	Captures           int             `json:"captures"`
	ImagesStored       int             `json:"images_stored"`
	ImagesDeduplicated int             `json:"images_deduplicated"`
	UnknownKeys        int             `json:"unknown_keys"`
}

func (a capturesAnswer) buildID() string {
	var id string
	if json.Unmarshal(a.Build, &id) == nil {
		return id
	}
	var b struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(a.Build, &b)
	return b.ID
}

// UploadCaptures posts a glossa.captures/v1 manifest and its images to the
// Captures API (POST …/captures, multipart/form-data): the part
// `manifest` (application/json) first, then one image/png part per image,
// named by its lowercase hex SHA-256. It isn't retried: the images are
// streams.
func (c *Client) UploadCaptures(ctx context.Context, project Scope, manifest []byte, images map[string]io.Reader) (UploadedCaptures, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part := func(name, filename, contentType string) (io.Writer, error) {
		h := textproto.MIMEHeader{}
		disposition := fmt.Sprintf(`form-data; name=%q`, name)
		if filename != "" {
			disposition += fmt.Sprintf(`; filename=%q`, filename)
		}
		h.Set("Content-Disposition", disposition)
		h.Set("Content-Type", contentType)
		return w.CreatePart(h)
	}
	pw, err := part("manifest", "", "application/json")
	if err == nil {
		_, err = pw.Write(manifest)
	}
	digests := make([]string, 0, len(images))
	for d := range images {
		digests = append(digests, d)
	}
	sort.Strings(digests)
	for _, d := range digests {
		if err != nil {
			break
		}
		if pw, err = part(d, d+".png", "image/png"); err == nil {
			_, err = io.Copy(pw, images[d])
		}
	}
	if err == nil {
		err = w.Close()
	}
	if err != nil {
		return UploadedCaptures{}, err
	}
	url := c.path("/v1/tenants/%s/projects/%s/captures", project.Tenant, project.Project)
	req, err := c.transferRequest(ctx, http.MethodPost, url, &body)
	if err != nil {
		return UploadedCaptures{}, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := c.transfer.Do(req)
	if err != nil {
		return UploadedCaptures{}, &APIError{Method: http.MethodPost, URL: url, Err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return UploadedCaptures{}, problem(resp.StatusCode, raw, http.MethodPost, url)
	}
	var a capturesAnswer
	if err := json.Unmarshal(raw, &a); err != nil {
		return UploadedCaptures{}, &APIError{Method: http.MethodPost, URL: url, Status: resp.StatusCode, Code: "unexpected_response", Detail: err.Error()}
	}
	return UploadedCaptures{Build: a.buildID(), Captures: a.Captures, ImagesStored: a.ImagesStored,
		ImagesDeduplicated: a.ImagesDeduplicated, UnknownKeys: a.UnknownKeys, Replayed: resp.StatusCode == http.StatusOK}, nil
}
