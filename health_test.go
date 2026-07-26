package infolang

import (
	"context"
	"net/http"
	"testing"
)

func TestHealth(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"status": "ok", "uptime_s": 12})
	})
	c := newTestClient(t, ms.URL)
	h, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if h.Status != "ok" || h.Raw["uptime_s"].(float64) != 12 {
		t.Errorf("unexpected health: %+v", h)
	}
	if ms.last.Method != "GET" || ms.last.Path != "/healthz" {
		t.Errorf("unexpected route: %s %s", ms.last.Method, ms.last.Path)
	}
}

func TestReady(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"status": "ok", "ml_ready": true})
	})
	c := newTestClient(t, ms.URL)
	h, err := c.Ready(context.Background())
	if err != nil {
		t.Fatalf("Ready: %v", err)
	}
	if h.Status != "ok" || h.Raw["ml_ready"] != true {
		t.Errorf("unexpected ready: %+v", h)
	}
	if ms.last.Method != "GET" || ms.last.Path != "/readyz" {
		t.Errorf("unexpected route: %s %s", ms.last.Method, ms.last.Path)
	}
}
