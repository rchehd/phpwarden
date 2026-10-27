package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/rchehd/phpwarden/internal/perfmcp"
	"github.com/rchehd/phpwarden/internal/performance"
)

type writeCloser struct{ io.Writer }

func (writeCloser) Close() error { return nil }

func performanceMCPCLI(ctx context.Context, args []string, cwd string, input io.Reader, stdout, stderr io.Writer) int {
	options := performance.Options{}
	seen := map[string]bool{}
	invalid := func(message string) int { fmt.Fprintln(stderr, "phpwarden performance mcp: "+message); return 2 }
	for i := 0; i < len(args); i++ {
		name, value, inline := strings.Cut(args[i], "=")
		if name == "--help" || name == "-h" {
			fmt.Fprintf(stdout, "Usage: phpwarden performance mcp --reports DIR --producer COMMIT [options]\n\nServe validated SPX captures over MCP stdio. No PHP, Docker, network or project execution.\n\n  --reports DIR         Fixed reports directory visible to tools\n  --producer COMMIT     Supported producer: %s\n  --report KEY          Restrict this server to one capture\n  --project DIR         Optional source workspace\n  --container-root DIR  Corresponding captured source root\n\nTools: find_reports, get_report_metadata, get_flat_profile, get_callers,\n       get_aggregated_call_graph. All are read-only.\n\nstdout is JSON-RPC only; diagnostics go to stderr.\nNo client settings, hooks or runtime configuration are changed.\n", performance.Producer)
			return 0
		}
		switch name {
		case "--reports", "--producer", "--report", "--project", "--container-root":
		default:
			return invalid("unknown option; see --help")
		}
		if seen[name] {
			return invalid("duplicate option")
		}
		seen[name] = true
		if !inline {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return invalid("option requires a value")
			}
			i++
			value = args[i]
		}
		if value == "" || strings.ContainsAny(value, "\x00\r\n") {
			return invalid("option requires a nonempty single-line value")
		}
		switch name {
		case "--reports":
			options.Directory = value
		case "--producer":
			options.Producer = value
		case "--report":
			options.Key = value
		case "--project":
			options.Workspace = value
		case "--container-root":
			options.ContainerRoot = value
		}
	}
	for _, value := range []*string{&options.Directory, &options.Workspace} {
		if *value != "" && !filepath.IsAbs(*value) {
			*value = filepath.Join(cwd, *value)
		}
	}
	server, err := perfmcp.New(options)
	if err != nil {
		return invalid(err.Error())
	}
	defer server.Close()
	rc, ok := input.(io.ReadCloser)
	if !ok {
		rc = io.NopCloser(input)
	}
	wc, ok := stdout.(io.WriteCloser)
	if !ok {
		wc = writeCloser{stdout}
	}
	if err := server.Serve(ctx, rc, wc); err != nil {
		if ctx.Err() == context.Canceled {
			return 130
		}
		fmt.Fprintln(stderr, "PHPWarden MCP session failed; check client protocol, session limits and report availability.")
		return 3
	}
	return 0
}
