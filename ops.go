package infolang

import "context"

// Execute batches primitives in one round trip
// (POST /v2/workspaces/{ws}/execute). The body is the simple {"operations"}
// batch — the gateway accepts it directly. The result is an OpResult; sub-op
// results sit under payload.results, one OpResult each, in order.
func (c *Client) Execute(ctx context.Context, operations []Operation) (*OpResult, error) {
	base, err := c.wsPath(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := c.t.do(ctx, "POST", base+"/execute", map[string]any{"operations": operations})
	if err != nil {
		return nil, err
	}
	return parseOpResult(resp.data)
}

// Stats returns server stats for the workspace
// (GET /v2/workspaces/{ws}/stats) in the native OpResult envelope.
func (c *Client) Stats(ctx context.Context) (*OpResult, error) {
	base, err := c.wsPath(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := c.t.do(ctx, "GET", base+"/stats", nil)
	if err != nil {
		return nil, err
	}
	return parseOpResult(resp.data)
}

func parseOpResult(data any) (*OpResult, error) {
	var out OpResult
	if err := remarshal(data, &out); err != nil {
		return nil, &ConfigError{Message: "failed to decode op result: " + err.Error()}
	}
	return &out, nil
}

// Encode encodes text to an embedding (POST /v2/workspaces/{ws}/encode). The
// response is the server payload, verbatim.
func (c *Client) Encode(ctx context.Context, text string) (map[string]any, error) {
	base, err := c.wsPath(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := c.t.do(ctx, "POST", base+"/encode", map[string]any{"text": text})
	if err != nil {
		return nil, err
	}
	if m, ok := resp.data.(map[string]any); ok {
		return m, nil
	}
	return map[string]any{}, nil
}

// Similarity scores two texts (POST /v2/workspaces/{ws}/similarity). The
// response is the server payload, verbatim.
func (c *Client) Similarity(ctx context.Context, text1, text2 string) (map[string]any, error) {
	base, err := c.wsPath(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := c.t.do(ctx, "POST", base+"/similarity", map[string]any{"text1": text1, "text2": text2})
	if err != nil {
		return nil, err
	}
	if m, ok := resp.data.(map[string]any); ok {
		return m, nil
	}
	return map[string]any{}, nil
}
