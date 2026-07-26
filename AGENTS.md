# infolang-go — agent instructions

Official **Go SDK + CLI** for InfoLang semantic memory. Wraps the InfoLang
gateway `/v2` API. Module: `github.com/InfoLang-Inc/infolang-go`; CLI binary:
`infolang`. Standard library only — no third-party dependencies.

## Architecture

- `client.go` — `Client`, functional `Option`s, credential/base-URL/namespace resolution.
- `workspace.go` — `Whoami` + lazy workspace resolution (`/v2/workspaces/{ws}/...` path scoping).
- `transport.go` — `net/http` transport: retries (429 + 5xx), backoff with full jitter, error mapping, metering headers, raw-body sends.
- `auth.go` — `apiKeyAuth` (managed cloud) and the deprecated `devKeyAuth` (`key:namespace`).
- `memory.go` / `ops.go` / `ingest.go` / `health.go` — the API surface (recall, investigate, remember, remember batch, forget, list, namespaces, encode, similarity, execute, stats, ingest, healthz/readyz).
- `types.go` / `errors.go` — typed results and the error hierarchy (`*APIError` + `errors.Is` sentinels).
- `cmd/infolang` + `internal/cli` — the CLI; `internal/cli.Run` holds all logic and is unit-tested.

## Contract

The gateway `/v2` REST contract (https://api.infolang.ai/docs) is the source of
truth. Verify request/response shapes against it, never against assumptions.

## Rules

- Keep the SDK dependency-free; the CLI uses the stdlib `flag` package.
- New endpoints: add the request builder + typed parser, the client method, and
  a table-driven test with an `httptest` mock server.
- Tests must stay offline by default; the live probe is gated by `INFOLANG_LIVE_TEST`.

## Commands

```bash
go build ./...
go vet ./...
go test -race -cover ./...
```
