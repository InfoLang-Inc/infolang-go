package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// runCLI executes Run with the given args against a fresh in-memory buffer pair.
func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code = Run(context.Background(), args, &out, &errBuf)
	return code, out.String(), errBuf.String()
}

// mockRuntime starts a server returning body for any request and records the
// last path (with query) it saw.
func mockRuntime(t *testing.T, status int, body any) (*httptest.Server, *string) {
	t.Helper()
	var lastPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastPath = r.URL.Path
		if r.URL.RawQuery != "" {
			lastPath += "?" + r.URL.RawQuery
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &lastPath
}

// clearEnv blanks InfoLang env vars so credentials come only from flags.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"INFOLANG_API_KEY", "INFOLANG_DEV_KEY", "INFOLANG_BASE_URL",
		"INFOLANG_NAMESPACE", "INFOLANG_WORKSPACE", "INFOLANG_WORKSPACE_ID",
	} {
		t.Setenv(k, "")
	}
}

// scoped injects the credential/workspace flags every /v2 command needs right
// after the command name (flag parsing stops at the first positional).
func scoped(srvURL string, args ...string) []string {
	out := []string{args[0], "--api-key", "k", "--base-url", srvURL, "--workspace", "ws"}
	return append(out, args[1:]...)
}

func TestVersionAndHelp(t *testing.T) {
	if code, out, _ := runCLI(t, "version"); code != 0 || strings.TrimSpace(out) != "0.3.0" {
		t.Errorf("version: code=%d out=%q", code, out)
	}
	if code, out, _ := runCLI(t, "help"); code != 0 || !strings.Contains(out, "Usage:") {
		t.Errorf("help: code=%d", code)
	}
	if code, _, errOut := runCLI(t); code != 2 || !strings.Contains(errOut, "Usage:") {
		t.Errorf("no args: code=%d err=%q", code, errOut)
	}
	if code, _, errOut := runCLI(t, "frobnicate"); code != 2 || !strings.Contains(errOut, "unknown command") {
		t.Errorf("unknown: code=%d err=%q", code, errOut)
	}
	// The retired banks command must no longer exist.
	if code, _, errOut := runCLI(t, "banks"); code != 2 || !strings.Contains(errOut, "unknown command") {
		t.Errorf("banks should be retired: code=%d err=%q", code, errOut)
	}
}

func TestRecallHuman(t *testing.T) {
	clearEnv(t)
	srv, lastPath := mockRuntime(t, 200, map[string]any{
		"hits": []map[string]any{{"id": "m1", "text": "alpha", "tags": "a,b", "similarity": 0.9}},
	})
	code, out, errOut := runCLI(t, scoped(srv.URL, "recall", "--top-k", "3", "my", "query")...)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	if !strings.Contains(out, "1 chunk(s)") || !strings.Contains(out, "alpha") || !strings.Contains(out, "[a,b]") {
		t.Errorf("unexpected output: %q", out)
	}
	if *lastPath != "/v2/workspaces/ws/recall" {
		t.Errorf("path = %q", *lastPath)
	}
}

func TestRecallWeakLabel(t *testing.T) {
	clearEnv(t)
	srv, _ := mockRuntime(t, 200, map[string]any{
		"hits": []map[string]any{{"id": "m1", "text": "x", "similarity": 0.2}},
	})
	_, out, _ := runCLI(t, scoped(srv.URL, "recall", "q")...)
	if !strings.Contains(out, "weak match") {
		t.Errorf("expected weak label, got %q", out)
	}
}

func TestRecallJSON(t *testing.T) {
	clearEnv(t)
	srv, _ := mockRuntime(t, 200, map[string]any{
		"namespace": "ns",
		"hits":      []map[string]any{{"id": "m1", "text": "alpha", "similarity": 0.9}},
	})
	code, out, _ := runCLI(t, scoped(srv.URL, "recall", "--json", "q")...)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if parsed["namespace"] != "ns" {
		t.Errorf("namespace = %v", parsed["namespace"])
	}
}

func TestInvestigate(t *testing.T) {
	clearEnv(t)
	srv, lastPath := mockRuntime(t, 200, map[string]any{"hits": []any{}})
	code, out, _ := runCLI(t, scoped(srv.URL, "investigate", "q")...)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "0 chunk(s)") {
		t.Errorf("out=%q", out)
	}
	if *lastPath != "/v2/workspaces/ws/recall" {
		t.Errorf("path=%q", *lastPath)
	}
}

