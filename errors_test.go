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

// The gateway /v2 envelope {"error":{code,message}} renders as "code: message"
// and surfaces the machine-readable Code.
func TestV2ErrorEnvelope(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 404, map[string]any{
			"error": map[string]any{"code": "memory_not_found", "message": "no such memory"},
		})
	})
	c := newTestClient(t, ms.URL, WithMaxRetries(0))
	err := c.Forget(context.Background(), "mem-x", nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want APIError, got %v", err)
	}
	if apiErr.Message != "memory_not_found: no such memory" {
		t.Errorf("message = %q, want code: message", apiErr.Message)
	}
	if apiErr.Code != "memory_not_found" {
		t.Errorf("code = %q", apiErr.Code)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("404 should match ErrNotFound")
	}
}

// lane_not_supported is 403 (was 401) — both map to ErrAuthentication, so no
// caller change is needed.
func TestLaneNotSupported403IsAuthError(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 403, map[string]any{
			"error": map[string]any{"code": "lane_not_supported", "message": "dev keys cannot reach /v2"},
		})
	})
	c := newTestClient(t, ms.URL, WithMaxRetries(0))
	_, err := c.Recall(context.Background(), "q", nil)
	if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("403 lane_not_supported should be ErrAuthentication, got %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "lane_not_supported" {
		t.Errorf("code = %v", err)
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
		name string
		body any
		want string
	}{
		{"v2 envelope", map[string]any{"error": map[string]any{"code": "c", "message": "m"}}, "c: m"},
		{"v2 envelope message only", map[string]any{"error": map[string]any{"message": "m"}}, "m"},
		{"v2 envelope code only", map[string]any{"error": map[string]any{"code": "c"}}, "c"},
		{"flat error", map[string]any{"error": "e"}, "e"},
		{"flat message", map[string]any{"message": "m"}, "m"},
		{"flat detail", map[string]any{"detail": "d"}, "d"},
		{"no known key", map[string]any{"other": "x"}, ""},
		{"raw string", "raw string", "raw string"},
		{"number", 123, ""},
		{"nil", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := messageFromBody(tc.body); got != tc.want {
				t.Errorf("messageFromBody(%v) = %q, want %q", tc.body, got, tc.want)
			}
		})
	}
}

func TestErrorCode(t *testing.T) {
	if got := errorCode(map[string]any{"error": map[string]any{"code": "x"}}); got != "x" {
		t.Errorf("code = %q", got)
	}
	if got := errorCode(map[string]any{"error": "flat"}); got != "" {
		t.Errorf("flat error should have no code, got %q", got)
	}
	if got := errorCode(nil); got != "" {
		t.Errorf("nil body should have no code, got %q", got)
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
