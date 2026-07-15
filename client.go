package infolang

import (
	"net/http"
	"os"
	"strings"
	"time"
)

// Base URLs for the managed cloud edge and a self-hosted runtime.
const (
	CloudBaseURL  = "https://api.infolang.ai"
	DirectBaseURL = "http://127.0.0.1:8766"
)

// Client is a typed client over the il-runtime REST API. Construct it with New
// and reuse it; it is safe for concurrent use.
type Client struct {
	// Namespace is the default memory bank applied to reads and writes when a
	// per-call namespace is not given.
	Namespace string
	// Workspace is the account/tenant sent as X-InfoLang-Workspace-Id.
	Workspace string
	// BaseURL is the resolved runtime endpoint.
	BaseURL string

	t *transport
}

// config accumulates option state before the client is built.
type config struct {
	apiKey     string
	devKey     string
	baseURL    string
	namespace  string
	workspace  string
	httpClient *http.Client
	userAgent  string
	maxRetries int
	timeout    time.Duration
}

// Option customizes a Client at construction time.
type Option func(*config)

// WithDevKey authenticates against a self-hosted runtime with a dev key in
// "key:namespace" form. The namespace is pinned by the credential.
func WithDevKey(devKey string) Option {
	return func(c *config) { c.devKey = devKey }
}

// WithBaseURL overrides the runtime endpoint (otherwise chosen from the
// credential type or INFOLANG_BASE_URL).
func WithBaseURL(url string) Option {
	return func(c *config) { c.baseURL = url }
}

// WithNamespace sets the default memory bank.
func WithNamespace(ns string) Option {
	return func(c *config) { c.namespace = ns }
}

// WithWorkspace sets the account/tenant (X-InfoLang-Workspace-Id header).
func WithWorkspace(ws string) Option {
	return func(c *config) { c.workspace = ws }
}

// WithHTTPClient supplies a custom *http.Client (proxies, custom TLS, mocks).
func WithHTTPClient(hc *http.Client) Option {
	return func(c *config) { c.httpClient = hc }
}

// WithUserAgent overrides the default User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *config) { c.userAgent = ua }
}

// WithMaxRetries sets how many times a 429/5xx/transport error is retried.
func WithMaxRetries(n int) Option {
	return func(c *config) { c.maxRetries = n }
}

// WithTimeout sets the per-request timeout on the default HTTP client. Ignored
// when WithHTTPClient is also supplied.
func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// New constructs a Client. The API key may be empty, in which case credentials
// are resolved from options or the environment (INFOLANG_API_KEY, then
// INFOLANG_DEV_KEY). It returns a *ConfigError when no credential is available.
func New(apiKey string, opts ...Option) (*Client, error) {
	cfg := &config{
		apiKey:     apiKey,
		maxRetries: 2,
		timeout:    30 * time.Second,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	auth, err := resolveAuth(cfg)
	if err != nil {
		return nil, err
	}

	baseURL := resolveBaseURL(cfg, auth)
	namespace := resolveNamespace(cfg, auth)
	workspace := resolveWorkspace(cfg)

	httpClient := cfg.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.timeout}
	}
	userAgent := cfg.userAgent
	if userAgent == "" {
		userAgent = "infolang-go/" + Version
	}

	return &Client{
		Namespace: namespace,
		Workspace: workspace,
		BaseURL:   baseURL,
		t: &transport{
			baseURL:     strings.TrimRight(baseURL, "/"),
			httpClient:  httpClient,
			auth:        auth,
			userAgent:   userAgent,
			workspaceID: workspace,
			maxRetries:  cfg.maxRetries,
			backoffBase: 500 * time.Millisecond,
			backoffCap:  8 * time.Second,
			sleep:       sleepCtx,
			rng:         defaultRNG,
		},
	}, nil
}

func resolveAuth(cfg *config) (authProvider, error) {
	if cfg.apiKey != "" {
		return apiKeyAuth{key: cfg.apiKey}, nil
	}
	if cfg.devKey != "" {
		return newDevKeyAuth(cfg.devKey)
	}
	if v := os.Getenv("INFOLANG_API_KEY"); v != "" {
		return apiKeyAuth{key: v}, nil
	}
	if v := os.Getenv("INFOLANG_DEV_KEY"); v != "" {
		return newDevKeyAuth(v)
	}
	return nil, &ConfigError{
		Message: "no credentials: pass an api key, WithDevKey, or set INFOLANG_API_KEY",
	}
}

func resolveBaseURL(cfg *config, auth authProvider) string {
	if cfg.baseURL != "" {
		return cfg.baseURL
	}
	if v := os.Getenv("INFOLANG_BASE_URL"); v != "" {
		return v
	}
	if auth.prefersDirect() {
		return DirectBaseURL
	}
	return CloudBaseURL
}

func resolveNamespace(cfg *config, auth authProvider) string {
	if cfg.namespace != "" {
		return cfg.namespace
	}
	if ns := auth.pinnedNamespace(); ns != "" {
		return ns
	}
	return os.Getenv("INFOLANG_NAMESPACE")
}

func resolveWorkspace(cfg *config) string {
	if cfg.workspace != "" {
		return cfg.workspace
	}
	if v := os.Getenv("INFOLANG_WORKSPACE"); v != "" {
		return v
	}
	return os.Getenv("INFOLANG_WORKSPACE_ID")
}