func TestRecallMissingQuery(t *testing.T) {
	clearEnv(t)
	code, _, errOut := runCLI(t, "recall", "--api-key", "k", "--base-url", "http://x")
	if code != 2 || !strings.Contains(errOut, "query") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func TestNoCredentials(t *testing.T) {
	clearEnv(t)
	code, _, errOut := runCLI(t, "namespaces")
	if code != 2 || !strings.Contains(errOut, "credentials") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

// A command without --workspace resolves the workspace via /v2/whoami once.
func TestWorkspaceAutoResolutionFromWhoami(t *testing.T) {
	clearEnv(t)
	var whoamiCalls int
	var recallPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v2/whoami" {
			whoamiCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"lane": "api_key", "principal": "p",
				"workspaces": []map[string]any{{"workspace_id": "ws-auto", "role": "admin"}},
			})
			return
		}
		recallPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{"hits": []any{}})
	}))
	t.Cleanup(srv.Close)
	code, _, errOut := runCLI(t, "recall", "--api-key", "k", "--base-url", srv.URL, "q")
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	if whoamiCalls != 1 || recallPath != "/v2/workspaces/ws-auto/recall" {
		t.Errorf("whoami=%d path=%q", whoamiCalls, recallPath)
	}
}

func TestRememberSplitsTags(t *testing.T) {
	clearEnv(t)
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "mem-9", "namespace": "docs", "stored": true, "total_memories": 3,
		})
	}))
	t.Cleanup(srv.Close)
	code, out, errOut := runCLI(t, scoped(srv.URL, "remember", "--source", "f.md", "--tags", "a, b", "a", "fact")...)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	if !strings.Contains(out, "stored mem-9") {
		t.Errorf("out=%q", out)
	}
	tags, ok := body["tags"].([]any)
	if !ok || len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
		t.Errorf("--tags a,b should be a JSON array, got %#v", body["tags"])
	}
}

