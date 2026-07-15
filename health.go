package infolang

import "context"

// Health checks runtime liveness/readiness.
func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	resp, err := c.t.do(ctx, "GET", "/v1/health", nil)
	if err != nil {
		return nil, err
	}
	var health HealthResponse
	if err := remarshal(resp.data, &health); err != nil {
		return nil, &ConfigError{Message: "failed to decode health response: " + err.Error()}
	}
	return &health, nil
}
