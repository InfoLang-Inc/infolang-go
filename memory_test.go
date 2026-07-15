package infolang

import (
	"context"
	"net/http"
	"reflect"
	"testing"
)

func TestRecallNormalizesHits(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"namespace": "default",
			"count":     2,
			"hits": []map[string]any{
				{"id": "m1", "text": "alpha", "tags": "a,b", "similarity": 0.92},
				{"id": "m2", "text": "beta", "tags": "", "similarity": 0.40},
			},
		})
	})
	c := newTestClient(t, ms.URL, WithNamespace("default"))

	res, err := c.Recall(context.Background(), "q", &RecallOptions{TopK: 2, Verbose: true, Filters: map[string]any{"k": "v"}})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(res.Chunks) != 2 {
		t.Fatalf("want 2 chunks, got %d", len(res.Chunks))
	}
	if res.Chunks[0].ID != "m1" || res.Chunks[0].Text != "alpha" || res.Chunks[0].Tags != "a,b" || res.Chunks[0].Score != 0.92 {
		t.Errorf("unexpected chunk[0]: %+v", res.Chunks[0])
	}
	if res.Namespace != "default" || res.Count != 2 {
		t.Errorf("unexpected metadata: ns=%q count=%d", res.Namespace, res.Count)
	}
	// Request body assertions against the contract.
	if ms.last.Body["query"] != "q" || ms.last.Body["namespace"] != "default" {
		t.Errorf("unexpected body: %v", ms.last.Body)
	}
	if ms.last.Body["top_k"].(float64) != 2 || ms.last.Body["verbose"] != true {
		t.Errorf("missing top_k/verbose: %v", ms.last.Body)
	}
	if res.Weak() {
		t.Error("top score 0.92 should not be weak")
	}
}

func TestRecallNormalizesChunks(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"chunks": []map[string]any{
				{"i": "c1", "t": "gamma", "g": "x", "s": 0.5},
			},
		})
	})
	c := newTestClient(t, ms.URL)
	res, err := c.Recall(context.Background(), "q", nil)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(res.Chunks) != 1 || res.Chunks[0].ID != "c1" || res.Chunks[0].Text != "gamma" {
		t.Fatalf("unexpected chunks: %+v", res.Chunks)
	}
	if !res.Weak() {
		t.Error("top score 0.5 should be weak")
	}
	// nil opts -> no top_k/verbose/namespace keys in body
	if _, ok := ms.last.Body["top_k"]; ok {
		t.Error("top_k should be omitted for nil opts")
	}
}

func TestRecallWeakEmpty(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"hits": []any{}})
	})
	c := newTestClient(t, ms.URL)
	res, err := c.Recall(context.Background(), "q", nil)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if res.Weak() {
		t.Error("empty result should not be weak")
	}
}

func TestInvestigateDefaultsTopK(t *testing.T) {
	tests := []struct {
		name     string
		opts     *InvestigateOptions
		wantTopK float64
		wantNS   any
	}{
		{"nil opts", nil, 5, nil},
		{"explicit topK", &InvestigateOptions{TopK: 9, NamespaceHint: "bank"}, 9, "bank"},
		{"zero topK falls back", &InvestigateOptions{NamespaceHint: "bank"}, 5, "bank"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, 200, map[string]any{"hits": []any{}})
			})
			c := newTestClient(t, ms.URL)
			if _, err := c.Investigate(context.Background(), "q", tt.opts); err != nil {
				t.Fatalf("Investigate: %v", err)
			}
			if ms.last.Body["top_k"].(float64) != tt.wantTopK {
				t.Errorf("top_k = %v, want %v", ms.last.Body["top_k"], tt.wantTopK)
			}
			if ms.last.Body["namespace"] != tt.wantNS {
				t.Errorf("namespace = %v, want %v", ms.last.Body["namespace"], tt.wantNS)
			}
		})
	}
}

func TestRemember(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"id": "mem-1", "namespace": "docs", "stored": true, "total_memories": 42,
		})
	})
	c := newTestClient(t, ms.URL, WithNamespace("default"))
	res, err := c.Remember(context.Background(), "a fact", &RememberOptions{Namespace: "docs", Source: "f.md", Tags: "a,b"})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if res.MemoryID != "mem-1" || !res.Stored || res.TotalMemories != 42 {
		t.Errorf("unexpected result: %+v", res)
	}
	if ms.last.Path != "/v1/remember" || ms.last.Method != "POST" {
		t.Errorf("unexpected route: %s %s", ms.last.Method, ms.last.Path)
	}
	if ms.last.Body["text"] != "a fact" || ms.last.Body["namespace"] != "docs" ||
		ms.last.Body["source"] != "f.md" || ms.last.Body["tags"] != "a,b" {
		t.Errorf("unexpected body: %v", ms.last.Body)
	}
}

func TestRememberUsesClientNamespace(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"id": "x"})
	})
	c := newTestClient(t, ms.URL, WithNamespace("fallback"))
	if _, err := c.Remember(context.Background(), "t", nil); err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if ms.last.Body["namespace"] != "fallback" {
		t.Errorf("namespace = %v, want fallback", ms.last.Body["namespace"])
	}
	if _, ok := ms.last.Body["source"]; ok {
		t.Error("empty source should be omitted")
	}
}

