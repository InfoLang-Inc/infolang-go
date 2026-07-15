package infolang

// The types below mirror the il-runtime REST contract (openapi/il-runtime.yaml).
// The runtime emits compact keys (i, s, t, g) on some routes and descriptive
// keys (id, text, tags, similarity) on others; the client normalizes both onto
// the readable fields exposed here.

// Chunk is a single recalled memory chunk.
type Chunk struct {
	// ID is the memory id (wire: "i" or "id").
	ID string `json:"id"`
	// Score is the similarity score (wire: "s" or "similarity").
	Score float64 `json:"score"`
	// Text is the chunk text (wire: "t" or "text").
	Text string `json:"text"`
	// Tags is the comma-joined tag string (wire: "g" or "tags").
	Tags string `json:"tags"`
}

// Savings is token-savings telemetry attached to a recall response. Fields are
// pointers so an absent value is distinguishable from a zero value.
type Savings struct {
	BaselineTokens      *int     `json:"baseline_tokens,omitempty"`
	ReturnedTokens      *int     `json:"returned_tokens,omitempty"`
	TokensSaved         *int     `json:"tokens_saved,omitempty"`
	BaselineMethod      string   `json:"baseline_method,omitempty"`
	SimilarityThreshold *float64 `json:"similarity_threshold,omitempty"`
	WeakChunks          *int     `json:"weak_chunks,omitempty"`
	LastModel           *string  `json:"last_model,omitempty"`
}

// Metering is usage metadata parsed from managed-cloud response headers.
type Metering struct {
	TokensSaved  *int
	ChunksUsed   *int
	RepoCoverage *float64
	RequestID    string
}

// weakScoreFloor is the confidence threshold below which the top recall match is
// considered weak, matching the other InfoLang SDKs.
const weakScoreFloor = 0.85

// RecallResult is the result of a Recall or Investigate call.
type RecallResult struct {
	Chunks    []Chunk   `json:"chunks"`
	Namespace string    `json:"namespace"`
	Count     int       `json:"count"`
	Savings   *Savings  `json:"savings,omitempty"`
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

// RememberResult is the result of a Remember or RememberBatch item.
type RememberResult struct {
	MemoryID      string `json:"id"`
	Namespace     string `json:"namespace"`
	Stored        bool   `json:"stored"`
	TotalMemories int    `json:"total_memories"`
}

// Bank is a memory-bank descriptor.
type Bank struct {
	Namespace     string   `json:"namespace"`
	TotalMemories int      `json:"total_memories"`
	Count         int      `json:"count"`
	ParquetSizeMB float64  `json:"parquet_size_mb"`
	SampleSources []string `json:"sample_sources"`
}

// ContextPack is the result of a ContextPack call: a token-budgeted context
// string.
type ContextPack struct {
	Pack            string    `json:"pack"`
	TokensEstimated int       `json:"tokens_estimated"`
	Namespace       string    `json:"namespace"`
	Metering        *Metering `json:"-"`
}

// HealthResponse is the runtime liveness/readiness payload.
type HealthResponse struct {
	Status      string `json:"status"`
	ModelLoaded bool   `json:"model_loaded"`
	EngineOK    bool   `json:"engine_ok"`
	DevMode     bool   `json:"dev_mode"`
}

// Operation is a single entry in an Execute batch.
type Operation struct {
	Op     string         `json:"op"`
	Args   map[string]any `json:"args,omitempty"`
	Select []string       `json:"select,omitempty"`
}

// ExecuteResult is the outcome of one operation in an Execute batch.
type ExecuteResult struct {
	Op      string         `json:"op"`
	OK      bool           `json:"ok"`
	Payload map[string]any `json:"payload"`
	Error   string         `json:"error"`
	Status  int            `json:"status"`
}

// ExecuteResponse wraps the per-operation results of an Execute batch.
type ExecuteResponse struct {
	Results []ExecuteResult `json:"results"`
}

// RememberItem is one entry passed to RememberBatch.
type RememberItem struct {
	Text   string   `json:"text"`
	Tags   []string `json:"tags,omitempty"`
	Source string   `json:"source,omitempty"`
}

// --- per-call options -------------------------------------------------------

// RecallOptions tunes a Recall call. A nil *RecallOptions uses defaults.
type RecallOptions struct {
	// Namespace overrides the client's default bank for this call.
	Namespace string
	// TopK caps the number of chunks returned. Zero omits the field so the
	// runtime applies its own default.
	TopK int
	// Filters is an optional metadata filter passed through to the runtime.
	Filters map[string]any
	// Verbose requests extra telemetry when true.
	Verbose bool
}

// InvestigateOptions tunes an Investigate call. A nil *InvestigateOptions uses
// defaults (TopK 5).
type InvestigateOptions struct {
	// NamespaceHint overrides the client's default bank for this call.
	NamespaceHint string
	// TopK caps the number of chunks returned. Zero falls back to 5.
	TopK int
}

// RememberOptions tunes a Remember or RememberBatch call.
type RememberOptions struct {
	Namespace string
	Source    string
	Tags      string
}

// ForgetOptions tunes a Forget call.
type ForgetOptions struct {
	// Namespace is reserved for future query/header passthrough; forget is
	// scoped by the credential and the workspace header.
	Namespace string
}

// ListRecentOptions tunes a ListRecent call.
type ListRecentOptions struct {
	Namespace string
	// N caps the number of memories returned (maps to the "limit" query param).
	N int
}

// ContextPackOptions tunes a ContextPack call.
type ContextPackOptions struct {
	Namespace string
	MaxTokens int
	RepoRoot  string
}