func TestRememberDeduplicated(t *testing.T) {
	clearEnv(t)
	srv, _ := mockRuntime(t, 200, map[string]any{
		"id": "mem-old", "namespace": "docs", "stored": false,
		"deduplicated": true, "deduped_against": "mem-old",
	})
	code, out, _ := runCLI(t, scoped(srv.URL, "remember", "same", "fact")...)
	if code != 0 || !strings.Contains(out, "deduplicated against mem-old") {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestRememberMissingText(t *testing.T) {
	clearEnv(t)
	code, _, errOut := runCLI(t, "remember", "--api-key", "k", "--base-url", "http://x")
	if code != 2 || !strings.Contains(errOut, "text") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func TestForget(t *testing.T) {
	clearEnv(t)
	srv, lastPath := mockRuntime(t, 200, map[string]any{})
	code, out, _ := runCLI(t, scoped(srv.URL, "forget", "mem-1")...)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "forgotten mem-1") {
		t.Errorf("out=%q", out)
	}
	if *lastPath != "/v2/workspaces/ws/memories/mem-1" {
		t.Errorf("path=%q", *lastPath)
	}
}

func TestForgetWithNamespace(t *testing.T) {
	clearEnv(t)
	srv, lastPath := mockRuntime(t, 200, map[string]any{})
	code, _, errOut := runCLI(t, scoped(srv.URL, "forget", "--namespace", "docs", "mem-1")...)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	if *lastPath != "/v2/workspaces/ws/memories/mem-1?namespace=docs" {
		t.Errorf("path=%q, want namespace query", *lastPath)
	}
}

func TestForgetJSON(t *testing.T) {
	clearEnv(t)
	srv, _ := mockRuntime(t, 200, map[string]any{})
	code, out, _ := runCLI(t, scoped(srv.URL, "forget", "--json", "mem-1")...)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil || parsed["forgotten"] != "mem-1" {
		t.Errorf("json=%q err=%v", out, err)
	}
}

func TestForgetMissingID(t *testing.T) {
	clearEnv(t)
	code, _, errOut := runCLI(t, "forget", "--api-key", "k", "--base-url", "http://x")
	if code != 2 || !strings.Contains(errOut, "memory id") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func TestNamespaces(t *testing.T) {
	clearEnv(t)
	srv, lastPath := mockRuntime(t, 200, map[string]any{
		"namespaces": []any{
			map[string]any{"namespace": "docs", "memories": 12, "chunks": 40},
			"bare",
		},
	})
	code, out, _ := runCLI(t, scoped(srv.URL, "namespaces")...)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if *lastPath != "/v2/workspaces/ws/namespaces" {
		t.Errorf("path=%q", *lastPath)
	}
	if !strings.Contains(out, "docs") || !strings.Contains(out, "12") || !strings.Contains(out, "40") {
		t.Errorf("counts missing: %q", out)
	}
	if !strings.Contains(out, "bare") {
		t.Errorf("bare string namespace missing: %q", out)
	}
}

func TestNamespacesEmpty(t *testing.T) {
	clearEnv(t)
	srv, _ := mockRuntime(t, 200, map[string]any{"namespaces": []any{}})
	code, out, _ := runCLI(t, scoped(srv.URL, "namespaces")...)
	if code != 0 || !strings.Contains(out, "no namespaces") {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestNamespacesJSON(t *testing.T) {
	clearEnv(t)
	srv, _ := mockRuntime(t, 200, map[string]any{
		"namespaces": []any{map[string]any{"namespace": "a", "memories": 5, "chunks": 9}},
	})
	code, out, _ := runCLI(t, scoped(srv.URL, "namespaces", "--json")...)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	var parsed []map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil || len(parsed) != 1 || parsed[0]["namespace"] != "a" {
		t.Errorf("json=%q err=%v", out, err)
	}
}

func TestWhoami(t *testing.T) {
	clearEnv(t)
	srv, lastPath := mockRuntime(t, 200, map[string]any{
		"lane": "api_key", "principal": "user_7",
		"workspaces": []map[string]any{
			{"workspace_id": "ws-1", "role": "admin"},
			{"workspace_id": "ws-2", "role": "viewer"},
		},
	})
	code, out, _ := runCLI(t, "whoami", "--api-key", "k", "--base-url", srv.URL)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if *lastPath != "/v2/whoami" {
		t.Errorf("path=%q", *lastPath)
	}
	if !strings.Contains(out, "lane=api_key") || !strings.Contains(out, "principal=user_7") {
		t.Errorf("identity missing: %q", out)
	}
	if !strings.Contains(out, "ws-1") || !strings.Contains(out, "role=viewer") {
		t.Errorf("workspaces missing: %q", out)
	}
}

func TestWhoamiNullPrincipalAndNoGrants(t *testing.T) {
	clearEnv(t)
	srv, _ := mockRuntime(t, 200, map[string]any{
		"lane": "dev", "principal": nil, "workspaces": []any{},
	})
	code, out, _ := runCLI(t, "whoami", "--api-key", "k", "--base-url", srv.URL)
	if code != 0 || !strings.Contains(out, "principal=-") || !strings.Contains(out, "no workspace grants") {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestWhoamiJSON(t *testing.T) {
	clearEnv(t)
	srv, _ := mockRuntime(t, 200, map[string]any{
		"lane": "api_key", "principal": "p", "workspaces": []any{}, "extra": "kept",
	})
	code, out, _ := runCLI(t, "whoami", "--api-key", "k", "--base-url", srv.URL, "--json")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil || parsed["extra"] != "kept" {
		t.Errorf("raw body should pass through: %q err=%v", out, err)
	}
}

func TestStats(t *testing.T) {
	clearEnv(t)
	srv, lastPath := mockRuntime(t, 200, map[string]any{
		"ok": true, "payload": map[string]any{"total": 12},
	})
	code, out, _ := runCLI(t, scoped(srv.URL, "stats")...)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if *lastPath != "/v2/workspaces/ws/stats" {
		t.Errorf("path=%q", *lastPath)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil || parsed["ok"] != true {
		t.Errorf("json=%q err=%v", out, err)
	}
}

func TestHealth(t *testing.T) {
	clearEnv(t)
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}))
	t.Cleanup(srv.Close)
	code, out, _ := runCLI(t, "health", "--api-key", "k", "--base-url", srv.URL)
	if code != 0 || !strings.Contains(out, "healthz=ok readyz=ok") {
		t.Errorf("code=%d out=%q", code, out)
	}
	if len(paths) != 2 || paths[0] != "/healthz" || paths[1] != "/readyz" {
		t.Errorf("paths=%v, want /healthz then /readyz", paths)
	}
}

func TestHealthJSON(t *testing.T) {
	clearEnv(t)
	srv, _ := mockRuntime(t, 200, map[string]any{"status": "ok"})
	code, out, _ := runCLI(t, "health", "--api-key", "k", "--base-url", srv.URL, "--json")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json=%q err=%v", out, err)
	}
	if _, ok := parsed["healthz"]; !ok {
		t.Errorf("healthz missing: %v", parsed)
	}
	if _, ok := parsed["readyz"]; !ok {
		t.Errorf("readyz missing: %v", parsed)
	}
}

func TestServerErrorExitCode(t *testing.T) {
	clearEnv(t)
	srv, _ := mockRuntime(t, 401, map[string]any{"error": map[string]any{"code": "bad_key", "message": "bad key"}})
	code, _, errOut := runCLI(t, scoped(srv.URL, "namespaces")...)
	if code != 1 || !strings.Contains(errOut, "bad_key: bad key") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func TestBadFlag(t *testing.T) {
	clearEnv(t)
	code, _, _ := runCLI(t, "recall", "--nope")
	if code != 2 {
		t.Errorf("bad flag code=%d, want 2", code)
	}
}

func TestDevKeyFlag(t *testing.T) {
	clearEnv(t)
	srv, _ := mockRuntime(t, 200, map[string]any{"hits": []any{}})
	// dev-key pins namespace; still functional against the gateway.
	code, _, errOut := runCLI(t, "recall", "--dev-key", "secret:bank", "--base-url", srv.URL, "--workspace", "ws", "q")
	if code != 0 {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func TestInvalidDevKey(t *testing.T) {
	clearEnv(t)
	code, _, errOut := runCLI(t, "namespaces", "--dev-key", "nocolon", "--base-url", "http://x")
	if code != 2 || !strings.Contains(errOut, "dev key") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}
