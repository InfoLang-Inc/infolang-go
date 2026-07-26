package infolang

import "context"

func (c *Client) healthProbe(ctx context.Context, path string) (*HealthStatus, error) {
	resp, err := c.t.do(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var status HealthStatus
	if err := remarshal(resp.data, &status); err != nil {
		return nil, &ConfigError{Message: "failed to decode " + path + " response: " + err.Error()}
	}
	if raw, ok := resp.data.(map[string]any); ok {
		status.Raw = raw
	}
	return &status, nil
}

// Health checks gateway liveness (GET /healthz).
func (c *Client) Health(ctx context.Context) (*HealthStatus, error) {
	return c.healthProbe(ctx, "/healthz")
}

// Ready checks gateway readiness (GET /readyz) — the authoritative signal
// that the ML model is loaded and requests will succeed.
func (c *Client) Ready(ctx context.Context) (*HealthStatus, error) {
	return c.healthProbe(ctx, "/readyz")
}
