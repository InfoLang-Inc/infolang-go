package infolang

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/url"
)

// ingestPath builds POST /v2/workspaces/{ws}/ingest?ns=&tag_prefix=.
func (c *Client) ingestPath(ctx context.Context, opts *IngestOptions) (string, error) {
	base, err := c.wsPath(ctx)
	if err != nil {
		return "", err
	}
	ns := opts.Namespace
	if ns == "" {
		ns = c.Namespace
	}
	q := url.Values{}
	if ns != "" {
		q.Set("ns", ns)
	}
	if opts.TagPrefix != "" {
		q.Set("tag_prefix", opts.TagPrefix)
	}
	path := base + "/ingest"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	return path, nil
}

func parseIngest(data any) *IngestJob {
	job := &IngestJob{}
	if m, ok := data.(map[string]any); ok {
		job.Raw = m
		if s, ok := m["status"].(string); ok {
			job.Status = s
		}
	}
	return job
}

// Ingest uploads a zip archive of text content
// (POST /v2/workspaces/{ws}/ingest, Content-Type application/zip). v1 of the
// endpoint is synchronous — the job returns finished. A nil *IngestOptions
// uses the client defaults.
func (c *Client) Ingest(ctx context.Context, archive []byte, opts *IngestOptions) (*IngestJob, error) {
	if opts == nil {
		opts = &IngestOptions{}
	}
	path, err := c.ingestPath(ctx, opts)
	if err != nil {
		return nil, err
	}
	contentType := opts.ContentType
	if contentType == "" {
		contentType = "application/zip"
	}
	resp, err := c.t.doRaw(ctx, "POST", path, archive, contentType)
	if err != nil {
		return nil, err
	}
	return parseIngest(resp.data), nil
}

// IngestFiles uploads individual text files as multipart form data with
// repeated "files" parts on the same ingest endpoint (the gateway dispatches
// on content type) — no zip step needed. A nil *IngestOptions uses the client
// defaults; ContentType is ignored, the multipart writer sets its own
// boundary.
func (c *Client) IngestFiles(ctx context.Context, files []IngestFile, opts *IngestOptions) (*IngestJob, error) {
	if opts == nil {
		opts = &IngestOptions{}
	}
	path, err := c.ingestPath(ctx, opts)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, f := range files {
		part, err := w.CreateFormFile("files", f.Name)
		if err != nil {
			return nil, &ConfigError{Message: "failed to build multipart body: " + err.Error()}
		}
		if _, err := part.Write(f.Content); err != nil {
			return nil, &ConfigError{Message: "failed to build multipart body: " + err.Error()}
		}
	}
	if err := w.Close(); err != nil {
		return nil, &ConfigError{Message: "failed to build multipart body: " + err.Error()}
	}

	resp, err := c.t.doRaw(ctx, "POST", path, buf.Bytes(), w.FormDataContentType())
	if err != nil {
		return nil, err
	}
	return parseIngest(resp.data), nil
}
