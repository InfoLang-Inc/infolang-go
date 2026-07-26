// Package cli implements the `infolang` command-line interface over the
// InfoLang Go SDK. Run is the single entry point; it is written to be testable
// (explicit args and writers, an int exit code, no os.Exit).
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	infolang "github.com/InfoLang-Inc/infolang-go"
)

const usage = `infolang - InfoLang semantic memory CLI

Usage:
  infolang <command> [flags] [args]

Commands:
  recall       <query>   Semantic recall from a namespace
  investigate  <query>   Agent-style recall (defaults to top-k 5)
  remember     <text>    Store a memory
  forget       <id>      Delete a memory by id (--namespace to scope)
  namespaces             List namespaces with memory/chunk counts
  whoami                 Show credential identity and workspace grants
  stats                  Show server stats for the workspace
  health                 Gateway liveness (/healthz) and readiness (/readyz)
  version                Print the client version

Common flags:
  --api-key string     Managed-cloud API key (or $INFOLANG_API_KEY)
  --dev-key string     Legacy dev key "key:namespace" (or $INFOLANG_DEV_KEY)
  --namespace string   Namespace (or $INFOLANG_NAMESPACE)
  --workspace string   Workspace id (or $INFOLANG_WORKSPACE); optional for
                       credentials with exactly one workspace grant
  --base-url string    Gateway endpoint (or $INFOLANG_BASE_URL)
  --timeout duration   Per-request timeout (default 30s)
  --json               Emit JSON instead of human-readable output

Examples:
  infolang recall "auth middleware" --top-k 5
  infolang remember "a fact worth keeping" --source docs/auth.md --tags a,b
  INFOLANG_API_KEY=il_live_... infolang namespaces --json
`

// commonFlags holds the credential/scoping/output flags shared by every command.
type commonFlags struct {
	apiKey    string
	devKey    string
	namespace string
	workspace string
	baseURL   string
	timeout   time.Duration
	jsonOut   bool
}

func registerCommon(fs *flag.FlagSet) *commonFlags {
	c := &commonFlags{}
	fs.StringVar(&c.apiKey, "api-key", "", "managed-cloud API key (or $INFOLANG_API_KEY)")
	fs.StringVar(&c.devKey, "dev-key", "", "legacy dev key key:namespace (or $INFOLANG_DEV_KEY)")
	fs.StringVar(&c.namespace, "namespace", "", "namespace (or $INFOLANG_NAMESPACE)")
	fs.StringVar(&c.workspace, "workspace", "", "workspace id (or $INFOLANG_WORKSPACE)")
	fs.StringVar(&c.baseURL, "base-url", "", "gateway endpoint (or $INFOLANG_BASE_URL)")
	fs.DurationVar(&c.timeout, "timeout", 30*time.Second, "per-request timeout")
	fs.BoolVar(&c.jsonOut, "json", false, "emit JSON output")
	return c
}

func (c *commonFlags) newClient() (*infolang.Client, error) {
	opts := []infolang.Option{infolang.WithTimeout(c.timeout)}
	if c.devKey != "" {
		opts = append(opts, infolang.WithDevKey(c.devKey))
	}
	if c.namespace != "" {
		opts = append(opts, infolang.WithNamespace(c.namespace))
	}
	if c.workspace != "" {
		opts = append(opts, infolang.WithWorkspace(c.workspace))
	}
	if c.baseURL != "" {
		opts = append(opts, infolang.WithBaseURL(c.baseURL))
	}
	return infolang.New(c.apiKey, opts...)
}

// Run parses args (excluding the program name) and executes a command, writing
// to stdout/stderr. It returns a process exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	cmd, rest := args[0], args[1:]

	switch cmd {
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	case "version", "--version":
		fmt.Fprintln(stdout, infolang.Version)
		return 0
	case "recall":
		return runRecall(ctx, rest, stdout, stderr, false)
	case "investigate":
		return runRecall(ctx, rest, stdout, stderr, true)
	case "remember":
		return runRemember(ctx, rest, stdout, stderr)
	case "forget":
		return runForget(ctx, rest, stdout, stderr)
	case "namespaces":
		return runNamespaces(ctx, rest, stdout, stderr)
	case "whoami":
		return runWhoami(ctx, rest, stdout, stderr)
	case "stats":
		return runStats(ctx, rest, stdout, stderr)
	case "health":
		return runHealth(ctx, rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", cmd, usage)
		return 2
	}
}

// parse wires common flags plus a per-command customizer, then parses args.
func parse(name string, args []string, stderr io.Writer, extra func(*flag.FlagSet)) (*flag.FlagSet, *commonFlags, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	common := registerCommon(fs)
	if extra != nil {
		extra(fs)
	}
	if err := fs.Parse(args); err != nil {
		return nil, nil, err
	}
	return fs, common, nil
}

