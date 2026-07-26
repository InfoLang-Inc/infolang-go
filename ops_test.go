package infolang

import (
	"context"
	"net/http"
	"testing"
)

func TestExecuteSimpleBatch(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"ok": true,
			"payload": map[string]any{
				"results": []map[string]any{
					{"ok": true, "payload": map[string]any{"n": 1}},
				},
			},
		})
	})
	c := newTestClient(t, ms.URL)
	out, err := c.Execute(context.Background(), []Operation{
		{Op: "recall", Args: map[string]any{"query": "x"}, Select: []string{"t"}},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if ms.last.Method != "POST" || ms.last.Path != "/v2/workspaces/ws/execute" {
		t.Errorf("route = %s %s", ms.last.Method, ms.last.Path)
	}
	// The body is the SIMPLE {"operations": [...]} batch, nothing else.
	if len(ms.last.Body) != 1 {
		t.Errorf("body should carry only operations: %v", ms.last.Body)
	}
	ops := ms.last.Body["operations"].([]any)
	op := ops[0].(map[string]any)
	if op["op"] != "recall" || op["args"].(map[string]any)["query"] != "x" {
		t.Errorf("unexpected op: %v", op)
	}
	if !out.OK || out.Payload == nil || out.Error != nil {
		t.Errorf("unexpected envelope: %+v", out)
	}
}

func TestExecuteOpResultError(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"ok": false,
			"error": map[string]any{
				"code": "capability_required", "message": "nope", "capability": "memory:write",
			},
		})
	})
	c := newTestClient(t, ms.URL)
	out, err := c.Execute(context.Background(), []Operation{{Op: "remember"}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.OK || out.Error == nil {
		t.Fatalf("unexpected envelope: %+v", out)
	}
	if out.Error.Code != "capability_required" || out.Error.Message != "nope" || out.Error.Capability != "memory:write" {
		t.Errorf("unexpected error: %+v", out.Error)
	}
}

func TestStatsOpResult(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"ok": true, "payload": map[string]any{"total_memories": 7},
		})
	})
	c := newTestClient(t, ms.URL)
	stats, err := c.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if ms.last.Method != "GET" || ms.last.Path != "/v2/workspaces/ws/stats" {
		t.Errorf("route = %s %s", ms.last.Method, ms.last.Path)
	}
	if !stats.OK || stats.Payload["total_memories"].(float64) != 7 {
		t.Errorf("unexpected stats: %+v", stats)
	}
}

func TestEncode(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"embedding": []any{0.1, 0.2}, "dims": 2})
	})
	c := newTestClient(t, ms.URL)
	out, err := c.Encode(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if ms.last.Method != "POST" || ms.last.Path != "/v2/workspaces/ws/encode" {
		t.Errorf("route = %s %s", ms.last.Method, ms.last.Path)
	}
	if ms.last.Body["text"] != "hello" || len(ms.last.Body) != 1 {
		t.Errorf("body = %v", ms.last.Body)
	}
	if out["dims"].(float64) != 2 {
		t.Errorf("payload not verbatim: %v", out)
	}
}

func TestSimilarity(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"similarity": 0.87})
	})
	c := newTestClient(t, ms.URL)
	out, err := c.Similarity(context.Background(), "one", "two")
	if err != nil {
		t.Fatalf("Similarity: %v", err)
	}
	if ms.last.Method != "POST" || ms.last.Path != "/v2/workspaces/ws/similarity" {
		t.Errorf("route = %s %s", ms.last.Method, ms.last.Path)
	}
	if ms.last.Body["text1"] != "one" || ms.last.Body["text2"] != "two" {
		t.Errorf("body = %v", ms.last.Body)
	}
	if out["similarity"].(float64) != 0.87 {
		t.Errorf("payload not verbatim: %v", out)
	}
}

func TestEncodeNonObjectPayload(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, []any{1, 2})
	})
	c := newTestClient(t, ms.URL)
	out, err := c.Encode(context.Background(), "x")
	if err != nil || len(out) != 0 {
		t.Errorf("want empty map, got %v err=%v", out, err)
	}
	out2, err := c.Similarity(context.Background(), "a", "b")
	if err != nil || len(out2) != 0 {
		t.Errorf("want empty map, got %v err=%v", out2, err)
	}
}
