# Changelog

All notable changes to the InfoLang Go SDK are documented here. This project
adheres to [Semantic Versioning](https://semver.org).

## [Unreleased]

## [0.3.0] - 2026-07-25

**BREAKING**: the SDK now targets the InfoLang gateway `/v2` API
(https://api.infolang.ai/docs). The legacy `/v1` endpoints this SDK
called in 0.1.x (`/v1/banks`, `/v1/context-pack`, `/v1/repos/{ns}/ingest`, the
bare-batch `/v1/execute`) no longer exist; `/v1` as a whole is deprecated.
0.1.0 was never released, so there is no semver debt — the whole surface moved
at once.

### Added
- `Whoami(ctx)` (`GET /v2/whoami`) and automatic workspace resolution: a
  credential with exactly one workspace grant needs no configuration;
  multi-workspace credentials pass `WithWorkspace` (or `INFOLANG_WORKSPACE`).
  The multi-grant error names the candidate ids; a failed resolution is never
  cached. Workspace ids are opaque strings — never parsed client-side — and
  ride the path (`/v2/workspaces/{ws}/...`), replacing the old
  `X-InfoLang-Workspace-Id` header model.
- Recall retrieval extras: `Golden`, `Format`, `SnippetChars`, `Adaptive`,
  `Margin`.
  **Reserved:** these options are accepted today and activate in a
  future release — no SDK change needed.
- `List(ctx, *ListOptions)` (gateway MemoryPage: `ns`/`q`/`limit`,
  `NextCursor`, search scores), `Namespaces(ctx)` with per-namespace
  memory/chunk counts (bare string entries tolerated).
- `Encode(ctx, text)`, `Similarity(ctx, text1, text2)`; `Stats(ctx)` and
  `Execute(ctx, []Operation)` now return the `OpResult`
  envelope (`ok`/`payload`/`error{code,message,capability}`); the execute
  body is the simple `{"operations"}` batch.
- `Ingest(ctx, archive, *IngestOptions)`: zip archives via
  `POST /v2/workspaces/{ws}/ingest` (`Content-Type: application/zip`).
- `IngestFiles(ctx, []IngestFile, *IngestOptions)`: multipart upload of
  individual text files (repeated `files` parts) on the same ingest endpoint —
  no zip step needed.
- `Ready(ctx)` (`GET /readyz`) — the authoritative readiness signal;
  `Health(ctx)` now probes `GET /healthz`.
- Remember response fields surfaced: `Stored`, `Deduplicated`,
  `DedupedAgainst`, `TotalMemories`; `X-InfoLang-Overage` surfaced as
  `Metering.Overage`; `{"error":{code,message}}` envelopes render as "code: message" and
  surface the machine-readable `APIError.Code`.

### Changed
- `Forget` sends the namespace
  (`DELETE /v2/.../memories/{id}?namespace=`) via `ForgetOptions.Namespace`.
- `RememberBatch` is client-side sugar over `Execute` with one real
  `remember` sub-op per item (the `remember_batch` pseudo-op is gone); an
  empty input short-circuits without a request.
- Comma-separated `Tags` strings are always split into the JSON array the
  API expects.
- The base URL always defaults to `https://api.infolang.ai`; dev keys no
  longer flip the default to the direct endpoint.
- CLI: `banks` retired in favor of `namespaces` (memory/chunk counts); new
  `whoami` command; `health` reworked to probe `/healthz` + `/readyz`;
  `forget` gains `--namespace`; `remember --tags a,b` is sent as a JSON
  array; all commands ride the new workspace resolution
  (`--workspace` / `INFOLANG_WORKSPACE`, optional for single-grant
  credentials).
- `lane_not_supported` is now **403** (was 401); both map to
  `ErrAuthentication`, so no code change is needed by callers.
- Multi-workspace API keys now get their full workspace list from
  `GET /v2/whoami`, so automatic workspace resolution's multi-grant error
  can name the candidate ids.

### Removed
- `ListBanks`, `ContextPack`, `IngestRepo` and their types — their endpoints
  no longer exist. `ListRecent` and `DirectBaseURL` remain as deprecated
  shims; `WithDevKey` stays functional but deprecated.

## [0.1.0] - Unreleased

### Added
- Initial (unreleased) Go client and CLI over the legacy `/v1`
  REST API: recall/investigate/remember/forget/banks/recent/context-pack/
  execute/stats/health, typed errors, retries with jitter, metering headers.
