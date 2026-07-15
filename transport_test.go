package infolang

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryThenSuccess(t *testing.T) {
	var calls atomic.Int32
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(503)
			return
		}
		writeJSON(w, 200, map[string]any{"status": "ok"})
	})
	c := newTestClient(t, ms.URL, WithMaxRetries(2))
	if _, err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("want 2 calls, got %d", calls.Load())
	}
}

func TestRetryExhaustedReturnsServerError(t *testing.T) {
	var calls atomic.Int32
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeJSON(w, 500, map[string]any{"error": "kaboom"})
	})
	c := newTestClient(t, ms.URL, WithMaxRetries(2))
	_, err := c.Health(context.Background())
	if !errors.Is(err, ErrServer) {
		t.Fatalf("want ErrServer, got %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Message != "kaboom" {
		t.Fatalf("want APIError with message, got %v", err)
	}
	if calls.Load() != 3 { // initial + 2 retries
		t.Errorf("want 3 calls, got %d", calls.Load())
	}
}

func TestRetryAfterHonored(t *testing.T) {
	var calls atomic.Int32
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(429)
			return
		}
		writeJSON(w, 200, map[string]any{"status": "ok"})
	})
	c := newTestClient(t, ms.URL, WithMaxRetries(1))
	var slept []time.Duration
	c.t.sleep = func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}
	if _, err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if len(slept) != 1 || slept[0] != 2*time.Second {
		t.Errorf("expected a 2s sleep from Retry-After, got %v", slept)
	}
}

func TestConnectionErrorRetriesThenFails(t *testing.T) {
	clearEnv(t)
	// Port 0 (or an unroutable address) forces a dial failure.
	c, err := New("k", WithBaseURL("http://127.0.0.1:1"), WithMaxRetries(1))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.t.sleep = func(_ context.Context, _ time.Duration) error { return nil }
	c.t.rng = func() float64 { return 0 }
	_, err = c.Health(context.Background())
	var connErr *ConnectionError
	if !errors.As(err, &connErr) {
		t.Fatalf("want ConnectionError, got %v", err)
	}
}

func TestContextCanceledNoRetry(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{})
	})
	c := newTestClient(t, ms.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Health(ctx)
	var connErr *ConnectionError
	if !errors.As(err, &connErr) {
		t.Fatalf("want ConnectionError, got %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("want wrapped context.Canceled, got %v", err)
	}
}

func TestSleepInterruptedByContext(t *testing.T) {
	var calls atomic.Int32
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(503)
	})
	c := newTestClient(t, ms.URL, WithMaxRetries(3))
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel during the first backoff sleep.
	c.t.sleep = func(_ context.Context, _ time.Duration) error {
		cancel()
		return context.Canceled
	}
	_, err := c.Health(ctx)
	var connErr *ConnectionError
	if !errors.As(err, &connErr) {
		t.Fatalf("want ConnectionError, got %v", err)
	}
}

func TestDelayBackoff(t *testing.T) {
	tr := &transport{
		backoffBase: 500 * time.Millisecond,
		backoffCap:  8 * time.Second,
		rng:         func() float64 { return 1.0 }, // full window
	}
	// retry-after takes precedence
	if got := tr.delay(0, 3); got != 3*time.Second {
		t.Errorf("retry-after delay = %v, want 3s", got)
	}
	// attempt 0 window = base
	if got := tr.delay(0, 0); got != 500*time.Millisecond {
		t.Errorf("attempt 0 = %v", got)
	}
	// attempt 2 window = base*4 = 2s
	if got := tr.delay(2, 0); got != 2*time.Second {
		t.Errorf("attempt 2 = %v", got)
	}
	// large attempt caps at backoffCap
	if got := tr.delay(20, 0); got != 8*time.Second {
		t.Errorf("capped delay = %v, want 8s", got)
	}
	// jitter of 0 -> 0 delay
	tr.rng = func() float64 { return 0 }
	if got := tr.delay(2, 0); got != 0 {
		t.Errorf("zero jitter = %v, want 0", got)
	}
}

func TestSleepCtx(t *testing.T) {
	if err := sleepCtx(context.Background(), 0); err != nil {
		t.Errorf("zero sleep: %v", err)
	}
	start := time.Now()
	if err := sleepCtx(context.Background(), 10*time.Millisecond); err != nil {
		t.Errorf("sleep: %v", err)
	}
	if time.Since(start) < 5*time.Millisecond {
		t.Error("sleep returned too early")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepCtx(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Errorf("want canceled, got %v", err)
	}
}

func TestParseMetering(t *testing.T) {
	h := http.Header{}
	h.Set("x-infolang-tokens-saved", "50")
	h.Set("x-infolang-chunks-used", "3")
	h.Set("x-infolang-repo-coverage", "0.75")
	h.Set("x-request-id", "req-1")
	m := parseMetering(h)
	if m.TokensSaved == nil || *m.TokensSaved != 50 {
		t.Errorf("tokens saved: %v", m.TokensSaved)
	}
	if m.ChunksUsed == nil || *m.ChunksUsed != 3 {
		t.Errorf("chunks used: %v", m.ChunksUsed)
	}
	if m.RepoCoverage == nil || *m.RepoCoverage != 0.75 {
		t.Errorf("repo coverage: %v", m.RepoCoverage)
	}
	if m.RequestID != "req-1" {
		t.Errorf("request id: %q", m.RequestID)
	}
}

func TestParseMeteringIgnoresGarbage(t *testing.T) {
	h := http.Header{}
	h.Set("x-infolang-tokens-saved", "notanint")
	h.Set("x-infolang-repo-coverage", "nope")
	m := parseMetering(h)
	if m.TokensSaved != nil || m.RepoCoverage != nil {
		t.Errorf("garbage should be ignored: %+v", m)
	}
}

func TestDecodeBody(t *testing.T) {
	if v := decodeBody([]byte("   ")); v != nil {
		t.Errorf("blank -> %v, want nil", v)
	}
	if v := decodeBody([]byte("not json")); v != "not json" {
		t.Errorf("non-json -> %v", v)
	}
	v := decodeBody([]byte(`{"a":1}`))
	m, ok := v.(map[string]any)
	if !ok || m["a"].(float64) != 1 {
		t.Errorf("json -> %v", v)
	}
}

func TestParseRetryAfterInvalid(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	if got := parseRetryAfter(resp); got != 0 {
		t.Errorf("empty -> %v", got)
	}
	resp.Header.Set("Retry-After", "soon")
	if got := parseRetryAfter(resp); got != 0 {
		t.Errorf("garbage -> %v", got)
	}
}
