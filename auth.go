package infolang

import (
	"net/http"
	"strings"
)

// authProvider supplies the Authorization header for each request.
type authProvider interface {
	apply(h http.Header)
	// pinnedNamespace returns a namespace the credential is bound to, or "".
	pinnedNamespace() string
	// prefersDirect reports whether the credential targets a self-hosted
	// runtime (used to pick the default base URL).
	prefersDirect() bool
}

// apiKeyAuth is bearer authentication with a managed-cloud API key
// (il_live_...). It honors the client namespace on both reads and writes.
type apiKeyAuth struct {
	key string
}

func (a apiKeyAuth) apply(h http.Header)     { h.Set("Authorization", "Bearer "+a.key) }
func (a apiKeyAuth) pinnedNamespace() string { return "" }
func (a apiKeyAuth) prefersDirect() bool     { return false }

// devKeyAuth is a self-hosted dev key in "key:namespace" form. The namespace is
// pinned by the credential and used as the default bank.
type devKeyAuth struct {
	key       string
	namespace string
}

func newDevKeyAuth(raw string) (devKeyAuth, error) {
	key, ns, ok := strings.Cut(raw, ":")
	if !ok || key == "" {
		return devKeyAuth{}, &ConfigError{Message: "dev key must be in 'key:namespace' form"}
	}
	return devKeyAuth{key: key, namespace: ns}, nil
}

func (a devKeyAuth) apply(h http.Header)     { h.Set("Authorization", "Bearer "+a.key) }
func (a devKeyAuth) pinnedNamespace() string { return a.namespace }
func (a devKeyAuth) prefersDirect() bool     { return true }
