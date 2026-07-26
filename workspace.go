package infolang

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// Whoami returns the identity echo for the current credential
// (GET /v2/whoami).
func (c *Client) Whoami(ctx context.Context) (*Whoami, error) {
	resp, err := c.t.do(ctx, "GET", "/v2/whoami", nil)
	if err != nil {
		return nil, err
	}
	var who Whoami
	if err := remarshal(resp.data, &who); err != nil {
		return nil, &ConfigError{Message: "failed to decode whoami response: " + err.Error()}
	}
	if raw, ok := resp.data.(map[string]any); ok {
		who.Raw = raw
	}
	return &who, nil
}

// WorkspaceID returns the workspace id every call operates in. Explicit
// configuration (WithWorkspace or INFOLANG_WORKSPACE / INFOLANG_WORKSPACE_ID)
// wins and needs no network call; otherwise the id is resolved once via
// Whoami and cached (single-grant credentials only — multi-workspace
// credentials must pass WithWorkspace). A failed resolution is never cached,
// so a retry re-asks the gateway.
func (c *Client) WorkspaceID(ctx context.Context) (string, error) {
	if c.Workspace != "" {
		return c.Workspace, nil
	}
	c.wsMu.Lock()
	defer c.wsMu.Unlock()
	if c.resolvedWS != "" {
		return c.resolvedWS, nil
	}
	who, err := c.Whoami(ctx)
	if err != nil {
		return "", err
	}
	ids := make([]string, 0, len(who.Workspaces))
	for _, w := range who.Workspaces {
		if w.WorkspaceID != "" {
			ids = append(ids, w.WorkspaceID)
		}
	}
	switch len(ids) {
	case 1:
		c.resolvedWS = ids[0]
		return ids[0], nil
	case 0:
		return "", &ConfigError{
			Message: "this credential has no visible workspace grants; mint a key " +
				"from the Console or pass WithWorkspace explicitly",
		}
	default:
		return "", &ConfigError{
			Message: fmt.Sprintf(
				"this credential can reach %d workspaces (%s); pass WithWorkspace "+
					"(or set INFOLANG_WORKSPACE) to pick one",
				len(ids), strings.Join(ids, ", ")),
		}
	}
}

// wsPath resolves the workspace and returns the /v2 workspace path prefix.
// Workspace ids are opaque and path-escaped verbatim.
func (c *Client) wsPath(ctx context.Context) (string, error) {
	ws, err := c.WorkspaceID(ctx)
	if err != nil {
		return "", err
	}
	return "/v2/workspaces/" + url.PathEscape(ws), nil
}
