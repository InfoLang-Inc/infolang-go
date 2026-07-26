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
			"namespace":  "default",
			"latency_ms": 12.5,
			"hits": []map[string]any{
				{"id": "m1", "text": "alpha", "tags": "a,b", "similarity": 0.92, "source": "f.md", "timestamp": 1700000000},
				{"id": "m2", "text": "beta", "tags": "", "similarity": 0.40},
			},
		})
	})
	c := newTestClient(t, ms.URL, WithNamespace("default"))

	res, err := c.Recall(context.Background(), "q", &RecallOptions{TopK: 2, Verbose: true})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(res.Chunks) != 2 {
		t.Fatalf("want 2 chunks, got %d", len(res.Chunks))
	}
	first := res.Chunks[0]
	if first.ID != "m1" || first.Text != "alpha" || first.Tags != "a,b" || first.Score != 0.92 ||
		first.Source != "f.md" || first.Timestamp != 1700000000 {
		t.Errorf("unexpected chunk[0]: %+v", first)
	}
	if res.Namespace != "default" || res.LatencyMs != 12.5 {
		t.Errorf("unexpected metadata: ns=%q latency=%v", res.Namespace, res.LatencyMs)
	}
	// Request path + body assertions against the /v2 contract.
	if ms.last.Method != "POST" || ms.last.Path != "/v2/workspaces/ws/recall" {
		t.Errorf("route = %s %s", ms.last.Method, ms.last.Path)
	}
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

func TestRecallWeak(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"hits": []map[string]any{{"id": "c1", "text": "gamma", "similarity": 0.5}},
		})
	})
	c := newTestClient(t, ms.URL)
	res, err := c.Recall(context.Background(), "q", nil)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if !res.Weak() {
		t.Error("top score 0.5 should be weak")
	}
	// nil opts -> no optional keys in body
	for _, key := range []string{"top_k", "verbose", "namespace", "golden", "format", "snippet_chars", "adaptive", "margin"} {
		if _, ok := ms.last.Body[key]; ok {
			t.Errorf("%s should be omitted for nil opts", key)
		}
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

// The retrieval extras ride the body only when set (golden omitempty
// serialization). Reserved fields: the wire shape is asserted here so the
// SDK is ready when they activate.
func TestRecallRetrievalExtrasSerialized(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"hits": []any{}})
	})
	c := newTestClient(t, ms.URL)
	_, err := c.Recall(context.Background(), "q", &RecallOptions{
		Golden:       true,
		Format:       "snippet",
		SnippetChars: 240,
		Adaptive:     true,
		Margin:       0.12,
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	b := ms.last.Body
	if b["golden"] != true || b["format"] != "snippet" || b["snippet_chars"].(float64) != 240 ||
		b["adaptive"] != true || b["margin"].(float64) != 0.12 {
		t.Errorf("extras missing from body: %v", b)
	}
}

func TestRecallExtrasOmittedWhenZero(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"hits": []any{}})
	})
	c := newTestClient(t, ms.URL)
	if _, err := c.Recall(context.Background(), "q", &RecallOptions{TopK: 3}); err != nil {
		t.Fatalf("Recall: %v", err)
	}
	for _, key := range []string{"golden", "format", "snippet_chars", "adaptive", "margin"} {
		if _, ok := ms.last.Body[key]; ok {
			t.Errorf("%s should be omitted when zero", key)
		}
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

func TestRememberSplitsTagsIntoArray(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"id": "mem-1", "namespace": "docs", "stored": true, "total_memories": 42,
		})
	})
	c := newTestClient(t, ms.URL, WithNamespace("default"))
	res, err := c.Remember(context.Background(), "a fact", &RememberOptions{Namespace: "docs", Source: "f.md", Tags: " a, b ,, c"})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if res.MemoryID != "mem-1" || !res.Stored || res.TotalMemories != 42 {
		t.Errorf("unexpected result: %+v", res)
	}
	if ms.last.Path != "/v2/workspaces/ws/remember" || ms.last.Method != "POST" {
		t.Errorf("route = %s %s", ms.last.Method, ms.last.Path)
	}
	if ms.last.Body["text"] != "a fact" || ms.last.Body["namespace"] != "docs" || ms.last.Body["source"] != "f.md" {
		t.Errorf("unexpected body: %v", ms.last.Body)
	}
	// Tags are ALWAYS a JSON array: comma string split, trimmed, empties dropped.
	if !reflect.DeepEqual(ms.last.Body["tags"], []any{"a", "b", "c"}) {
		t.Errorf("tags = %#v, want [a b c]", ms.last.Body["tags"])
	}
}

