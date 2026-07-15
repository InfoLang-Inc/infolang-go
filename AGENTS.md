# infolang-go — agent instructions

Official **Go SDK + CLI** for InfoLang semantic memory. Wraps the public
`il-runtime` REST API. Module: `github.com/InfoLang-Inc/infolang-go`; CLI binary:
`infolang`. Standard library only — no third-party dependencies.

## Architecture

- `client.go` — `Client`, functional `Option`s, credential/base-URL/namespace/workspace resolution.
- `transport.go` — `net/http` transport: retries (429 + 5xx), backoff with full jitter, error mapping, metering headers.
- `auth.go` — `apiKeyAuth` (managed cloud) and `devKeyAuth` (`key:namespace`, self-hosted).
- `memory.go` / `context.go` / `health.go` — the API surface (recall, investigate, remember, remember_batch, forget, banks, recent, context-pack, ingest, execute, stats, health).
- `types.go` / `errors.go` — typed results and the error hierarchy (`*APIError` + `errors.Is` sentinels).
- `cmd/infolang` + `internal/cli` — the CLI; `internal/cli.Run` holds all logic and is unit-tested.

## Contract

The REST contract is the source of truth in `infolang-runtime`
(`openapi/il-runtime.yaml`). This repo pins a copy under `openapi/`; the pinned
version is in `openapi/IL_RUNTIME_VERSION`. Verify request/response shapes against
that file, never against engine internals.

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
