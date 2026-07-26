// Package infolang is the official Go client for InfoLang semantic memory. It
// wraps the InfoLang gateway /v2 API (https://api.infolang.ai/docs) with
// idiomatic, context.Context-first methods and typed results.
//
// Construct a client from an API key and call the memory operations:
//
//	client, err := infolang.New("il_live_...")
//	if err != nil {
//		log.Fatal(err)
//	}
//	res, err := client.Investigate(ctx, "how does auth middleware work?", nil)
//	for _, chunk := range res.Chunks {
//		fmt.Println(chunk.Score, chunk.Text)
//	}
//
// Every call operates in a workspace. A credential with exactly one workspace
// grant needs no configuration — the client resolves the workspace via
// GET /v2/whoami on first use. Multi-workspace credentials pass WithWorkspace
// (or set INFOLANG_WORKSPACE). Workspace ids are opaque strings.
//
// The client is safe for concurrent use by multiple goroutines. It has no
// external dependencies beyond the Go standard library.
package infolang
