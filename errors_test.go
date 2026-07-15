package infolang

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		status   int
		sentinel error
	}{
		{401, ErrAuthentication},
		{403, ErrAuthentication},
		{404, ErrNotFound},
		{400, ErrValidation},
		{422, ErrValidation},
		{429, ErrRateLimit},
		{500, ErrServer},
		{503, ErrServer},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, tt.status, map[string]any{"error": "nope"})
			})
			// No retries so a single error status returns immediately.
			c := newTestClient(t, ms.URL, WithMaxRetries(0))
			_, err := c.Recall(context.Background(), "q", nil)
			if !errors.Is(err, tt.sentinel) {
				t.Fatalf("status %d: want sentinel match, got %v", tt.status, err)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != tt.status {
				t.Fatalf("want APIError status %d, got %v", tt.status, err)
			}
		})
	}
}

func TestAPIErrorUnmatchedStatus(t *testing.T) {
	err := &APIError{StatusCode: 418, Message: "teapot"}
	for _, s := range []error{ErrAuthentication, ErrNotFound, ErrValidation, ErrRateLimit, ErrServer} {
		if errors.Is(err, s) {
			t.Errorf("418 should not match %v", s)
		}
	}
}

func TestAPIErrorString(t *testing.T) {
	withID := &APIError{StatusCode: 404, Message: "missing", RequestID: "r1"}
	if withID.Error() != "infolang: missing (status=404 request_id=r1)" {
		t.Errorf("with id: %q", withID.Error())
	}
	noID := &APIError{StatusCode: 500, Message: "boom"}
	if noID.Error() != "infolang: boom (status=500)" {
		t.Errorf("no id: %q", noID.Error())
	}
}

func TestRateLimitCarriesRetryAfter(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "5")
		writeJSON(w, 429, map[string]any{"message": "slow down"})
	})
	c := newTestClient(t, ms.URL, WithMaxRetries(0))
	_, err := c.Recall(context.Background(), "q", nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want APIError, got %v", err)
	}
	if apiErr.RetryAfter != 5 || apiErr.Message != "slow down" {
		t.Errorf("unexpected: retryAfter=%v msg=%q", apiErr.RetryAfter, apiErr.Message)
	}
}

func TestMessageFromBody(t *testing.T) {
	cases := []struct {
		body any
		want string
	}{
		{map[string]any{"error": "e"}, "e"},
		{map[string]any{"message": "m"}, "m"},
		{map[string]any{"detail": "d"}, "d"},
		{map[string]any{"other": "x"}, ""},
		{"raw string", "raw string"},
		{123, ""},
		{nil, ""},
	}
	for _, tc := range cases {
		if got := messageFromBody(tc.body); got != tc.want {
			t.Errorf("messageFromBody(%v) = %q, want %q", tc.body, got, tc.want)
		}
	}
}

func TestErrorFromResponseFallbackMessage(t *testing.T) {
	err := errorFromResponse(500, nil, "", 0)
	if err.Message != "request failed with status 500" {
		t.Errorf("fallback message = %q", err.Message)
	}
}

func TestConfigAndConnectionErrorStrings(t *testing.T) {
	cfg := &ConfigError{Message: "bad"}
	if cfg.Error() != "infolang: bad" {
		t.Errorf("config: %q", cfg.Error())
	}
	inner := errors.New("dial tcp")
	conn := &ConnectionError{Err: inner}
	if !errors.Is(conn, inner) {
		t.Error("ConnectionError should unwrap to inner")
	}
	if conn.Error() == "" {
		t.Error("empty connection error string")
	}
}