func TestRememberDedupFields(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"memory_id": "mem-old", "namespace": "docs", "stored": false,
			"deduplicated": true, "deduped_against": "mem-old", "total_memories": 42,
		})
	})
	c := newTestClient(t, ms.URL)
	res, err := c.Remember(context.Background(), "dup", nil)
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if res.Stored || !res.Deduplicated || res.DedupedAgainst != "mem-old" {
		t.Errorf("dedup fields not surfaced: %+v", res)
	}
	if res.MemoryID != "mem-old" { // memory_id spelling tolerated
		t.Errorf("memory id = %q", res.MemoryID)
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
	for _, key := range []string{"source", "tags"} {
		if _, ok := ms.last.Body[key]; ok {
			t.Errorf("empty %s should be omitted", key)
		}
	}
}

func TestRememberBatchSubOps(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"ok": true,
			"payload": map[string]any{
				"results": []map[string]any{
					{"ok": true, "payload": map[string]any{"id": "a", "stored": true}},
					{"ok": true, "payload": map[string]any{"id": "b", "stored": false, "deduplicated": true, "deduped_against": "a"}},
				},
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
	if !res[1].Deduplicated || res[1].DedupedAgainst != "a" {
		t.Errorf("dedup fields lost: %+v", res[1])
	}
	// One real "remember" sub-op per item on the /v2 execute route.
	if ms.last.Path != "/v2/workspaces/ws/execute" {
		t.Fatalf("want /v2/workspaces/ws/execute, got %s", ms.last.Path)
	}
	ops := ms.last.Body["operations"].([]any)
	if len(ops) != 2 {
		t.Fatalf("want 2 sub-ops, got %d", len(ops))
	}
	first := ops[0].(map[string]any)
	if first["op"] != "remember" {
		t.Errorf("op = %v, want remember", first["op"])
	}
	args := first["args"].(map[string]any)
	if args["text"] != "one" || args["namespace"] != "eval" || args["source"] != "batch" {
		t.Errorf("unexpected args: %v", args)
	}
	if !reflect.DeepEqual(args["tags"], []any{"t1"}) {
		t.Errorf("tags = %v", args["tags"])
	}
	second := ops[1].(map[string]any)
	if second["args"].(map[string]any)["source"] != "s2" { // per-item source wins
		t.Errorf("item source not honored: %v", second)
	}
}

func TestRememberBatchEmptyShortCircuits(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("server should not be called for empty batch")
	})
	c := newTestClient(t, ms.URL)
	res, err := c.RememberBatch(context.Background(), nil, nil)
	if err != nil || res == nil || len(res) != 0 {
		t.Fatalf("want empty non-nil slice, got %+v err=%v", res, err)
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
	if ms.last.EscapedPath != "/v2/workspaces/ws/memories/mem%20id%2Fwith%3Fchars" {
		t.Errorf("escaped path = %q", ms.last.EscapedPath)
	}
	if ms.last.Query != "" {
		t.Errorf("no namespace -> no query, got %q", ms.last.Query)
	}
}

