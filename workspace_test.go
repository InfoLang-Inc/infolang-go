package infolang

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// whoamiServer answers /v2/whoami with the given workspace grants and any
// workspace-scoped recall with an empty hit list, counting whoami calls.
func whoamiServer(t *testing.T, grants []map[string]any) (*mockServer, *atomic.Int32) {
	t.Helper()
	var whoamiCalls atomic.Int32
	ms := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/whoami" {
			whoamiCalls.Add(1)
			writeJSON(w, 200, map[string]any{
				"lane": "api_key", "principal": "user_1", "workspaces": grants,
			})
			return
		}
		writeJSON(w, 200, map[string]any{"hits": []any{}})
	})
	return ms, &whoamiCalls
}

func TestWhoamiParses(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"lane":      "api_key",
			"principal": nil,
			"workspaces": []map[string]any{
				{"workspace_id": "ws-1", "role": "admin", "scopes": []string{"memory:read"}},
			},
			"scopes":       []string{"memory:read"},
			"extra_future": "field",
		})
	})
	c := newTestClient(t, ms.URL)
	who, err := c.Whoami(context.Background())
	if err != nil {
		t.Fatalf("Whoami: %v", err)
	}
	if ms.last.Method != "GET" || ms.last.Path != "/v2/whoami" {
		t.Errorf("route = %s %s", ms.last.Method, ms.last.Path)
	}
	if who.Lane != "api_key" || who.Principal != "" {
		t.Errorf("lane/principal = %q/%q", who.Lane, who.Principal)
	}
	if len(who.Workspaces) != 1 || who.Workspaces[0].WorkspaceID != "ws-1" || who.Workspaces[0].Role != "admin" {
		t.Errorf("workspaces = %+v", who.Workspaces)
	}
	if who.Raw["extra_future"] != "field" {
		t.Errorf("raw body not preserved: %v", who.Raw)
	}
}

func TestWorkspaceExplicitSkipsWhoami(t *testing.T) {
	var whoamiCalls atomic.Int32
	ms := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/whoami" {
			whoamiCalls.Add(1)
		}
		writeJSON(w, 200, map[string]any{"hits": []any{}})
	})
	c := newTestClient(t, ms.URL, WithWorkspace("ws-explicit"))
	if _, err := c.Recall(context.Background(), "q", nil); err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if whoamiCalls.Load() != 0 {
		t.Errorf("whoami called %d times for explicit workspace", whoamiCalls.Load())
	}
	if ms.last.Path != "/v2/workspaces/ws-explicit/recall" {
		t.Errorf("path = %q", ms.last.Path)
	}
}

func TestWorkspaceFromEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("INFOLANG_WORKSPACE", "ws-env")
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"hits": []any{}})
	})
	c, err := New("k", WithBaseURL(ms.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.Recall(context.Background(), "q", nil); err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if ms.last.Path != "/v2/workspaces/ws-env/recall" {
		t.Errorf("path = %q", ms.last.Path)
	}
}

func TestWorkspaceSingleGrantResolvedOnceAndCached(t *testing.T) {
	ms, whoamiCalls := whoamiServer(t, []map[string]any{
		{"workspace_id": "ws-auto", "role": "admin"},
	})
	c := newUnscopedTestClient(t, ms.URL)
	// Two calls: whoami must run exactly once.
	for i := 0; i < 2; i++ {
		if _, err := c.Recall(context.Background(), "q", nil); err != nil {
			t.Fatalf("Recall %d: %v", i, err)
		}
	}
	if whoamiCalls.Load() != 1 {
		t.Errorf("whoami calls = %d, want 1", whoamiCalls.Load())
	}
	if ms.last.Path != "/v2/workspaces/ws-auto/recall" {
		t.Errorf("path = %q", ms.last.Path)
	}
}

func TestWorkspaceZeroGrantsConfigError(t *testing.T) {
	ms, _ := whoamiServer(t, []map[string]any{})
	c := newUnscopedTestClient(t, ms.URL)
	_, err := c.Recall(context.Background(), "q", nil)
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("want ConfigError, got %v", err)
	}
	if !strings.Contains(cfgErr.Message, "no visible workspace grants") {
		t.Errorf("message = %q", cfgErr.Message)
	}
}

