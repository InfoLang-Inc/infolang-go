package infolang

// The types below mirror the InfoLang gateway /v2 contract
// (https://api.infolang.ai/docs). Workspace-scoped routes return the server
// shapes verbatim; the client normalizes hits into Chunks and keeps
// every id opaque — never parse or validate id prefixes client-side.

// Chunk is a single recalled memory (normalized from the "hits" wire shape).
type Chunk struct {
	// ID is the memory id (wire: "id"). Opaque — do not parse.
	ID string `json:"id"`
	// Score is the cosine similarity (wire: "similarity").
	Score float64 `json:"score"`
	// Text is the memory text. Empty when Format "meta" was requested.
	Text string `json:"text"`
	// Tags is the comma-joined tag string; empty when the memory has no tags.
	Tags string `json:"tags,omitempty"`
	// Source is the origin label; present only on verbose recalls.
	Source string `json:"source,omitempty"`
	// Timestamp is epoch seconds; present only on verbose recalls.
	Timestamp float64 `json:"timestamp,omitempty"`
}

// Metering is usage metadata parsed from gateway response headers. Fields are
// pointers so an absent header is distinguishable from a zero value.
type Metering struct {
	TokensSaved  *int
	ChunksUsed   *int
	RepoCoverage *float64
	// Overage is the overage units incurred by this call, when the plan is
	// over its inclusion (X-InfoLang-Overage).
	Overage   *float64
	RequestID string
}

// weakScoreFloor is the confidence threshold below which the top recall match is
// considered weak, matching the other InfoLang SDKs.
const weakScoreFloor = 0.85

// RecallResult is the result of a Recall or Investigate call.
type RecallResult struct {
	Chunks    []Chunk `json:"chunks"`
	Namespace string  `json:"namespace"`
	// LatencyMs is the server-observed latency in milliseconds, when reported.
	LatencyMs float64   `json:"latency_ms,omitempty"`
	Metering  *Metering `json:"-"`
}

// Weak reports whether the top match scored below the 0.85 confidence floor.
// It returns false when there are no chunks.
func (r *RecallResult) Weak() bool {
	if len(r.Chunks) == 0 {
		return false
	}
	return r.Chunks[0].Score < weakScoreFloor
}

// RememberResult is the result of a Remember or RememberBatch item (server
// shape).
type RememberResult struct {
	MemoryID  string `json:"id"`
	Namespace string `json:"namespace"`
	// Stored is false when the write was absorbed by a near-duplicate (never
	// billed).
	Stored bool `json:"stored"`
	// Deduplicated is true only on a dedup absorb.
	Deduplicated bool `json:"deduplicated,omitempty"`
	// DedupedAgainst is the id of the existing memory that absorbed this write.
	DedupedAgainst string `json:"deduped_against,omitempty"`
	TotalMemories  int    `json:"total_memories"`
}

