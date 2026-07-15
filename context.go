package infolang

import (
	"context"
	"net/url"
)

// ContextPack builds a token-budgeted context string for a query.
func (c *Client) ContextPack(ctx context.Context, query string, opts *ContextPackOptions) (*ContextPack, error) {
	if opts == nil {
		opts = &ContextPackOptions{}
	}
	ns := opts.Namespace
	if ns == "" {
		ns = c.Namespace
	}
	body := map[string]any{"query": query}
	if ns != "" {
		body["namespace"] = ns
	}
	if opts.MaxTokens > 0 {
		body["max_tokens"] = opts.MaxTokens
	}
	if opts.RepoRoot != "" {
		body["repo_root"] = opts.RepoRoot
	}
	resp, err := c.t.do(ctx, "POST", "/v1/context-pack", body)
	if err != nil {
		return nil, err
	}
	var pack ContextPack
	if err := remarshal(resp.data, &pack); err != nil {
		return nil, &ConfigError{Message: "failed to decode context-pack response: " + err.Error()}
	}
	pack.Metering = resp.metering
	return &pack, nil
}

// IngestRepo indexes a repository into a bank. It returns the decoded status
// payload.
func (c *Client) IngestRepo(ctx context.Context, namespace, repoRoot, ref string) (map[string]any, error) {
	body := map[string]any{}
	if repoRoot != "" {
		body["repo_root"] = repoRoot
	}
	if ref != "" {
		body["ref"] = ref
	}
	path := "/v1/repos/" + url.PathEscape(namespace) + "/ingest"
	resp, err := c.t.do(ctx, "POST", path, body)
	if err != nil {
		return nil, err
	}
	if m, ok := resp.data.(map[string]any); ok {
		return m, nil
	}
	return map[string]any{}, nil
}

// Execute runs a batch of operations against /v1/execute.
func (c *Client) Execute(ctx context.Context, operations []Operation) (*ExecuteResponse, error) {
	resp, err := c.t.do(ctx, "POST", "/v1/execute", map[string]any{"operations": operations})
	if err != nil {
		return nil, err
	}
	var out ExecuteResponse
	if err := remarshal(resp.data, &out); err != nil {
		return nil, &ConfigError{Message: "failed to decode execute response: " + err.Error()}
	}
	return &out, nil
}
