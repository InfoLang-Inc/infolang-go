package infolang

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var retryStatuses = map[int]bool{429: true, 500: true, 502: true, 503: true, 504: true}

// transport wraps *http.Client with the resilience defaults expected of a modern
// SDK: targeted retries (429 + 5xx) using exponential backoff with full jitter,
// consistent header shaping, and typed error mapping.
type transport struct {
	baseURL     string
	httpClient  *http.Client
	auth        authProvider
	userAgent   string
	workspaceID string
	maxRetries  int
	backoffBase time.Duration
	backoffCap  time.Duration
	// sleep is overridable in tests so backoff does not slow the suite.
	sleep func(context.Context, time.Duration) error
	// rng supplies jitter; overridable in tests.
	rng func() float64
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// response carries the decoded body plus per-request metering metadata.
type response struct {
	data     any
	metering *Metering
}

func (t *transport) delay(attempt int, retryAfter float64) time.Duration {
	if retryAfter > 0 {
		return time.Duration(retryAfter * float64(time.Second))
	}
	window := time.Duration(float64(t.backoffBase) * math.Pow(2, float64(attempt)))
	if window > t.backoffCap {
		window = t.backoffCap
	}
	return time.Duration(t.rng() * float64(window))
}

// do issues an HTTP request with retries. body, when non-nil, is JSON-encoded.
func (t *transport) do(ctx context.Context, method, path string, body any) (*response, error) {
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			return nil, &ConfigError{Message: "failed to encode request body: " + err.Error()}
		}
	}

	var lastErr error
	for attempt := 0; attempt <= t.maxRetries; attempt++ {
		var reader io.Reader
		if encoded != nil {
			reader = bytes.NewReader(encoded)
		}
		req, err := http.NewRequestWithContext(ctx, method, t.baseURL+path, reader)
		if err != nil {
			return nil, &ConfigError{Message: "failed to build request: " + err.Error()}
		}
		t.applyHeaders(req, encoded != nil)

		resp, err := t.httpClient.Do(req)
		if err != nil {
			// Never retry a canceled/expired context.
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, &ConnectionError{Err: err}
			}
			lastErr = err
			if attempt >= t.maxRetries {
				return nil, &ConnectionError{Err: err}
			}
			if serr := t.sleep(ctx, t.delay(attempt, 0)); serr != nil {
				return nil, &ConnectionError{Err: serr}
			}
			continue
		}

		if retryStatuses[resp.StatusCode] && attempt < t.maxRetries {
			ra := parseRetryAfter(resp)
			drainAndClose(resp)
			if serr := t.sleep(ctx, t.delay(attempt, ra)); serr != nil {
				return nil, &ConnectionError{Err: serr}
			}
			continue
		}
		return t.finish(resp)
	}

	return nil, &ConnectionError{Err: lastErr}
}

func (t *transport) applyHeaders(req *http.Request, hasBody bool) {
	req.Header.Set("User-Agent", t.userAgent)
	req.Header.Set("Accept", "application/json")
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}
	t.auth.apply(req.Header)
	if t.workspaceID != "" {
		req.Header.Set("X-InfoLang-Workspace-Id", t.workspaceID)
	}
}

func (t *transport) finish(resp *http.Response) (*response, error) {
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	metering := parseMetering(resp.Header)
	decoded := decodeBody(raw)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return &response{data: decoded, metering: metering}, nil
	}
	return nil, errorFromResponse(resp.StatusCode, decoded, metering.RequestID, parseRetryAfter(resp))
}

// decodeBody returns a parsed JSON value, falling back to the raw string.
func decodeBody(raw []byte) any {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return v
}

func drainAndClose(resp *http.Response) {
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func parseRetryAfter(resp *http.Response) float64 {
	raw := resp.Header.Get("Retry-After")
	if raw == "" {
		return 0
	}
	if v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil {
		return v
	}
	return 0
}

func parseMetering(h http.Header) *Metering {
	m := &Metering{RequestID: h.Get("x-request-id")}
	if v := h.Get("x-infolang-tokens-saved"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			m.TokensSaved = &n
		}
	}
	if v := h.Get("x-infolang-chunks-used"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			m.ChunksUsed = &n
		}
	}
	if v := h.Get("x-infolang-repo-coverage"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			m.RepoCoverage = &f
		}
	}
	return m
}

// remarshal round-trips a decoded any value into a typed struct via JSON so the
// parsers can lean on struct tags rather than manual map access.
func remarshal(src any, dst any) error {
	buf, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(buf, dst)
}

// defaultRNG is the production jitter source.
func defaultRNG() float64 { return rand.Float64() }