func runRecall(ctx context.Context, args []string, stdout, stderr io.Writer, investigate bool) int {
	var topK int
	fs, common, err := parse("recall", args, stderr, func(fs *flag.FlagSet) {
		fs.IntVar(&topK, "top-k", 0, "maximum chunks to return")
	})
	if err != nil {
		return 2
	}
	query := strings.Join(fs.Args(), " ")
	if query == "" {
		fmt.Fprintln(stderr, "error: a query argument is required")
		return 2
	}
	client, err := common.newClient()
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}

	var res *infolang.RecallResult
	if investigate {
		res, err = client.Investigate(ctx, query, &infolang.InvestigateOptions{TopK: topK})
	} else {
		res, err = client.Recall(ctx, query, &infolang.RecallOptions{TopK: topK})
	}
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	if common.jsonOut {
		return emitJSON(stdout, stderr, res)
	}
	weak := ""
	if res.Weak() {
		weak = " (weak match)"
	}
	fmt.Fprintf(stdout, "%d chunk(s)%s\n", len(res.Chunks), weak)
	for _, ch := range res.Chunks {
		tags := ""
		if ch.Tags != "" {
			tags = "  [" + ch.Tags + "]"
		}
		fmt.Fprintf(stdout, "\n%.3f  %s%s\n%s\n", ch.Score, ch.ID, tags, ch.Text)
	}
	return 0
}

func runRemember(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	var source, tags string
	fs, common, err := parse("remember", args, stderr, func(fs *flag.FlagSet) {
		fs.StringVar(&source, "source", "", "source label for the memory")
		fs.StringVar(&tags, "tags", "", "comma-separated tags (sent as a JSON array)")
	})
	if err != nil {
		return 2
	}
	text := strings.Join(fs.Args(), " ")
	if text == "" {
		fmt.Fprintln(stderr, "error: text to remember is required")
		return 2
	}
	client, err := common.newClient()
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	res, err := client.Remember(ctx, text, &infolang.RememberOptions{Source: source, Tags: tags})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if common.jsonOut {
		return emitJSON(stdout, stderr, res)
	}
	if res.Deduplicated {
		fmt.Fprintf(stdout, "deduplicated against %s (namespace=%s)\n", res.DedupedAgainst, res.Namespace)
		return 0
	}
	fmt.Fprintf(stdout, "stored %s (namespace=%s, total=%d)\n", res.MemoryID, res.Namespace, res.TotalMemories)
	return 0
}

func runForget(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs, common, err := parse("forget", args, stderr, nil)
	if err != nil {
		return 2
	}
	id := strings.Join(fs.Args(), " ")
	if id == "" {
		fmt.Fprintln(stderr, "error: a memory id is required")
		return 2
	}
	client, err := common.newClient()
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	if err := client.Forget(ctx, id, &infolang.ForgetOptions{Namespace: common.namespace}); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if common.jsonOut {
		return emitJSON(stdout, stderr, map[string]any{"forgotten": id})
	}
	fmt.Fprintf(stdout, "forgotten %s\n", id)
	return 0
}

func runNamespaces(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	_, common, err := parse("namespaces", args, stderr, nil)
	if err != nil {
		return 2
	}
	client, err := common.newClient()
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	namespaces, err := client.Namespaces(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if common.jsonOut {
		return emitJSON(stdout, stderr, namespaces)
	}
	if len(namespaces) == 0 {
		fmt.Fprintln(stdout, "no namespaces")
		return 0
	}
	fmt.Fprintf(stdout, "%-24s %10s %10s\n", "NAMESPACE", "MEMORIES", "CHUNKS")
	for _, ns := range namespaces {
		fmt.Fprintf(stdout, "%-24s %10d %10d\n", ns.Namespace, ns.Memories, ns.Chunks)
	}
	return 0
}

func runWhoami(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	_, common, err := parse("whoami", args, stderr, nil)
	if err != nil {
		return 2
	}
	client, err := common.newClient()
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	who, err := client.Whoami(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if common.jsonOut {
		return emitJSON(stdout, stderr, who.Raw)
	}
	principal := who.Principal
	if principal == "" {
		principal = "-"
	}
	fmt.Fprintf(stdout, "lane=%s principal=%s\n", who.Lane, principal)
	if len(who.Workspaces) == 0 {
		fmt.Fprintln(stdout, "no workspace grants")
		return 0
	}
	fmt.Fprintln(stdout, "workspaces:")
	for _, ws := range who.Workspaces {
		fmt.Fprintf(stdout, "  %-36s role=%s\n", ws.WorkspaceID, ws.Role)
	}
	return 0
}

func runStats(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	_, common, err := parse("stats", args, stderr, nil)
	if err != nil {
		return 2
	}
	client, err := common.newClient()
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	stats, err := client.Stats(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	// Stats are free-form, so always render as JSON for fidelity.
	return emitJSON(stdout, stderr, stats)
}

func runHealth(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	_, common, err := parse("health", args, stderr, nil)
	if err != nil {
		return 2
	}
	client, err := common.newClient()
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	live, err := client.Health(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	ready, err := client.Ready(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if common.jsonOut {
		return emitJSON(stdout, stderr, map[string]any{"healthz": live.Raw, "readyz": ready.Raw})
	}
	fmt.Fprintf(stdout, "healthz=%s readyz=%s\n", live.Status, ready.Status)
	return 0
}

func emitJSON(stdout, stderr io.Writer, v any) int {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}
