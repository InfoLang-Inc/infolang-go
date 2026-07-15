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
// last path it saw.
func mockRuntime(t *testing.T, status int, body any) (*httptest.Server, *string) {
	t.Helper()
	var lastPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastPath = r.URL.Path
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

func TestVersionAndHelp(t *testing.T) {
	if code, out, _ := runCLI(t, "version"); code != 0 || strings.TrimSpace(out) == "" {
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
}

func TestRecallHuman(t *testing.T) {
	clearEnv(t)
	srv, lastPath := mockRuntime(t, 200, map[string]any{
		"hits": []map[string]any{{"id": "m1", "text": "alpha", "tags": "a,b", "similarity": 0.9}},
	})
	code, out, errOut := runCLI(t, "recall", "--api-key", "k", "--base-url", srv.URL, "--top-k", "3", "my", "query")
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	if !strings.Contains(out, "1 chunk(s)") || !strings.Contains(out, "alpha") || !strings.Contains(out, "[a,b]") {
		t.Errorf("unexpected output: %q", out)
	}
	if *lastPath != "/v1/recall" {
		t.Errorf("path = %q", *lastPath)
	}
}

func TestRecallWeakLabel(t *testing.T) {
	clearEnv(t)
	srv, _ := mockRuntime(t, 200, map[string]any{
		"hits": []map[string]any{{"id": "m1", "text": "x", "similarity": 0.2}},
	})
	_, out, _ := runCLI(t, "recall", "--api-key", "k", "--base-url", srv.URL, "q")
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
	code, out, _ := runCLI(t, "recall", "--api-key", "k", "--base-url", srv.URL, "--json", "q")
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
	code, out, _ := runCLI(t, "investigate", "--api-key", "k", "--base-url", srv.URL, "q")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "0 chunk(s)") {
		t.Errorf("out=%q", out)
	}
	if *lastPath != "/v1/recall" {
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
	code, _, errOut := runCLI(t, "banks")
	if code != 2 || !strings.Contains(errOut, "credentials") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func TestRemember(t *testing.T) {
	clearEnv(t)
	srv, lastPath := mockRuntime(t, 200, map[string]any{
		"id": "mem-9", "namespace": "docs", "total_memories": 3,
	})
	code, out, errOut := runCLI(t, "remember", "--api-key", "k", "--base-url", srv.URL,
		"--source", "f.md", "--tags", "a,b", "a", "fact")
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	if !strings.Contains(out, "stored mem-9") {
		t.Errorf("out=%q", out)
	}
	if *lastPath != "/v1/remember" {
		t.Errorf("path=%q", *lastPath)
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
	code, out, _ := runCLI(t, "forget", "--api-key", "k", "--base-url", srv.URL, "mem-1")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "forgotten mem-1") {
		t.Errorf("out=%q", out)
	}
	if !strings.HasPrefix(*lastPath, "/v1/memories/") {
		t.Errorf("path=%q", *lastPath)
	}
}

func TestForgetJSON(t *testing.T) {
	clearEnv(t)
	srv, _ := mockRuntime(t, 200, map[string]any{})
	code, out, _ := runCLI(t, "forget", "--api-key", "k", "--base-url", srv.URL, "--json", "mem-1")
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

func TestBanks(t *testing.T) {
	clearEnv(t)
	srv, _ := mockRuntime(t, 200, map[string]any{
		"banks": []map[string]any{{"namespace": "a", "total_memories": 5}},
	})
	code, out, _ := runCLI(t, "banks", "--api-key", "k", "--base-url", srv.URL)
	if code != 0 || !strings.Contains(out, "a") || !strings.Contains(out, "5") {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestBanksEmpty(t *testing.T) {
	clearEnv(t)
	srv, _ := mockRuntime(t, 200, map[string]any{"banks": []any{}})
	code, out, _ := runCLI(t, "banks", "--api-key", "k", "--base-url", srv.URL)
	if code != 0 || !strings.Contains(out, "no banks") {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestBanksJSON(t *testing.T) {
	clearEnv(t)
	srv, _ := mockRuntime(t, 200, map[string]any{
		"banks": []map[string]any{{"namespace": "a", "total_memories": 5}},
	})
	code, out, _ := runCLI(t, "banks", "--api-key", "k", "--base-url", srv.URL, "--json")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	var parsed []map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil || len(parsed) != 1 {
		t.Errorf("json=%q err=%v", out, err)
	}
}

func TestStats(t *testing.T) {
	clearEnv(t)
	srv, _ := mockRuntime(t, 200, map[string]any{"total": 12})
	code, out, _ := runCLI(t, "stats", "--api-key", "k", "--base-url", srv.URL)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil || parsed["total"].(float64) != 12 {
		t.Errorf("json=%q err=%v", out, err)
	}
}

func TestHealth(t *testing.T) {
	clearEnv(t)
	srv, _ := mockRuntime(t, 200, map[string]any{"status": "ok", "model_loaded": true, "engine_ok": true})
	code, out, _ := runCLI(t, "health", "--api-key", "k", "--base-url", srv.URL)
	if code != 0 || !strings.Contains(out, "status=ok") {
		t.Errorf("code=%d out=%q", code, out)
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
	if err := json.Unmarshal([]byte(out), &parsed); err != nil || parsed["status"] != "ok" {
		t.Errorf("json=%q err=%v", out, err)
	}
}

func TestServerErrorExitCode(t *testing.T) {
	clearEnv(t)
	srv, _ := mockRuntime(t, 401, map[string]any{"error": "bad key"})
	code, _, errOut := runCLI(t, "banks", "--api-key", "k", "--base-url", srv.URL)
	if code != 1 || !strings.Contains(errOut, "bad key") {
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
	// dev-key pins namespace; base-url overrides the direct default.
	code, _, errOut := runCLI(t, "recall", "--dev-key", "secret:bank", "--base-url", srv.URL, "q")
	if code != 0 {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func TestInvalidDevKey(t *testing.T) {
	clearEnv(t)
	code, _, errOut := runCLI(t, "banks", "--dev-key", "nocolon", "--base-url", "http://x")
	if code != 2 || !strings.Contains(errOut, "dev key") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}
