package infolang

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// recordedRequest captures what the mock server received.
type recordedRequest struct {
	Method      string
	Path        string
	EscapedPath string
	Query       string
	Header      http.Header
	Body        map[string]any
	Raw         string
}

// mockServer is an httptest server plus the last request it handled.
type mockServer struct {
	*httptest.Server
	last *recordedRequest
}

// newMockServer starts a server that runs handler for every request while
// recording the request into ms.last.
func newMockServer(t *testing.T, handler http.HandlerFunc) *mockServer {
	t.Helper()
	ms := &mockServer{}
	ms.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		rec := &recordedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			EscapedPath: r.URL.EscapedPath(),
			Query:       r.URL.RawQuery,
			Header:      r.Header.Clone(),
			Raw:         string(raw),
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &rec.Body)
		}
		ms.last = rec
		handler(w, r)
	}))
	t.Cleanup(ms.Close)
	return ms
}

// writeJSON is a handler helper that emits a JSON body with a status code.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// newTestClient builds a Client pointed at url with retries and jitter made
// deterministic so backoff never slows the suite.
func newTestClient(t *testing.T, url string, opts ...Option) *Client {
	t.Helper()
	base := []Option{WithBaseURL(url)}
	c, err := New("il_live_test", append(base, opts...)...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.t.sleep = func(_ context.Context, _ time.Duration) error { return nil }
	c.t.rng = func() float64 { return 0 }
	return c
}
