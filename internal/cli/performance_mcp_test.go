package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rchehd/phpwarden/internal/performance"
)

func TestMCPCLIStdioWithOfficialClientHasNoHumanStdout(t *testing.T) {
	fixtures, err := filepath.Abs("../../tools/spx/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	serverRead, clientWrite := io.Pipe()
	clientRead, serverWrite := io.Pipe()
	var stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- RunContext(ctx, []string{"performance", "mcp", "--reports", fixtures, "--producer", performance.Producer}, t.TempDir(), serverRead, serverWrite, &stderr)
	}()
	client := mcp.NewClient(&mcp.Implementation{Name: "CLI-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: clientRead, Writer: clientWrite}, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_flat_profile", Arguments: map[string]any{"report_key": "repeated", "limit": 1}})
	if err != nil || result.IsError {
		t.Fatalf("%+v %v", result, err)
	}
	data, _ := json.Marshal(result.StructuredContent)
	var profile performance.QueryResult
	if json.Unmarshal(data, &profile) != nil || profile.Functions[0].Calls != 24 {
		t.Fatalf("%s", data)
	}
	session.Close()
	select {
	case code := <-done:
		if code != 0 || stderr.Len() != 0 {
			t.Fatalf("%d %s", code, &stderr)
		}
	case <-ctx.Done():
		t.Fatal("MCP CLI did not terminate on EOF")
	}
}

func TestMCPCLIOptionsOnlyUseStderrForFailures(t *testing.T) {
	for _, args := range [][]string{{}, {"--producer=bad"}, {"--format=json"}, {"--reports"}, {"--reports=a", "--reports=b"}, {"--listen=:8080"}, {"--apply"}, {"--reports=bad\npath"}} {
		var stdout, stderr bytes.Buffer
		code := RunContext(context.Background(), append([]string{"performance", "mcp"}, args...), t.TempDir(), strings.NewReader(""), &stdout, &stderr)
		if code != 2 || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("%v: %d %s %s", args, code, &stdout, &stderr)
		}
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"performance", "mcp", "--help"}, t.TempDir(), &stdout, &stderr); code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "get_callers") {
		t.Fatalf("%d %s %s", code, &stdout, &stderr)
	}
}
