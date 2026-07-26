package infolang

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

// splitTags splits a comma-separated tag string into the JSON array the
// API expects, trimming whitespace and dropping empties.
func splitTags(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Recall performs a semantic recall (POST /v2/workspaces/{ws}/recall). A nil
// *RecallOptions uses the client defaults.
//
// Reserved: the retrieval extras (Golden, Format, SnippetChars, Adaptive,
// Margin) are accepted today and activate in a future release.
func (c *Client) Recall(ctx context.Context, query string, opts *RecallOptions) (*RecallResult, error) {
	if opts == nil {
		opts = &RecallOptions{}
	}
	base, err := c.wsPath(ctx)
	if err != nil {
		return nil, err
	}
	ns := opts.Namespace
	if ns == "" {
		ns = c.Namespace
	}
	body := map[string]any{"query": query}
	if ns != "" {
		body["namespace"] = ns
	}
	if opts.TopK > 0 {
		body["top_k"] = opts.TopK
	}
	if opts.Verbose {
		body["verbose"] = true
	}
	if opts.Golden {
		body["golden"] = true
	}
	if opts.Format != "" {
		body["format"] = opts.Format
	}
	if opts.SnippetChars > 0 {
		body["snippet_chars"] = opts.SnippetChars
	}
	if opts.Adaptive {
		body["adaptive"] = true
	}
	if opts.Margin != 0 {
		body["margin"] = opts.Margin
	}

	resp, err := c.t.do(ctx, "POST", base+"/recall", body)
	if err != nil {
		return nil, err
	}
	return parseRecall(resp)
}

// Investigate is agent-style recall with a default TopK of 5. A nil
// *InvestigateOptions uses those defaults.
func (c *Client) Investigate(ctx context.Context, query string, opts *InvestigateOptions) (*RecallResult, error) {
	if opts == nil {
		opts = &InvestigateOptions{}
	}
	topK := opts.TopK
	if topK == 0 {
		topK = 5
	}
	return c.Recall(ctx, query, &RecallOptions{Namespace: opts.NamespaceHint, TopK: topK})
}

// recallWire mirrors the hits response returned by the gateway.
type recallWire struct {
	Namespace string   `json:"namespace"`
	LatencyMs *float64 `json:"latency_ms"`
	Hits      []struct {
		ID         string   `json:"id"`
		Text       string   `json:"text"`
		Tags       string   `json:"tags"`
		Similarity *float64 `json:"similarity"`
		Source     string   `json:"source"`
		Timestamp  *float64 `json:"timestamp"`
	} `json:"hits"`
}

func parseRecall(resp *response) (*RecallResult, error) {
	var wire recallWire
	if err := remarshal(resp.data, &wire); err != nil {
		return nil, &ConfigError{Message: "failed to decode recall response: " + err.Error()}
	}
	result := &RecallResult{
		Namespace: wire.Namespace,
		LatencyMs: deref(wire.LatencyMs),
		Metering:  resp.metering,
	}
	for _, h := range wire.Hits {
		result.Chunks = append(result.Chunks, Chunk{
			ID:        h.ID,
			Text:      h.Text,
			Tags:      h.Tags,
			Score:     deref(h.Similarity),
			Source:    h.Source,
			Timestamp: deref(h.Timestamp),
		})
	}
	return result, nil
}

func deref(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

// rememberWire tolerates both id spellings the server emits.
type rememberWire struct {
	ID             string `json:"id"`
	MemoryID       string `json:"memory_id"`
	Namespace      string `json:"namespace"`
	Stored         *bool  `json:"stored"`
	Deduplicated   bool   `json:"deduplicated"`
	DedupedAgainst string `json:"deduped_against"`
	TotalMemories  int    `json:"total_memories"`
}

func parseRemember(data any) (*RememberResult, error) {
	var wire rememberWire
	if err := remarshal(data, &wire); err != nil {
		return nil, &ConfigError{Message: "failed to decode remember response: " + err.Error()}
	}
	id := wire.ID
	if id == "" {
		id = wire.MemoryID
	}
	stored := false
	if wire.Stored != nil {
		stored = *wire.Stored
	}
	return &RememberResult{
		MemoryID:       id,
		Namespace:      wire.Namespace,
		Stored:         stored,
		Deduplicated:   wire.Deduplicated,
		DedupedAgainst: wire.DedupedAgainst,
		TotalMemories:  wire.TotalMemories,
	}, nil
}

// Remember stores a memory (POST /v2/workspaces/{ws}/remember). Tags are
// always sent as a JSON array; the comma-separated Tags option is split
// client-side.
func (c *Client) Remember(ctx context.Context, text string, opts *RememberOptions) (*RememberResult, error) {
	if opts == nil {
		opts = &RememberOptions{}
	}
	base, err := c.wsPath(ctx)
	if err != nil {
		return nil, err
	}
	ns := opts.Namespace
	if ns == "" {
		ns = c.Namespace
	}
	body := map[string]any{"text": text}
	if ns != "" {
		body["namespace"] = ns
	}
	if opts.Source != "" {
		body["source"] = opts.Source
	}
	if tags := splitTags(opts.Tags); tags != nil {
		body["tags"] = tags
	}
	resp, err := c.t.do(ctx, "POST", base+"/remember", body)
	if err != nil {
		return nil, err
	}
	return parseRemember(resp.data)
}

// RememberBatch stores many memories in a single round-trip. It is client-side
// sugar over Execute with one real "remember" sub-op per item (the old
// remember_batch pseudo-op is gone). Sub-op results come back one OpResult
// each, in order; an empty input short-circuits without a request.
func (c *Client) RememberBatch(ctx context.Context, items []RememberItem, opts *RememberOptions) ([]RememberResult, error) {
	if len(items) == 0 {
		return []RememberResult{}, nil
	}
	if opts == nil {
		opts = &RememberOptions{}
	}
	ns := opts.Namespace
	if ns == "" {
		ns = c.Namespace
	}

	operations := make([]Operation, 0, len(items))
	for _, item := range items {
		source := item.Source
		if source == "" {
			source = opts.Source
		}
		args := map[string]any{"text": item.Text}
		if source != "" {
			args["source"] = source
		}
		if len(item.Tags) > 0 {
			args["tags"] = item.Tags
		}
		if ns != "" {
			args["namespace"] = ns
		}
		operations = append(operations, Operation{Op: "remember", Args: args})
	}

	envelope, err := c.Execute(ctx, operations)
	if err != nil {
		return nil, err
	}
	return parseExecuteRememberBatch(envelope), nil
}

// parseExecuteRememberBatch unwraps payload.results[].payload from the execute
// envelope into per-item remember results.
func parseExecuteRememberBatch(envelope *OpResult) []RememberResult {
	out := []RememberResult{}
	if envelope == nil || envelope.Payload == nil {
		return out
	}
	results, ok := envelope.Payload["results"].([]any)
	if !ok {
		return out
	}
	for _, item := range results {
		var payload any
		if m, ok := item.(map[string]any); ok {
			payload = m["payload"]
		}
		rr, err := parseRemember(payload)
		if err != nil || rr == nil {
			rr = &RememberResult{}
		}
		out = append(out, *rr)
	}
	return out
}

// Forget deletes a memory by id
// (DELETE /v2/workspaces/{ws}/memories/{id}?namespace=). The namespace query
// parameter is sent only when set.
func (c *Client) Forget(ctx context.Context, memoryID string, opts *ForgetOptions) error {
	base, err := c.wsPath(ctx)
	if err != nil {
		return err
	}
	path := base + "/memories/" + url.PathEscape(memoryID)
	if opts != nil && opts.Namespace != "" {
		q := url.Values{}
		q.Set("namespace", opts.Namespace)
		path += "?" + q.Encode()
	}
	_, err = c.t.do(ctx, "DELETE", path, nil)
	return err
}

// List pages memories (GET /v2/workspaces/{ws}/memories?ns=&q=&limit=). A
// Query switches to semantic search; results then carry Score.
func (c *Client) List(ctx context.Context, opts *ListOptions) (*MemoryPage, error) {
	if opts == nil {
		opts = &ListOptions{}
	}
	base, err := c.wsPath(ctx)
	if err != nil {
		return nil, err
	}
	ns := opts.Namespace
	if ns == "" {
		ns = c.Namespace
	}
	q := url.Values{}
	if ns != "" {
		q.Set("ns", ns)
	}
	if opts.Query != "" {
		q.Set("q", opts.Query)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	path := base + "/memories"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	resp, err := c.t.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var page MemoryPage
	if err := remarshal(resp.data, &page); err != nil {
		return nil, &ConfigError{Message: "failed to decode memories response: " + err.Error()}
	}
	if page.Memories == nil {
		page.Memories = []MemoryItem{}
	}
	return &page, nil
}

// ListRecent lists recent memories.
//
// Deprecated: use List, which exposes the gateway MemoryPage (cursors and
// search scores). ListRecent is a thin wrapper kept for 0.1.x callers.
func (c *Client) ListRecent(ctx context.Context, opts *ListRecentOptions) ([]map[string]any, error) {
	if opts == nil {
		opts = &ListRecentOptions{}
	}
	page, err := c.List(ctx, &ListOptions{Namespace: opts.Namespace, Limit: opts.N})
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(page.Memories))
	for _, item := range page.Memories {
		m := map[string]any{}
		_ = remarshal(item, &m)
		out = append(out, m)
	}
	return out, nil
}

// Namespaces lists the namespaces in the workspace with per-namespace memory
// and chunk counts (GET /v2/workspaces/{ws}/namespaces). Bare string entries
// are tolerated and surface with zero counts.
func (c *Client) Namespaces(ctx context.Context) ([]NamespaceInfo, error) {
	base, err := c.wsPath(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := c.t.do(ctx, "GET", base+"/namespaces", nil)
	if err != nil {
		return nil, err
	}
	record, _ := resp.data.(map[string]any)
	items, _ := record["namespaces"].([]any)
	out := make([]NamespaceInfo, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, NamespaceInfo{Namespace: s})
			continue
		}
		var info NamespaceInfo
		if err := remarshal(item, &info); err != nil {
			return nil, &ConfigError{Message: "failed to decode namespaces response: " + err.Error()}
		}
		out = append(out, info)
	}
	return out, nil
}
