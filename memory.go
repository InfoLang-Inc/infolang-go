package infolang

import (
	"context"
	"net/url"
	"strconv"
)

// compactBody drops keys whose value is nil so the runtime applies its own
// defaults.
func compactBody(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if v != nil {
			out[k] = v
		}
	}
	return out
}

// Recall performs a semantic recall from a memory bank. A nil *RecallOptions
// uses the client defaults.
func (c *Client) Recall(ctx context.Context, query string, opts *RecallOptions) (*RecallResult, error) {
	if opts == nil {
		opts = &RecallOptions{}
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
	if opts.Filters != nil {
		body["filters"] = opts.Filters
	}
	if opts.Verbose {
		body["verbose"] = true
	}

	resp, err := c.t.do(ctx, "POST", "/v1/recall", body)
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

// recallWire mirrors both response shapes: the descriptive "hits" and the
// compact "chunks". Fields are pointers where a value is optional so an absent
// key stays distinguishable.
type recallWire struct {
	Namespace string   `json:"namespace"`
	Count     int      `json:"count"`
	Savings   *Savings `json:"savings"`
	Hits      []struct {
		ID         string   `json:"id"`
		Text       string   `json:"text"`
		Tags       string   `json:"tags"`
		Similarity *float64 `json:"similarity"`
	} `json:"hits"`
	Chunks []struct {
		I string   `json:"i"`
		T string   `json:"t"`
		G string   `json:"g"`
		S *float64 `json:"s"`
	} `json:"chunks"`
}

func parseRecall(resp *response) (*RecallResult, error) {
	var wire recallWire
	if err := remarshal(resp.data, &wire); err != nil {
		return nil, &ConfigError{Message: "failed to decode recall response: " + err.Error()}
	}
	result := &RecallResult{
		Namespace: wire.Namespace,
		Count:     wire.Count,
		Savings:   wire.Savings,
		Metering:  resp.metering,
	}
	// Prefer compact chunks; fall back to descriptive hits.
	if len(wire.Chunks) > 0 {
		for _, ch := range wire.Chunks {
			result.Chunks = append(result.Chunks, Chunk{
				ID:    ch.I,
				Text:  ch.T,
				Tags:  ch.G,
				Score: deref(ch.S),
			})
		}
	} else {
		for _, h := range wire.Hits {
			result.Chunks = append(result.Chunks, Chunk{
				ID:    h.ID,
				Text:  h.Text,
				Tags:  h.Tags,
				Score: deref(h.Similarity),
			})
		}
	}
	return result, nil
}

func deref(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

// Remember stores a memory in the caller namespace.
func (c *Client) Remember(ctx context.Context, text string, opts *RememberOptions) (*RememberResult, error) {
	if opts == nil {
		opts = &RememberOptions{}
	}
	ns := opts.Namespace
	if ns == "" {
		ns = c.Namespace
	}
	body := compactBody(map[string]any{
		"text":      text,
		"namespace": emptyToNil(ns),
		"source":    emptyToNil(opts.Source),
		"tags":      emptyToNil(opts.Tags),
	})
	resp, err := c.t.do(ctx, "POST", "/v1/remember", body)
	if err != nil {
		return nil, err
	}
	var result RememberResult
	if err := remarshal(resp.data, &result); err != nil {
		return nil, &ConfigError{Message: "failed to decode remember response: " + err.Error()}
	}
	return &result, nil
}

// RememberBatch stores many memories in a single round-trip via the runtime's
// batched remember_batch op on /v1/execute.
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

	batchItems := make([]map[string]any, 0, len(items))
	for _, item := range items {
		source := item.Source
		if source == "" {
			source = opts.Source
		}
		entry := compactBody(map[string]any{
			"text":   item.Text,
			"source": emptyToNil(source),
		})
		if len(item.Tags) > 0 {
			entry["tags"] = item.Tags
		}
		batchItems = append(batchItems, entry)
	}
	args := compactBody(map[string]any{
		"items":     batchItems,
		"namespace": emptyToNil(ns),
		"source":    emptyToNil(opts.Source),
	})

	execResp, err := c.Execute(ctx, []Operation{{Op: "remember_batch", Args: args}})
	if err != nil {
		return nil, err
	}
	return parseBatchResults(execResp), nil
}

// parseBatchResults flattens an execute response into per-item results. The
// batched remember_batch op nests per-item results under payload.results; older
// per-item ops carry a single remember result in payload.
func parseBatchResults(resp *ExecuteResponse) []RememberResult {
	out := []RememberResult{}
	for _, r := range resp.Results {
		if inner, ok := r.Payload["results"].([]any); ok {
			for _, item := range inner {
				var rr RememberResult
				_ = remarshal(item, &rr)
				out = append(out, rr)
			}
			continue
		}
		var rr RememberResult
		_ = remarshal(r.Payload, &rr)
		out = append(out, rr)
	}
	return out
}

// Forget deletes a memory by id. Forget is scoped by the credential and the
// workspace header; the options namespace is reserved for future use.
func (c *Client) Forget(ctx context.Context, memoryID string, opts *ForgetOptions) error {
	_ = opts
	_, err := c.t.do(ctx, "DELETE", "/v1/memories/"+url.PathEscape(memoryID), nil)
	return err
}

// ListBanks lists the available memory banks.
func (c *Client) ListBanks(ctx context.Context) ([]Bank, error) {
	resp, err := c.t.do(ctx, "GET", "/v1/banks", nil)
	if err != nil {
		return nil, err
	}
	var wire struct {
		Banks []Bank `json:"banks"`
	}
	if err := remarshal(resp.data, &wire); err != nil {
		return nil, &ConfigError{Message: "failed to decode banks response: " + err.Error()}
	}
	for i := range wire.Banks {
		if wire.Banks[i].Count == 0 && wire.Banks[i].TotalMemories > 0 {
			wire.Banks[i].Count = wire.Banks[i].TotalMemories
		}
	}
	return wire.Banks, nil
}

// ListRecent lists recent memories in the caller namespace.
func (c *Client) ListRecent(ctx context.Context, opts *ListRecentOptions) ([]map[string]any, error) {
	if opts == nil {
		opts = &ListRecentOptions{}
	}
	ns := opts.Namespace
	if ns == "" {
		ns = c.Namespace
	}
	q := url.Values{}
	if ns != "" {
		q.Set("namespace", ns)
	}
	if opts.N > 0 {
		q.Set("limit", strconv.Itoa(opts.N))
	}
	path := "/v1/memories"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	resp, err := c.t.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var wire struct {
		Memories []map[string]any `json:"memories"`
	}
	if err := remarshal(resp.data, &wire); err != nil {
		return nil, &ConfigError{Message: "failed to decode recent response: " + err.Error()}
	}
	if wire.Memories == nil {
		return []map[string]any{}, nil
	}
	return wire.Memories, nil
}

// Stats returns namespace/store statistics as a decoded map.
func (c *Client) Stats(ctx context.Context) (map[string]any, error) {
	resp, err := c.t.do(ctx, "GET", "/v1/stats", nil)
	if err != nil {
		return nil, err
	}
	if m, ok := resp.data.(map[string]any); ok {
		return m, nil
	}
	return map[string]any{}, nil
}

func emptyToNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}
