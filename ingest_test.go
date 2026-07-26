package infolang

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

func TestIngestZipBytes(t *testing.T) {
	archive := []byte("PK\x03\x04fake-zip-bytes")
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"status": "finished", "chunks": 12})
	})
	c := newTestClient(t, ms.URL)
	job, err := c.Ingest(context.Background(), archive, &IngestOptions{Namespace: "docs", TagPrefix: "repo"})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if ms.last.Method != "POST" || ms.last.Path != "/v2/workspaces/ws/ingest" {
		t.Errorf("route = %s %s", ms.last.Method, ms.last.Path)
	}
	if ms.last.Query != "ns=docs&tag_prefix=repo" {
		t.Errorf("query = %q", ms.last.Query)
	}
	if got := ms.last.Header.Get("Content-Type"); got != "application/zip" {
		t.Errorf("content type = %q, want application/zip", got)
	}
	// Zip bytes ride the body verbatim — no multipart wrapping.
	if !bytes.Equal(ms.last.RawBytes, archive) {
		t.Errorf("body not sent verbatim: %q", ms.last.Raw)
	}
	if job.Status != "finished" || job.Raw["chunks"].(float64) != 12 {
		t.Errorf("unexpected job: %+v", job)
	}
}

func TestIngestDefaultsClientNamespaceAndNilOpts(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"status": "finished"})
	})
	c := newTestClient(t, ms.URL, WithNamespace("clientns"))
	if _, err := c.Ingest(context.Background(), []byte("zip"), nil); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if ms.last.Query != "ns=clientns" {
		t.Errorf("query = %q", ms.last.Query)
	}
}

func TestIngestCustomContentType(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"status": "finished"})
	})
	c := newTestClient(t, ms.URL)
	_, err := c.Ingest(context.Background(), []byte("raw"), &IngestOptions{ContentType: "application/octet-stream"})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if got := ms.last.Header.Get("Content-Type"); got != "application/octet-stream" {
		t.Errorf("content type = %q", got)
	}
}

func TestIngestFilesMultipart(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"status": "finished"})
	})
	c := newTestClient(t, ms.URL)
	files := []IngestFile{
		{Name: "a.md", Content: []byte("alpha doc")},
		{Name: "b.txt", Content: []byte("beta doc")},
	}
	job, err := c.IngestFiles(context.Background(), files, &IngestOptions{Namespace: "docs", TagPrefix: "repo"})
	if err != nil {
		t.Fatalf("IngestFiles: %v", err)
	}
	if job.Status != "finished" {
		t.Errorf("unexpected job: %+v", job)
	}
	if ms.last.Path != "/v2/workspaces/ws/ingest" || ms.last.Query != "ns=docs&tag_prefix=repo" {
		t.Errorf("route = %s?%s", ms.last.Path, ms.last.Query)
	}

	// The multipart boundary must ride the Content-Type header.
	mediaType, params, err := mime.ParseMediaType(ms.last.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
		t.Fatalf("content type = %q err=%v", ms.last.Header.Get("Content-Type"), err)
	}

	// Every file is a repeated `files` part, in order, content intact.
	reader := multipart.NewReader(bytes.NewReader(ms.last.RawBytes), params["boundary"])
	var names, fields, contents []string
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("NextPart: %v", err)
		}
		body, _ := io.ReadAll(part)
		fields = append(fields, part.FormName())
		names = append(names, part.FileName())
		contents = append(contents, string(body))
	}
	if len(fields) != 2 || fields[0] != "files" || fields[1] != "files" {
		t.Errorf("form fields = %v, want repeated files parts", fields)
	}
	if names[0] != "a.md" || names[1] != "b.txt" {
		t.Errorf("file names = %v", names)
	}
	if contents[0] != "alpha doc" || contents[1] != "beta doc" {
		t.Errorf("contents = %v", contents)
	}
}

func TestIngestFilesNilOptsUsesClientNamespace(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"status": "finished"})
	})
	c := newTestClient(t, ms.URL, WithNamespace("clientns"))
	if _, err := c.IngestFiles(context.Background(), []IngestFile{{Name: "a", Content: []byte("x")}}, nil); err != nil {
		t.Fatalf("IngestFiles: %v", err)
	}
	if ms.last.Query != "ns=clientns" {
		t.Errorf("query = %q", ms.last.Query)
	}
	if !strings.HasPrefix(ms.last.Header.Get("Content-Type"), "multipart/form-data") {
		t.Errorf("content type = %q", ms.last.Header.Get("Content-Type"))
	}
}

func TestIngestNonObjectResponse(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, "ok")
	})
	c := newTestClient(t, ms.URL)
	job, err := c.Ingest(context.Background(), []byte("zip"), nil)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if job.Status != "" || job.Raw != nil {
		t.Errorf("unexpected job: %+v", job)
	}
}
