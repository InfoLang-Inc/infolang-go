package infolang

import (
	"context"
	"net/http"
	"testing"
)

func TestHealth(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"status": "ok", "model_loaded": true, "engine_ok": true, "dev_mode": false,
		})
	})
	c := newTestClient(t, ms.URL)
	h, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if h.Status != "ok" || !h.ModelLoaded || !h.EngineOK || h.DevMode {
		t.Errorf("unexpected health: %+v", h)
	}
	if ms.last.Method != "GET" || ms.last.Path != "/v1/health" {
		t.Errorf("unexpected route: %s %s", ms.last.Method, ms.last.Path)
	}
}
