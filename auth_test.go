package infolang

import (
	"net/http"
	"testing"
)

func TestAPIKeyAuth(t *testing.T) {
	a := apiKeyAuth{key: "il_live_x"}
	h := http.Header{}
	a.apply(h)
	if h.Get("Authorization") != "Bearer il_live_x" {
		t.Errorf("auth header = %q", h.Get("Authorization"))
	}
	if a.pinnedNamespace() != "" {
		t.Errorf("api key should not pin namespace")
	}
	if a.prefersDirect() {
		t.Errorf("api key should prefer cloud")
	}
}

func TestDevKeyAuth(t *testing.T) {
	a, err := newDevKeyAuth("secret:bank")
	if err != nil {
		t.Fatalf("newDevKeyAuth: %v", err)
	}
	h := http.Header{}
	a.apply(h)
	if h.Get("Authorization") != "Bearer secret" {
		t.Errorf("auth header = %q, want Bearer secret", h.Get("Authorization"))
	}
	if a.pinnedNamespace() != "bank" {
		t.Errorf("namespace = %q, want bank", a.pinnedNamespace())
	}
	if !a.prefersDirect() {
		t.Errorf("dev key should prefer direct")
	}
}

func TestDevKeyAuthNamespaceMayBeEmpty(t *testing.T) {
	a, err := newDevKeyAuth("secret:")
	if err != nil {
		t.Fatalf("newDevKeyAuth: %v", err)
	}
	if a.pinnedNamespace() != "" {
		t.Errorf("namespace = %q, want empty", a.pinnedNamespace())
	}
}

func TestDevKeyAuthErrors(t *testing.T) {
	for _, bad := range []string{"nocolon", ":onlyns", ""} {
		if _, err := newDevKeyAuth(bad); err == nil {
			t.Errorf("newDevKeyAuth(%q) should error", bad)
		}
	}
}