func TestForgetWithNamespace(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	})
	c := newTestClient(t, ms.URL)
	if err := c.Forget(context.Background(), "mem-1", &ForgetOptions{Namespace: "docs"}); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if ms.last.Query != "namespace=docs" {
		t.Errorf("query = %q, want namespace=docs", ms.last.Query)
	}
}

func TestList(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"memories": []map[string]any{
				{"id": "1", "text": "t1", "namespace": "ns", "tags": []string{"a"}, "score": 0.9},
				{"id": "2", "text": "t2", "namespace": "ns"},
			},
			"nextCursor": "cursor-2",
		})
	})
	c := newTestClient(t, ms.URL)
	page, err := c.List(context.Background(), &ListOptions{Namespace: "ns", Query: "topic", Limit: 5})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if ms.last.Method != "GET" || ms.last.Path != "/v2/workspaces/ws/memories" {
		t.Errorf("route = %s %s", ms.last.Method, ms.last.Path)
	}
	if ms.last.Query != "limit=5&ns=ns&q=topic" {
		t.Errorf("query = %q", ms.last.Query)
	}
	if len(page.Memories) != 2 || page.NextCursor != "cursor-2" {
		t.Fatalf("unexpected page: %+v", page)
	}
	if page.Memories[0].Score == nil || *page.Memories[0].Score != 0.9 {
		t.Errorf("search score not parsed: %+v", page.Memories[0])
	}
	if page.Memories[1].Score != nil {
		t.Errorf("absent score should stay nil: %+v", page.Memories[1])
	}
}

func TestListDefaultsAndEmpty(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{})
	})
	c := newTestClient(t, ms.URL, WithNamespace("clientns"))
	page, err := c.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if page.Memories == nil || len(page.Memories) != 0 || page.NextCursor != "" {
		t.Errorf("want empty non-nil page, got %+v", page)
	}
	if ms.last.Query != "ns=clientns" {
		t.Errorf("client namespace should ride ns=, got %q", ms.last.Query)
	}
}

func TestListRecentDeprecatedWrapper(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"memories": []map[string]any{{"id": "1", "text": "t", "namespace": "ns"}},
		})
	})
	c := newTestClient(t, ms.URL)
	items, err := c.ListRecent(context.Background(), &ListRecentOptions{Namespace: "ns", N: 3})
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(items) != 1 || items[0]["id"] != "1" {
		t.Fatalf("unexpected items: %+v", items)
	}
	if ms.last.Path != "/v2/workspaces/ws/memories" || ms.last.Query != "limit=3&ns=ns" {
		t.Errorf("route = %s?%s", ms.last.Path, ms.last.Query)
	}
}

func TestNamespaces(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"namespaces": []any{
				map[string]any{"namespace": "docs", "memories": 12, "chunks": 40},
				"bare-string",
			},
		})
	})
	c := newTestClient(t, ms.URL)
	out, err := c.Namespaces(context.Background())
	if err != nil {
		t.Fatalf("Namespaces: %v", err)
	}
	if ms.last.Method != "GET" || ms.last.Path != "/v2/workspaces/ws/namespaces" {
		t.Errorf("route = %s %s", ms.last.Method, ms.last.Path)
	}
	want := []NamespaceInfo{
		{Namespace: "docs", Memories: 12, Chunks: 40},
		{Namespace: "bare-string"},
	}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("namespaces = %+v, want %+v", out, want)
	}
}

func TestNamespacesEmpty(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"namespaces": []any{}})
	})
	c := newTestClient(t, ms.URL)
	out, err := c.Namespaces(context.Background())
	if err != nil || out == nil || len(out) != 0 {
		t.Errorf("want empty non-nil slice, got %+v err=%v", out, err)
	}
}

func TestSplitTags(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b", []string{"a", "b"}},
		{" a , b ,, ", []string{"a", "b"}},
		{",,", nil},
	}
	for _, tc := range cases {
		if got := splitTags(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitTags(%q) = %#v, want %#v", tc.in, got, tc.want)
		}
	}
}