// MemoryItem is one memory row from List (gateway MemoryPage item).
type MemoryItem struct {
	ID        string   `json:"id"`
	Text      string   `json:"text"`
	Namespace string   `json:"namespace"`
	Source    string   `json:"source,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Timestamp string   `json:"timestamp,omitempty"`
	// Score is the relevance score; present only on search (Query) results.
	Score *float64 `json:"score,omitempty"`
}

// MemoryPage is a page of memories; NextCursor is empty on the last page.
type MemoryPage struct {
	Memories   []MemoryItem `json:"memories"`
	NextCursor string       `json:"nextCursor,omitempty"`
}

// NamespaceInfo is a namespace with its logical memory count and chunk row
// count. Counts are zero when the gateway reports the namespace as a bare
// string.
type NamespaceInfo struct {
	Namespace string `json:"namespace"`
	Memories  int    `json:"memories,omitempty"`
	Chunks    int    `json:"chunks,omitempty"`
}

// WhoamiWorkspace is a workspace grant visible to the current credential.
type WhoamiWorkspace struct {
	// WorkspaceID is opaque — never parse it client-side.
	WorkspaceID string `json:"workspace_id"`
	Role        string `json:"role"`
	// Scopes are the scopes that actually work on this workspace, when
	// scoping is active.
	Scopes []string `json:"scopes,omitempty"`
}

// Whoami is the identity echo for the current credential (GET /v2/whoami).
// Raw carries the full decoded body: the gateway adds fields (scope sources,
// credential descriptors) without notice.
type Whoami struct {
	Lane string `json:"lane"`
	// Principal is empty when the gateway reports it as null.
	Principal  string            `json:"principal"`
	Workspaces []WhoamiWorkspace `json:"workspaces"`
	Scopes     []string          `json:"scopes,omitempty"`
	Raw        map[string]any    `json:"-"`
}

// IngestJob is the result of an ingest call; v1 of the endpoint is synchronous
// so the job returns finished. Raw carries the full decoded body.
type IngestJob struct {
	Status string         `json:"status"`
	Raw    map[string]any `json:"-"`
}

// IngestFile is one file passed to IngestFiles (multipart ingest).
type IngestFile struct {
	Name    string
	Content []byte
}

// OpError is the machine-readable error inside an OpResult.
type OpError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Capability string `json:"capability,omitempty"`
}

// OpResult is the envelope returned by Execute and Stats.
type OpResult struct {
	OK      bool           `json:"ok"`
	Payload map[string]any `json:"payload,omitempty"`
	Error   *OpError       `json:"error,omitempty"`
}

// Operation is a single entry in an Execute batch.
type Operation struct {
	Op     string         `json:"op"`
	Args   map[string]any `json:"args,omitempty"`
	Select []string       `json:"select,omitempty"`
}

// RememberItem is one entry passed to RememberBatch.
type RememberItem struct {
	Text   string   `json:"text"`
	Tags   []string `json:"tags,omitempty"`
	Source string   `json:"source,omitempty"`
}

// HealthStatus is the gateway liveness (/healthz) or readiness (/readyz)
// payload. Raw carries the full decoded body.
type HealthStatus struct {
	Status string         `json:"status"`
	Raw    map[string]any `json:"-"`
}

// --- per-call options -------------------------------------------------------

// RecallOptions tunes a Recall call. A nil *RecallOptions uses defaults.
//
// Reserved: the retrieval extras (Golden, Format, SnippetChars, Adaptive,
// Margin) are accepted today and activate in a future release.
type RecallOptions struct {
	// Namespace overrides the client's default namespace for this call.
	Namespace string
	// TopK caps the number of chunks returned. Zero omits the field so the
	// server applies its own default.
	TopK int
	// Verbose requests extra hit fields (source, timestamp) when true.
	Verbose bool
	// Golden is the token-optimal preset: adaptive top-k + snippet rendering.
	// Reserved — see the struct comment.
	Golden bool
	// Format selects hit text rendering: full | lossless | exact | snippet |
	// meta. Reserved — see the struct comment.
	Format string
	// SnippetChars is the window size in characters for Format "snippet".
	// Reserved — see the struct comment.
	SnippetChars int
	// Adaptive trims hits past a similarity cliff.
	// Reserved — see the struct comment.
	Adaptive bool
	// Margin is the similarity margin used by the adaptive cutoff.
	// Reserved — see the struct comment.
	Margin float64
}

// InvestigateOptions tunes an Investigate call. A nil *InvestigateOptions uses
// defaults (TopK 5).
type InvestigateOptions struct {
	// NamespaceHint overrides the client's default namespace for this call.
	NamespaceHint string
	// TopK caps the number of chunks returned. Zero falls back to 5.
	TopK int
}

// RememberOptions tunes a Remember or RememberBatch call.
type RememberOptions struct {
	Namespace string
	Source    string
	// Tags is a comma-separated tag list. The client always splits it into
	// the JSON array the API expects.
	Tags string
}

// ForgetOptions tunes a Forget call.
type ForgetOptions struct {
	// Namespace scopes the delete (sent as ?namespace= when set).
	Namespace string
}

// ListOptions tunes a List call.
type ListOptions struct {
	Namespace string
	// Query switches to semantic search instead of a listing; results carry
	// Score.
	Query string
	Limit int
}

// ListRecentOptions tunes a ListRecent call.
//
// Deprecated: use ListOptions with List.
type ListRecentOptions struct {
	Namespace string
	// N caps the number of memories returned (maps to the "limit" query param).
	N int
}

// IngestOptions tunes an Ingest or IngestFiles call.
type IngestOptions struct {
	// Namespace is the target namespace (default "default" server-side).
	Namespace string
	// TagPrefix is an extra tag applied to every ingested chunk.
	TagPrefix string
	// ContentType overrides the payload content type for Ingest; zip archives
	// (application/zip) are the default. Ignored by IngestFiles, which always
	// sends multipart/form-data.
	ContentType string
}
