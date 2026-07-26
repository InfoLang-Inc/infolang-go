# infolang-go

Official Go client and CLI for [InfoLang](https://infolang.ai) semantic memory.
It wraps the InfoLang gateway `/v2` API with an idiomatic, `context.Context`-first
client, typed errors, automatic retries, and a first-party `infolang` CLI.

- Zero external dependencies (standard library only).
- Module: `github.com/InfoLang-Inc/infolang-go`.
- CLI binary: `infolang` (Homebrew tap + `go install`).

## Install

Library:

```bash
go get github.com/InfoLang-Inc/infolang-go
```

CLI (from source):

```bash
go install github.com/InfoLang-Inc/infolang-go/cmd/infolang@latest
```

CLI (Homebrew, once published):

```bash
brew install InfoLang-Inc/tap/infolang
```

## Quickstart

```go
package main

import (
	"context"
	"fmt"
	"log"

	infolang "github.com/InfoLang-Inc/infolang-go"
)

func main() {
	client, err := infolang.New("il_live_...") // or set INFOLANG_API_KEY and call infolang.New("")
	if err != nil {
		log.Fatal(err)
	}

	res, err := client.Investigate(context.Background(), "how does auth middleware work?", nil)
	if err != nil {
		log.Fatal(err)
	}
	for _, chunk := range res.Chunks {
		fmt.Printf("%.3f  %s\n", chunk.Score, chunk.Text)
	}
}
```

## Workspaces

Every call operates in a workspace (`/v2/workspaces/{ws}/...`). A credential
with exactly one workspace grant needs no configuration — the client resolves
the workspace via `GET /v2/whoami` on first use and caches it.
Multi-workspace credentials pick one explicitly:

```go
client, _ := infolang.New("il_live_...",
	infolang.WithNamespace("docs"),  // default namespace
	infolang.WithWorkspace("<id>"),  // opaque workspace id, from the Console or Whoami
)
```

`Whoami(ctx)` returns the credential's lane, principal, and workspace grants;
`WorkspaceID(ctx)` returns the id every call will use.

## Authentication

Managed-cloud API keys (`infolang.New("il_live_...")`) target
`api.infolang.ai`. Legacy dev keys (`infolang.WithDevKey("key:namespace")`)
remain functional but deprecated; they pin their namespace, and the base URL
always defaults to the cloud gateway.

Credentials and scoping are also read from the environment when the corresponding
option is empty: `INFOLANG_API_KEY`, `INFOLANG_DEV_KEY`, `INFOLANG_BASE_URL`,
`INFOLANG_NAMESPACE`, `INFOLANG_WORKSPACE` (or `INFOLANG_WORKSPACE_ID`).

## Core API

Every method takes a `context.Context` first and a nilable `*Options` last.

| Method | Purpose |
|--------|---------|
| `Recall(ctx, query, *RecallOptions)` | Semantic recall |
| `Investigate(ctx, query, *InvestigateOptions)` | Agent-style recall (defaults to top-k 5) |
| `Remember(ctx, text, *RememberOptions)` | Store a memory |
| `RememberBatch(ctx, []RememberItem, *RememberOptions)` | Store many memories in one round-trip |
| `Forget(ctx, memoryID, *ForgetOptions)` | Delete a memory by id (optionally namespace-scoped) |
| `List(ctx, *ListOptions)` | Page memories (`ns`/`q`/`limit`, cursors, search scores) |
| `Namespaces(ctx)` | Namespaces with memory/chunk counts |
| `Whoami(ctx)` / `WorkspaceID(ctx)` | Credential identity and workspace resolution |
| `Encode(ctx, text)` / `Similarity(ctx, a, b)` | Embedding primitives |
| `Execute(ctx, []Operation)` | Batch primitives in one round-trip (native `OpResult`) |
| `Stats(ctx)` | Server stats for the workspace (native `OpResult`) |
| `Ingest(ctx, zipBytes, *IngestOptions)` | Ingest a zip of text content |
| `IngestFiles(ctx, []IngestFile, *IngestOptions)` | Multipart ingest of individual files |
| `Health(ctx)` / `Ready(ctx)` | Gateway liveness (`/healthz`) and readiness (`/readyz`) |

`RecallResult.Weak()` reports whether the top match scored below the 0.85
confidence floor. Recall chunks carry `ID`, `Text`, `Score`, `Tags`, and (on
verbose recalls) `Source` and `Timestamp`.

The recall retrieval extras (`Golden`, `Format`, `SnippetChars`, `Adaptive`,
`Margin`) are **reserved**: accepted today and activated in a future release.

### Batch remember

```go
items := []infolang.RememberItem{
	{Text: "Alice moved to Berlin in March 2024.", Tags: []string{"alice", "2024"}},
	{Text: "Bob's flight is on the 12th.", Tags: []string{"bob"}},
}
results, err := client.RememberBatch(ctx, items, &infolang.RememberOptions{Namespace: "eval"})
```

`RememberBatch` is client-side sugar over `Execute` with one real `remember`
sub-op per item — one round-trip instead of N separate remembers.

## CLI

```
infolang <command> [flags] [args]

Commands:
  recall       <query>   Semantic recall
  investigate  <query>   Agent-style recall (top-k 5)
  remember     <text>    Store a memory
  forget       <id>      Delete a memory by id (--namespace to scope)
  namespaces             List namespaces with memory/chunk counts
  whoami                 Credential identity and workspace grants
  stats                  Server stats for the workspace
  health                 Gateway liveness (/healthz) and readiness (/readyz)
  version                Print the client version
```

Common flags: `--api-key`, `--dev-key`, `--namespace`, `--workspace`,
`--base-url`, `--timeout`, `--json`. Recall/investigate add `--top-k`; remember
adds `--source` and `--tags` (comma-separated, sent as a JSON array).
`--workspace` is optional for credentials with exactly one workspace grant.

```bash
export INFOLANG_API_KEY=il_live_...
infolang recall "auth middleware" --top-k 5
infolang remember "a fact worth keeping" --source docs/auth.md --tags a,b
infolang namespaces --json
```

## Errors

Every failure is a typed error. API failures are `*APIError` (with `StatusCode`,
`Code`, `Body`, `RequestID`, `RetryAfter`); classify them with `errors.Is`:

```go
_, err := client.Recall(ctx, "q", nil)
switch {
case errors.Is(err, infolang.ErrAuthentication): // 401/403 (incl. lane_not_supported)
case errors.Is(err, infolang.ErrNotFound):       // 404
case errors.Is(err, infolang.ErrValidation):     // 400/422
case errors.Is(err, infolang.ErrRateLimit):      // 429
case errors.Is(err, infolang.ErrServer):         // 5xx
}
```

Gateway `{"error":{code,message}}` envelopes render as `code: message` and
surface the machine-readable code on `APIError.Code`. Transport failures are
`*ConnectionError`; misconfiguration (including workspace resolution) is
`*ConfigError`.

## Resilience

`429` and `5xx` responses (and transient transport errors) are retried with
exponential backoff plus full jitter, honoring `Retry-After`. Tune with
`WithMaxRetries` and `WithTimeout`, or supply your own client with
`WithHTTPClient`.

## Development

```bash
go build ./...
go vet ./...
go test -race -cover ./...
```

The gateway `/v2` API is documented at https://api.infolang.ai/docs. A live
smoke test is available and skipped by default:

```bash
INFOLANG_LIVE_TEST=1 INFOLANG_API_KEY=il_live_... go test -run TestLiveProbe ./...
```

## License

Apache-2.0. See [LICENSE](LICENSE).