func TestRememberBatch(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"results": []map[string]any{
				{"op": "remember_batch", "ok": true, "payload": map[string]any{
					"results": []map[string]any{
						{"id": "a", "stored": true},
						{"id": "b", "stored": true},
					},
				}},
			},
		})
	})
	c := newTestClient(t, ms.URL, WithNamespace("eval"))
	items := []RememberItem{
		{Text: "one", Tags: []string{"t1"}},
		{Text: "two", Source: "s2"},
	}
	res, err := c.RememberBatch(context.Background(), items, &RememberOptions{Source: "batch"})
	if err != nil {
		t.Fatalf("RememberBatch: %v", err)
	}
	if len(res) != 2 || res[0].MemoryID != "a" || res[1].MemoryID != "b" {
		t.Fatalf("unexpected results: %+v", res)
	}
	// Verify it went through /v1/execute with a remember_batch op.
	if ms.last.Path != "/v1/execute" {
		t.Fatalf("want /v1/execute, got %s", ms.last.Path)
	}
	ops := ms.last.Body["operations"].([]any)
	op := ops[0].(map[string]any)
	if op["op"] != "remember_batch" {
		t.Errorf("op = %v", op["op"])
	}
	args := op["args"].(map[string]any)
	if args["namespace"] != "eval" || args["source"] != "batch" {
		t.Errorf("unexpected args: %v", args)
	}
	batchItems := args["items"].([]any)
	if len(batchItems) != 2 {
		t.Fatalf("want 2 items, got %d", len(batchItems))
	}
	first := batchItems[0].(map[string]any)
	if !reflect.DeepEqual(first["tags"], []any{"t1"}) {
		t.Errorf("tags = %v", first["tags"])
	}
}

func TestRememberBatchEmpty(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("server should not be called for empty batch")
	})
	c := newTestClient(t, ms.URL)
	res, err := c.RememberBatch(context.Background(), nil, nil)
	if err != nil || len(res) != 0 {
		t.Fatalf("want empty, got %+v err=%v", res, err)
	}
}

func TestRememberBatchLegacyPayload(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"results": []map[string]any{
				{"op": "remember", "ok": true, "payload": map[string]any{"id": "solo", "stored": true}},
			},
		})
	})
	c := newTestClient(t, ms.URL)
	res, err := c.RememberBatch(context.Background(), []RememberItem{{Text: "x"}}, nil)
	if err != nil {
		t.Fatalf("RememberBatch: %v", err)
	}
	if len(res) != 1 || res[0].MemoryID != "solo" {
		t.Fatalf("unexpected: %+v", res)
	}
}

func TestForget(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	})
	c := newTestClient(t, ms.URL)
	if err := c.Forget(context.Background(), "mem id/with?chars", nil); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if ms.last.Method != "DELETE" {
		t.Errorf("method = %s", ms.last.Method)
	}
	if ms.last.EscapedPath != "/v1/memories/mem%20id%2Fwith%3Fchars" {
		t.Errorf("escaped path = %q", ms.last.EscapedPath)
	}
}

func TestListBanks(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"banks": []map[string]any{
				{"namespace": "a", "total_memories": 10},
				{"namespace": "b", "count": 3, "parquet_size_mb": 1.5, "sample_sources": []string{"x"}},
			},
		})
	})
	c := newTestClient(t, ms.URL)
	banks, err := c.ListBanks(context.Background())
	if err != nil {
		t.Fatalf("ListBanks: %v", err)
	}
	if len(banks) != 2 {
		t.Fatalf("want 2 banks, got %d", len(banks))
	}
	if banks[0].Count != 10 { // aliased from total_memories
		t.Errorf("bank a count = %d, want 10", banks[0].Count)
	}
	if banks[1].Count != 3 || banks[1].ParquetSizeMB != 1.5 {
		t.Errorf("unexpected bank b: %+v", banks[1])
	}
}

func TestListRecent(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"memories": []map[string]any{{"id": "1"}, {"id": "2"}},
		})
	})
	c := newTestClient(t, ms.URL, WithNamespace("ns"))
	items, err := c.ListRecent(context.Background(), &ListRecentOptions{N: 5})
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2, got %d", len(items))
	}
	if ms.last.Query != "limit=5&namespace=ns" {
		t.Errorf("query = %q", ms.last.Query)
	}
}

func TestListRecentEmpty(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{})
	})
	c := newTestClient(t, ms.URL)
	items, err := c.ListRecent(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if items == nil || len(items) != 0 {
		t.Errorf("want empty non-nil slice, got %v", items)
	}
}

func TestStats(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"total": 7})
	})
	c := newTestClient(t, ms.URL)
	stats, err := c.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats["total"].(float64) != 7 {
		t.Errorf("stats = %v", stats)
	}
	if ms.last.Path != "/v1/stats" {
		t.Errorf("path = %s", ms.last.Path)
	}
}

func TestStatsNonObject(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, []any{1, 2})
	})
	c := newTestClient(t, ms.URL)
	stats, err := c.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("want empty map, got %v", stats)
	}
}
