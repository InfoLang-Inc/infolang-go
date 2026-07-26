package infolang

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

// clearEnv blanks every InfoLang env var so construction tests are hermetic.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"INFOLANG_API_KEY", "INFOLANG_DEV_KEY", "INFOLANG_BASE_URL",
		"INFOLANG_NAMESPACE", "INFOLANG_WORKSPACE", "INFOLANG_WORKSPACE_ID",
	} {
		t.Setenv(k, "")
	}
}

func TestNewFromAPIKey(t *testing.T) {
	clearEnv(t)
	c, err := New("il_live_abc")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.BaseURL != CloudBaseURL {
		t.Errorf("baseURL = %q, want cloud", c.BaseURL)
	}
	if c.Namespace != "" {
		t.Errorf("namespace = %q, want empty", c.Namespace)
	}
}

// The direct endpoint is gone: a dev key still pins its namespace but the base
// URL now always defaults to the cloud gateway.
func TestNewDevKeyDefaultsCloudAndPinsNamespace(t *testing.T) {
	clearEnv(t)
	c, err := New("", WithDevKey("secret:mybank"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.BaseURL != CloudBaseURL {
		t.Errorf("baseURL = %q, want cloud (direct default removed)", c.BaseURL)
	}
	if c.Namespace != "mybank" {
		t.Errorf("namespace = %q, want mybank", c.Namespace)
	}
}

func TestNewDevKeyInvalid(t *testing.T) {
	clearEnv(t)
	_, err := New("", WithDevKey("nocolon"))
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("want ConfigError, got %v", err)
	}
}

func TestNewNoCredentials(t *testing.T) {
	clearEnv(t)
	_, err := New("")
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("want ConfigError, got %v", err)
	}
}

func TestNewResolvesFromEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("INFOLANG_API_KEY", "il_live_env")
	t.Setenv("INFOLANG_NAMESPACE", "envns")
	t.Setenv("INFOLANG_WORKSPACE", "ws-1")
	t.Setenv("INFOLANG_BASE_URL", "https://example.test")
	c, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.BaseURL != "https://example.test" {
		t.Errorf("baseURL = %q", c.BaseURL)
	}
	if c.Namespace != "envns" || c.Workspace != "ws-1" {
		t.Errorf("ns/ws = %q/%q", c.Namespace, c.Workspace)
	}
}

func TestNewDevKeyFromEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("INFOLANG_DEV_KEY", "s:devns")
	c, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.Namespace != "devns" || c.BaseURL != CloudBaseURL {
		t.Errorf("unexpected: ns=%q base=%q", c.Namespace, c.BaseURL)
	}
}

func TestNewWorkspaceIDEnvFallback(t *testing.T) {
	clearEnv(t)
	t.Setenv("INFOLANG_API_KEY", "k")
	t.Setenv("INFOLANG_WORKSPACE_ID", "ws-id")
	c, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.Workspace != "ws-id" {
		t.Errorf("workspace = %q, want ws-id", c.Workspace)
	}
}

func TestOptionsOverrideEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("INFOLANG_NAMESPACE", "envns")
	c, err := New("k", WithNamespace("optns"), WithWorkspace("optws"), WithBaseURL("http://x"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.Namespace != "optns" || c.Workspace != "optws" || c.BaseURL != "http://x" {
		t.Errorf("options did not override: ns=%q ws=%q base=%q", c.Namespace, c.Workspace, c.BaseURL)
	}
}

// The workspace rides the /v2 path — never a header.
func TestWorkspaceScopedByPathNotHeader(t *testing.T) {
	clearEnv(t)
	ms := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"hits": []any{}})
	})
	c := newTestClient(t, ms.URL, WithWorkspace("ws-99"))
	if _, err := c.Recall(context.Background(), "q", nil); err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if ms.last.Path != "/v2/workspaces/ws-99/recall" {
		t.Errorf("path = %q, want /v2/workspaces/ws-99/recall", ms.last.Path)
	}
	if got := ms.last.Header.Get("X-InfoLang-Workspace-Id"); got != "" {
		t.Errorf("legacy workspace header still sent: %q", got)
	}
	if got := ms.last.Header.Get("Authorization"); got != "Bearer il_live_test" {
		t.Errorf("auth header = %q", got)
	}
	if got := ms.last.Header.Get("User-Agent"); got != "infolang-go/"+Version {
		t.Errorf("user-agent = %q", got)
	}
}

func TestCustomHTTPClientAndUserAgent(t *testing.T) {
	clearEnv(t)
	ms := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"status": "ok"})
	})
	hc := &http.Client{Timeout: 2 * time.Second}
	c, err := New("k", WithBaseURL(ms.URL), WithHTTPClient(hc), WithUserAgent("custom/1"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if got := ms.last.Header.Get("User-Agent"); got != "custom/1" {
		t.Errorf("user-agent = %q", got)
	}
}

func TestVersionIs030(t *testing.T) {
	if Version != "0.3.0" {
		t.Errorf("Version = %q, want 0.3.0", Version)
	}
}
