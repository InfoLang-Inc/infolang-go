package infolang

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

// Each endpoint returns a *ConfigError when the runtime body cannot be decoded
// into the expected typed shape. A type-mismatched field forces that branch.
func TestDecodeErrors(t *testing.T) {
	call := func(t *testing.T, body any, fn func(*Client) error) {
		t.Helper()
		ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, 200, body)
		})
		c := newTestClient(t, ms.URL)
		err := fn(c)
		var cfgErr *ConfigError
		if !errors.As(err, &cfgErr) {
			t.Fatalf("want ConfigError, got %v", err)
		}
	}

	ctx := context.Background()
	t.Run("recall", func(t *testing.T) {
		call(t, map[string]any{"count": "notint"}, func(c *Client) error {
			_, err := c.Recall(ctx, "q", nil)
			return err
		})
	})
	t.Run("remember", func(t *testing.T) {
		call(t, map[string]any{"total_memories": "notint"}, func(c *Client) error {
			_, err := c.Remember(ctx, "t", nil)
			return err
		})
	})
	t.Run("banks", func(t *testing.T) {
		call(t, map[string]any{"banks": "notarray"}, func(c *Client) error {
			_, err := c.ListBanks(ctx)
			return err
		})
	})
	t.Run("recent", func(t *testing.T) {
		call(t, map[string]any{"memories": "notarray"}, func(c *Client) error {
			_, err := c.ListRecent(ctx, nil)
			return err
		})
	})
	t.Run("contextpack", func(t *testing.T) {
		call(t, map[string]any{"tokens_estimated": "notint"}, func(c *Client) error {
			_, err := c.ContextPack(ctx, "q", nil)
			return err
		})
	})
	t.Run("execute", func(t *testing.T) {
		call(t, map[string]any{"results": "notarray"}, func(c *Client) error {
			_, err := c.Execute(ctx, nil)
			return err
		})
	})
	t.Run("health", func(t *testing.T) {
		call(t, map[string]any{"model_loaded": "notbool"}, func(c *Client) error {
			_, err := c.Health(ctx)
			return err
		})
	})
}

func TestRecallNilScoreHit(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"hits": []map[string]any{{"id": "m", "text": "t"}},
		})
	})
	c := newTestClient(t, ms.URL)
	res, err := c.Recall(context.Background(), "q", nil)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if res.Chunks[0].Score != 0 {
		t.Errorf("absent similarity should default to 0, got %v", res.Chunks[0].Score)
	}
}

func TestWithTimeoutAppliesToClient(t *testing.T) {
	clearEnv(t)
	c, err := New("k", WithTimeout(3*time.Second))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.t.httpClient.Timeout != 3*time.Second {
		t.Errorf("timeout = %v, want 3s", c.t.httpClient.Timeout)
	}
}

func TestDefaultRNGInRange(t *testing.T) {
	for i := 0; i < 100; i++ {
		v := defaultRNG()
		if v < 0 || v >= 1 {
			t.Fatalf("defaultRNG out of range: %v", v)
		}
	}
}
