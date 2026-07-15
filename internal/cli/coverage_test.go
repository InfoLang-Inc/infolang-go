package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// errServer returns status 500 for every request.
func errServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
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
	{"banks", []string{"banks"}},
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
			args := append([]string{inv.args[0], "--api-key", "k", "--base-url", srv.URL, "--timeout", "5s"}, inv.args[1:]...)
			code, _, errOut := runCLI(t, args...)
			if code != 1 || !strings.Contains(errOut, "error:") {
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
	if !strings.Contains(out, "status=ok") {
		t.Errorf("out=%q", out)
	}
}
