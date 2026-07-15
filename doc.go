// Package infolang is the official Go client for the InfoLang semantic-memory
// runtime. It wraps the public il-runtime REST API (see openapi/il-runtime.yaml)
// with idiomatic, context.Context-first methods and typed results.
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
// The client is safe for concurrent use by multiple goroutines. It has no
// external dependencies beyond the Go standard library.
package infolang
