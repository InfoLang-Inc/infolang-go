package infolang

import (
	"context"
	"net/http"
	"testing"
)

func TestContextPack(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("x-infolang-tokens-saved", "120")
		writeJSON(w, 200, map[string]any{
			"pack": "packed context", "tokens_estimated": 200, "namespace": "docs",
		})
	})
	c := newTestClient(t, ms.URL, WithNamespace("docs"))
	pack, err := c.ContextPack(context.Background(), "how does auth work", &ContextPackOptions{MaxTokens: 500, RepoRoot: "/repo"})
	if err != nil {
		t.Fatalf("ContextPack: %v", err)
	}
	if pack.Pack != "packed context" || pack.TokensEstimated != 200 {
		t.Errorf("unexpected pack: %+v", pack)
	}
	if pack.Metering == nil || pack.Metering.TokensSaved == nil || *pack.Metering.TokensSaved != 120 {
		t.Errorf("metering not parsed: %+v", pack.Metering)
	}
	if ms.last.Body["max_tokens"].(float64) != 500 || ms.last.Body["repo_root"] != "/repo" ||
		ms.last.Body["namespace"] != "docs" {
		t.Errorf("unexpected body: %v", ms.last.Body)
	}
}

func TestContextPackNilOpts(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"pack": "x"})
	})
	c := newTestClient(t, ms.URL)
	if _, err := c.ContextPack(context.Background(), "q", nil); err != nil {
		t.Fatalf("ContextPack: %v", err)
	}
	if _, ok := ms.last.Body["max_tokens"]; ok {
		t.Error("max_tokens should be omitted")
	}
}

func TestIngestRepo(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{"accepted": true})
	})
	c := newTestClient(t, ms.URL)
	out, err := c.IngestRepo(context.Background(), "my ns", "/root", "main")
	if err != nil {
		t.Fatalf("IngestRepo: %v", err)
	}
	if out["accepted"] != true {
		t.Errorf("unexpected out: %v", out)
	}
	if ms.last.EscapedPath != "/v1/repos/my%20ns/ingest" {
		t.Errorf("escaped path = %q", ms.last.EscapedPath)
	}
	if ms.last.Body["repo_root"] != "/root" || ms.last.Body["ref"] != "main" {
		t.Errorf("unexpected body: %v", ms.last.Body)
	}
}

func TestIngestRepoNonObject(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, "ok")
	})
	c := newTestClient(t, ms.URL)
	out, err := c.IngestRepo(context.Background(), "ns", "", "")
	if err != nil {
		t.Fatalf("IngestRepo: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("want empty map, got %v", out)
	}
}

func TestExecute(t *testing.T) {
	ms := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, map[string]any{
			"results": []map[string]any{
				{"op": "recall", "ok": true, "status": 200, "payload": map[string]any{"n": 1}},
				{"op": "bad", "ok": false, "error": "boom", "status": 400},
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
	if len(out.Results) != 2 {
		t.Fatalf("want 2 results, got %d", len(out.Results))
	}
	if !out.Results[0].OK || out.Results[1].Error != "boom" {
		t.Errorf("unexpected results: %+v", out.Results)
	}
	ops := ms.last.Body["operations"].([]any)
	op := ops[0].(map[string]any)
	if op["op"] != "recall" {
		t.Errorf("op = %v", op["op"])
	}
}
