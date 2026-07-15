package infolang

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLiveProbe exercises the SDK against a real runtime. It is skipped unless
// INFOLANG_LIVE_TEST is set, so the default `go test` run stays fully offline.
//
// Enable it with:
//
//	INFOLANG_LIVE_TEST=1 INFOLANG_API_KEY=il_live_... go test -run TestLiveProbe ./...
//
// INFOLANG_BASE_URL / INFOLANG_NAMESPACE are honored if set.
func TestLiveProbe(t *testing.T) {
	if os.Getenv("INFOLANG_LIVE_TEST") == "" {
		t.Skip("set INFOLANG_LIVE_TEST=1 (plus INFOLANG_API_KEY) to run the live probe")
	}
	if os.Getenv("INFOLANG_API_KEY") == "" && os.Getenv("INFOLANG_DEV_KEY") == "" {
		t.Fatal("INFOLANG_LIVE_TEST set but no INFOLANG_API_KEY / INFOLANG_DEV_KEY")
	}

	c, err := New("", WithTimeout(15*time.Second))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	health, err := c.Health(ctx)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	t.Logf("live health: status=%q model_loaded=%v engine_ok=%v", health.Status, health.ModelLoaded, health.EngineOK)

	query := os.Getenv("INFOLANG_LIVE_QUERY")
	if query == "" {
		query = "hello"
	}
	res, err := c.Investigate(ctx, query, nil)
	if err != nil {
		t.Fatalf("Investigate: %v", err)
	}
	t.Logf("live investigate: %d chunks (weak=%v)", len(res.Chunks), res.Weak())
}
