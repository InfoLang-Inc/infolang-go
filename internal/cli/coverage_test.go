package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// errServer returns status 400 for every request. 400 is non-retryable, so the
// per-command error matrix stays fast (no backoff sleeps).
func errServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":{"code":"bad_request","message":"boom"}}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// invocation is a command plus the trailing positional args it needs.
var commandInvocations = []struct {
	name string
	args []string // command + positionals (flags injected by the test)
}{
	{"recall", []string{"recall", "q"}},
	{"investigate", []string{"investigate", "q"}},
	{"remember", []string{"remember", "text"}},
	{"forget", []string{"forget", "id"}},
	{"namespaces", []string{"namespaces"}},
	{"whoami", []string{"whoami"}},
	{"stats", []string{"stats"}},
	{"health", []string{"health"}},
}

func TestNewClientErrorPerCommand(t *testing.T) {
	for _, inv := range commandInvocations {
		t.Run(inv.name, func(t *testing.T) {
			clearEnv(t)
			args := append([]string{inv.args[0], "--dev-key", "nocolon", "--base-url", "http://x"}, inv.args[1:]...)
			code, _, errOut := runCLI(t, args...)
			if code != 2 || !strings.Contains(errOut, "dev key") {
				t.Errorf("code=%d err=%q", code, errOut)
			}
		})
	}
}

func TestRequestErrorPerCommand(t *testing.T) {
	for _, inv := range commandInvocations {
		t.Run(inv.name, func(t *testing.T) {
			clearEnv(t)
			srv := errServer(t)
			args := append([]string{inv.args[0], "--api-key", "k", "--base-url", srv.URL,
				"--workspace", "ws", "--timeout", "5s"}, inv.args[1:]...)
			code, _, errOut := runCLI(t, args...)
			if code != 1 || !strings.Contains(errOut, "error:") {
				t.Errorf("code=%d err=%q", code, errOut)
			}
		})
	}
}

// Without an explicit workspace, workspace-scoped commands surface the lazy
// whoami resolution failure as a runtime error (exit 1), never a panic.
func TestWorkspaceResolutionErrorPerCommand(t *testing.T) {
	for _, inv := range []string{"recall", "remember", "forget", "namespaces", "stats"} {
		t.Run(inv, func(t *testing.T) {
			clearEnv(t)
			// whoami answers with zero grants for every path.
			srv, _ := mockRuntime(t, 200, map[string]any{
				"lane": "api_key", "principal": "p", "workspaces": []any{},
			})
			args := []string{inv, "--api-key", "k", "--base-url", srv.URL}
			switch inv {
			case "recall":
				args = append(args, "q")
			case "remember":
				args = append(args, "text")
			case "forget":
				args = append(args, "id")
			}
			code, _, errOut := runCLI(t, args...)
			if code != 1 || !strings.Contains(errOut, "no visible workspace grants") {
				t.Errorf("code=%d err=%q", code, errOut)
			}
		})
	}
}

func TestBadFlagPerCommand(t *testing.T) {
	for _, inv := range commandInvocations {
		t.Run(inv.name, func(t *testing.T) {
			clearEnv(t)
			code, _, _ := runCLI(t, inv.args[0], "--nope")
			if code != 2 {
				t.Errorf("code=%d, want 2", code)
			}
		})
	}
}

func TestClientScopingOptions(t *testing.T) {
	clearEnv(t)
	srv, _ := mockRuntime(t, 200, map[string]any{"status": "ok"})
	code, out, errOut := runCLI(t, "health",
		"--api-key", "k", "--base-url", srv.URL,
		"--namespace", "ns", "--workspace", "ws")
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	if !strings.Contains(out, "healthz=ok") {
		t.Errorf("out=%q", out)
	}
}
