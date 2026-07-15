# infolang-go

Official Go client and CLI for [InfoLang](https://infolang.ai) semantic memory.
It wraps the public `il-runtime` REST API with an idiomatic, `context.Context`-first
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

Scoping (workspace = tenant, namespace = bank):

```go
client, _ := infolang.New("il_live_...",
	infolang.WithNamespace("docs"),   // default memory bank
	infolang.WithWorkspace("<uuid>"), // sent as X-InfoLang-Workspace-Id
)
```

## Authentication

| Mode | Construction | Default target |
|------|--------------|----------------|
| Managed cloud (API key) | `infolang.New("il_live_...")` | `api.infolang.ai` |
| Self-hosted dev key | `infolang.New("", infolang.WithDevKey("key:namespace"))` | `127.0.0.1:8766` |

A managed API key honors the client `namespace` on both reads and writes. A dev
key is pinned to the namespace embedded in `key:namespace`.

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
| `Forget(ctx, memoryID, *ForgetOptions)` | Delete a memory by id |
| `ListBanks(ctx)` / `ListRecent(ctx, *ListRecentOptions)` | Introspection |
| `ContextPack(ctx, query, *ContextPackOptions)` | One-shot token-budgeted context string |
| `IngestRepo(ctx, namespace, repoRoot, ref)` | Index a repository |
| `Execute(ctx, []Operation)` | Run a batch of operations |
| `Stats(ctx)` | Namespace/store stats |
| `Health(ctx)` | Liveness/readiness |

`RecallResult.Weak()` reports whether the top match scored below the 0.85
confidence floor. Recall chunks carry `ID`, `Text`, `Score`, and `Tags`.

### Batch remember

```go
items := []infolang.RememberItem{
	{Text: "Alice moved to Berlin in March 2024.", Tags: []string{"alice", "2024"}},
	{Text: "Bob's flight is on the 12th.", Tags: []string{"bob"}},
}
results, err := client.RememberBatch(ctx, items, &infolang.RememberOptions{Namespace: "eval"})
```

`RememberBatch` issues a single `remember_batch` op on `/v1/execute` — one encode
and one write instead of N separate remembers.

## CLI

```
infolang <command> [flags] [args]

Commands:
  recall       <query>   Semantic recall
  investigate  <query>   Agent-style recall (top-k 5)
  remember     <text>    Store a memory
  forget       <id>      Delete a memory by id
  banks                  List memory banks
  stats                  Namespace/store stats
  health                 Runtime liveness/readiness
  version                Print the client version
```

Common flags: `--api-key`, `--dev-key`, `--namespace`, `--workspace`,
`--base-url`, `--timeout`, `--json`. Recall/investigate add `--top-k`; remember
adds `--source` and `--tags`.

```bash
export INFOLANG_API_KEY=il_live_...
infolang recall "auth middleware" --top-k 5
infolang remember "a fact worth keeping" --source docs/auth.md --tags a,b
infolang banks --json
```

## Errors

Every failure is a typed error. API failures are `*APIError` (with `StatusCode`,
`Body`, `RequestID`, `RetryAfter`); classify them with `errors.Is`:

```go
_, err := client.Recall(ctx, "q", nil)
switch {
case errors.Is(err, infolang.ErrAuthentication): // 401/403
case errors.Is(err, infolang.ErrNotFound):       // 404
case errors.Is(err, infolang.ErrValidation):     // 400/422
case errors.Is(err, infolang.ErrRateLimit):      // 429
case errors.Is(err, infolang.ErrServer):         // 5xx
}
```

Transport failures are `*ConnectionError`; misconfiguration is `*ConfigError`.

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

The REST contract is pinned under `openapi/` (see `openapi/IL_RUNTIME_VERSION`).
A live smoke test is available and skipped by default:

```bash
INFOLANG_LIVE_TEST=1 INFOLANG_API_KEY=il_live_... go test -run TestLiveProbe ./...
```

## License

Apache-2.0. See [LICENSE](LICENSE).