func TestWorkspaceMultiGrantErrorNamesIDs(t *testing.T) {
	ms, whoamiCalls := whoamiServer(t, []map[string]any{
		{"workspace_id": "ws-a", "role": "admin"},
		{"workspace_id": "ws-b", "role": "viewer"},
	})
	c := newUnscopedTestClient(t, ms.URL)
	_, err := c.Recall(context.Background(), "q", nil)
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("want ConfigError, got %v", err)
	}
	if !strings.Contains(cfgErr.Message, "ws-a, ws-b") || !strings.Contains(cfgErr.Message, "2 workspaces") {
		t.Errorf("message should name the ids: %q", cfgErr.Message)
	}
	// Failed resolution must not be cached — a retry re-asks the gateway.
	_, _ = c.Recall(context.Background(), "q", nil)
	if whoamiCalls.Load() != 2 {
		t.Errorf("whoami calls = %d, want 2 (failure not cached)", whoamiCalls.Load())
	}
}

func TestWorkspaceFailedWhoamiNotCached(t *testing.T) {
	var calls atomic.Int32
	ms := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/whoami" {
			if calls.Add(1) == 1 {
				writeJSON(w, 401, map[string]any{"error": map[string]any{"code": "bad_key", "message": "nope"}})
				return
			}
			writeJSON(w, 200, map[string]any{
				"lane": "api_key", "principal": "p",
				"workspaces": []map[string]any{{"workspace_id": "ws-later", "role": "admin"}},
			})
			return
		}
		writeJSON(w, 200, map[string]any{"hits": []any{}})
	})
	c := newUnscopedTestClient(t, ms.URL, WithMaxRetries(0))
	if _, err := c.Recall(context.Background(), "q", nil); err == nil {
		t.Fatal("first recall should fail")
	}
	// Second attempt must retry whoami and succeed.
	if _, err := c.Recall(context.Background(), "q", nil); err != nil {
		t.Fatalf("second recall: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("whoami calls = %d, want 2", calls.Load())
	}
	if ms.last.Path != "/v2/workspaces/ws-later/recall" {
		t.Errorf("path = %q", ms.last.Path)
	}
}

func TestWorkspaceIDEscapedInPath(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"hits": []any{}})
	})
	c := newTestClient(t, ms.URL, WithWorkspace("ws id/odd"))
	if _, err := c.Recall(context.Background(), "q", nil); err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if ms.last.EscapedPath != "/v2/workspaces/ws%20id%2Fodd/recall" {
		t.Errorf("escaped path = %q", ms.last.EscapedPath)
	}
}

func TestWorkspaceIDDirect(t *testing.T) {
	ms, whoamiCalls := whoamiServer(t, []map[string]any{
		{"workspace_id": "ws-only", "role": "admin"},
	})
	c := newUnscopedTestClient(t, ms.URL)
	ws, err := c.WorkspaceID(context.Background())
	if err != nil || ws != "ws-only" {
		t.Fatalf("WorkspaceID = %q, %v", ws, err)
	}
	// Cached on the second direct call too.
	if _, err := c.WorkspaceID(context.Background()); err != nil {
		t.Fatalf("WorkspaceID: %v", err)
	}
	if whoamiCalls.Load() != 1 {
		t.Errorf("whoami calls = %d, want 1", whoamiCalls.Load())
	}
}

func TestWorkspaceGrantsWithEmptyIDsIgnored(t *testing.T) {
	ms, _ := whoamiServer(t, []map[string]any{
		{"workspace_id": "", "role": "ghost"},
		{"workspace_id": "ws-real", "role": "admin"},
	})
	c := newUnscopedTestClient(t, ms.URL)
	ws, err := c.WorkspaceID(context.Background())
	if err != nil || ws != "ws-real" {
		t.Fatalf("WorkspaceID = %q, %v", ws, err)
	}
}
