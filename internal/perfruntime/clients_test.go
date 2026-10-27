package perfruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func clientFixture(t *testing.T) (workspace, ClientOptions) {
	t.Helper()
	o := ddevFixture(t)
	w, _ := prepared(t, o)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return w, ClientOptions{Project: o.Project, StateDirectory: o.StateDirectory, Executable: executable, Clients: []string{"codex", "cursor", "claude"}}
}

func TestClientConnectionsRoundTrip(t *testing.T) {
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	w, options := clientFixture(t)
	before := git(t, w.project, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	exclude, _ := os.ReadFile(filepath.Join(w.project, ".git/info/exclude"))
	private := []byte(`{"account":"private-sentinel","counter":9007199254740993123,"projects":{"other":{"mcpServers":{"keep":{"command":"keep"}}}}}`)
	write(t, filepath.Join(claude, ".claude.json"), private)
	first, err := Connect(context.Background(), options)
	if err != nil || len(first) != 3 {
		t.Fatalf("%+v %v", first, err)
	}
	for _, result := range first {
		if !result.Changed || !result.Owned {
			t.Fatalf("%+v", result)
		}
		data, err := os.ReadFile(result.Path)
		if err != nil || !bytes.Contains(data, []byte("performance")) || !bytes.Contains(data, []byte(w.state.Producer)) {
			t.Fatalf("bad generated settings: %s %v", result.Client, err)
		}
		var config map[string]any
		if result.Client == "codex" {
			err = toml.Unmarshal(data, &config)
		} else {
			err = strictClientJSON(data, &config)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(before, git(t, w.project, "status", "--porcelain=v1", "-z", "--untracked-files=all")) {
		t.Fatal("connecting changed Git status")
	}
	second, err := Connect(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range second {
		if result.Changed {
			t.Fatal("repeat connect changed config")
		}
	}
	state, _ := os.ReadFile(filepath.Join(w.directory, "mcp-clients.json"))
	if bytes.Contains(state, []byte("private-sentinel")) {
		t.Fatal("ownership copied foreign secrets")
	}
	w.state.Phase = "removed"
	if err := w.save(); err != nil {
		t.Fatal(err)
	}
	if _, err := Connect(context.Background(), options); err != nil {
		t.Fatalf("preserved reports after teardown cannot connect: %v", err)
	}
	if err := os.Remove(filepath.Join(w.directory, "state.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := Disconnect(context.Background(), options); err != nil {
		t.Fatalf("disconnect should not need runtime state: %v", err)
	}
	for _, id := range []string{".codex", ".cursor"} {
		if _, err := os.Lstat(filepath.Join(w.project, id)); !os.IsNotExist(err) {
			t.Fatal("owned empty config directory remains")
		}
	}
	after, _ := os.ReadFile(filepath.Join(w.project, ".git/info/exclude"))
	if !bytes.Equal(exclude, after) {
		t.Fatal("disconnect did not restore exclusions")
	}
	var want, got map[string]any
	_ = strictClientJSON(private, &want)
	data, _ := os.ReadFile(filepath.Join(claude, ".claude.json"))
	_ = strictClientJSON(data, &got)
	if entryHash(want) != entryHash(got) {
		t.Fatal("foreign Claude settings changed")
	}
	if _, err := Disconnect(context.Background(), options); err != nil {
		t.Fatal("repeat disconnect failed", err)
	}
}

func TestClientPreservesExistingAndLaterSettings(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	w, options := clientFixture(t)
	codex := filepath.Join(w.project, ".codex/config.toml")
	if err := os.MkdirAll(filepath.Dir(codex), 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte("# personal choice\nmodel = 'example'\n[mcp_servers.other]\ncommand = 'keep'\n")
	write(t, codex, original)
	exclude := filepath.Join(w.project, ".git/info/exclude")
	data, _ := os.ReadFile(exclude)
	write(t, exclude, append(data, []byte("/.codex/config.toml\n")...))
	if _, err := Connect(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	cursor := filepath.Join(w.project, ".cursor/mcp.json")
	data, _ = os.ReadFile(cursor)
	var settings map[string]any
	_ = strictClientJSON(data, &settings)
	settings["user"] = "keep"
	data, _ = json.Marshal(settings)
	write(t, cursor, data)
	if _, err := Disconnect(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(codex)
	if !bytes.Equal(original, data) {
		t.Fatal("foreign TOML bytes or comments changed")
	}
	data, _ = os.ReadFile(cursor)
	if !bytes.Contains(data, []byte(`"keep"`)) || bytes.Contains(data, []byte("phpwarden-performance")) {
		t.Fatal("later settings were lost or owned entry remains")
	}
	if len(git(t, w.project, "status", "--porcelain=v1", "--", ".cursor/mcp.json")) != 0 {
		t.Fatal("disconnect exposed remaining local settings to Git")
	}
}

func TestClientConflictsPreflightAllSelectedClients(t *testing.T) {
	for _, scenario := range []string{"tracked", "visible", "duplicate", "null", "trailing", "symlink", "parent-symlink", "collision"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
			w, options := clientFixture(t)
			path := filepath.Join(w.project, ".cursor/mcp.json")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			data := []byte(`{"keep":true}`)
			switch scenario {
			case "duplicate":
				data = []byte(`{"mcpServers":{},"mcpServers":{}}`)
			case "null":
				data = []byte(`{"mcpServers":null}`)
			case "trailing":
				data = []byte(`{} {}`)
			case "collision":
				data = []byte(`{"mcpServers":{"phpwarden-performance":{"command":"foreign"}}}`)
			}
			write(t, path, data)
			if scenario == "tracked" {
				git(t, w.project, "add", ".cursor/mcp.json")
			} else if scenario != "visible" {
				exclude := filepath.Join(w.project, ".git/info/exclude")
				original, _ := os.ReadFile(exclude)
				write(t, exclude, append(original, []byte("/.cursor/mcp.json\n")...))
			}
			if scenario == "symlink" {
				if err := os.Rename(path, path+".real"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".real", path); err != nil {
					t.Fatal(err)
				}
			} else if scenario == "parent-symlink" {
				if err := os.Rename(filepath.Dir(path), filepath.Dir(path)+".real"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Dir(path)+".real", filepath.Dir(path)); err != nil {
					t.Fatal(err)
				}
			}
			exclude, _ := os.ReadFile(filepath.Join(w.project, ".git/info/exclude"))
			if _, err := Connect(context.Background(), options); err == nil {
				t.Fatal("conflicting client settings accepted")
			}
			if _, err := os.Lstat(filepath.Join(w.project, ".codex")); !os.IsNotExist(err) {
				t.Fatal("earlier client changed before preflight finished")
			}
			after, _ := os.ReadFile(filepath.Join(w.project, ".git/info/exclude"))
			if !bytes.Equal(exclude, after) {
				t.Fatal("preflight modified excludes")
			}
		})
	}
}

func TestClientOwnedChangesAreNotOverwritten(t *testing.T) {
	for _, id := range []string{"codex", "cursor", "claude"} {
		t.Run(id, func(t *testing.T) {
			t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
			_, options := clientFixture(t)
			options.Clients = []string{id}
			connections, err := Connect(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			name := connections[0].Path
			data, _ := os.ReadFile(name)
			data = bytes.Replace(data, []byte("--reports"), []byte("--changed"), 1)
			write(t, name, data)
			for _, manage := range []func(context.Context, ClientOptions) ([]ClientConnection, error){Connect, Disconnect} {
				if _, err := manage(context.Background(), options); err == nil {
					t.Fatal("edited owned entry was accepted")
				}
			}
			after, _ := os.ReadFile(name)
			if !bytes.Equal(data, after) {
				t.Fatal("edited owned entry was overwritten")
			}
		})
	}
}

func TestClientUpdatesExecutableAndReusesUnownedEntry(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	w, options := clientFixture(t)
	connections, err := Connect(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	wrapper := filepath.Join(t.TempDir(), "phpwarden-wrapper")
	write(t, wrapper, []byte("#!/bin/sh\nexit 0\n"))
	if err := os.Chmod(wrapper, 0700); err != nil {
		t.Fatal(err)
	}
	options.Executable = wrapper
	updated, err := Connect(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	for _, connection := range updated {
		data, _ := os.ReadFile(connection.Path)
		if !connection.Changed || !bytes.Contains(data, []byte(wrapper)) {
			t.Fatal("owned executable was not updated")
		}
	}
	if err := os.Remove(filepath.Join(w.directory, "mcp-clients.json")); err != nil {
		t.Fatal(err)
	}
	reused, err := Connect(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	for _, connection := range reused {
		if connection.Changed || connection.Owned {
			t.Fatal("foreign identical entry was adopted")
		}
	}
	if _, err := Disconnect(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	for _, connection := range connections {
		if _, err := os.Stat(connection.Path); err != nil {
			t.Fatal("foreign identical entry was removed")
		}
	}
}

func TestClientSharedClaudeSettingsConcurrentProjects(t *testing.T) {
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	_, first := clientFixture(t)
	_, second := clientFixture(t)
	first.Clients, second.Clients = []string{"claude"}, []string{"claude"}
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for _, options := range []ClientOptions{first, second} {
		wg.Go(func() { _, err := Connect(context.Background(), options); errors <- err })
	}
	wg.Wait()
	for range 2 {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(claude, ".claude.json"))
	var settings map[string]any
	if err := strictClientJSON(data, &settings); err != nil {
		t.Fatal(err)
	}
	if len(settings["projects"].(map[string]any)) != 2 {
		t.Fatal("concurrent project connection was lost")
	}
	if _, err := Disconnect(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(claude, ".claude.json"))
	if !bytes.Contains(data, []byte(second.Project)) || bytes.Contains(data, []byte(first.Project)) {
		t.Fatal("disconnect changed the other project")
	}
}

func TestClientEditRollbackAndCancellation(t *testing.T) {
	name := filepath.Join(t.TempDir(), "settings")
	write(t, name, []byte("original"))
	other := filepath.Join(t.TempDir(), "second")
	write(t, other, []byte("concurrently edited"))
	err := applyClientEdits(context.Background(), []clientEdit{
		{path: name, before: []byte("original"), after: []byte("new")},
		{path: other, before: []byte("stale"), after: []byte("bad")},
	})
	if err == nil || !strings.Contains(err.Error(), "restored") {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(name)
	if string(data) != "original" {
		t.Fatal("partial write was not rolled back")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := applyClientEdits(ctx, []clientEdit{{path: name, before: data, after: []byte("bad")}}); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestClientPendingJournalRecoversMissingWrites(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	w, options := clientFixture(t)
	connections, err := Connect(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(w.directory, "mcp-clients.json")
	data, _ := os.ReadFile(path)
	var receipt clientReceipt
	if err := strictClientJSON(data, &receipt); err != nil {
		t.Fatal(err)
	}
	for id, record := range receipt.Clients {
		record.Pending = true
		receipt.Clients[id] = record
	}
	data, _ = json.Marshal(receipt)
	write(t, path, data)
	for _, connection := range connections {
		if err := os.Remove(connection.Path); err != nil {
			t.Fatal(err)
		}
	}
	excludePath := filepath.Join(w.project, ".git/info/exclude")
	data, _ = os.ReadFile(excludePath)
	for _, record := range receipt.Clients {
		if record.ExcludeOwned {
			data = bytes.Replace(data, []byte(record.ExcludeBlock), nil, 1)
		}
	}
	write(t, excludePath, data)
	partial := options
	partial.Clients = []string{"codex"}
	if _, err := Connect(context.Background(), partial); err != nil {
		t.Fatal("partial retry cannot recover", err)
	}
	if _, err := Connect(context.Background(), options); err != nil {
		t.Fatal("pending connection cannot recover", err)
	}
	if _, err := Disconnect(context.Background(), options); err != nil {
		t.Fatal("recovered connection cannot disconnect", err)
	}
}

func TestClientNestedCheckoutAndWorktreeExclusions(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	w, options := clientFixture(t)
	git(t, w.project, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "Fixture")
	worktree := filepath.Join(t.TempDir(), "other")
	git(t, w.project, "worktree", "add", "--detach", "-q", worktree)
	nested := filepath.Join(worktree, "apps", "sample [local]")
	if err := os.MkdirAll(nested, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(nested, ".ddev"), 0700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(nested, ".ddev/config.yaml"), []byte("name: nested\nphp_version: '8.3'\n"))
	other := ddevFixture(t)
	other.Project = nested
	_, _ = prepared(t, other)
	second := options
	second.Project, second.StateDirectory = nested, other.StateDirectory
	options.Clients, second.Clients = []string{"codex", "cursor"}, []string{"codex", "cursor"}
	before := git(t, worktree, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if _, err := Connect(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	if _, err := Connect(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, git(t, worktree, "status", "--porcelain=v1", "-z", "--untracked-files=all")) {
		t.Fatal("nested worktree Git status changed")
	}
	if _, err := Disconnect(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	if len(git(t, worktree, "status", "--porcelain=v1", "--", "apps/sample [local]/.cursor/mcp.json")) != 0 {
		t.Fatal("disconnect removed another checkout's exclusion")
	}
	if _, err := Disconnect(context.Background(), second); err != nil {
		t.Fatal(err)
	}
}

func TestClientTOMLConflictsAndPrivateFiles(t *testing.T) {
	for _, scenario := range []string{"inline", "additional-owned-setting", "modified-block", "private-mode"} {
		t.Run(scenario, func(t *testing.T) {
			w, options := clientFixture(t)
			options.Clients = []string{"codex"}
			path := filepath.Join(w.project, ".codex/config.toml")
			if scenario == "inline" {
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				write(t, path, []byte("mcp_servers = {}\n"))
				exclude := filepath.Join(w.project, ".git/info/exclude")
				data, _ := os.ReadFile(exclude)
				write(t, exclude, append(data, []byte("/.codex/config.toml\n")...))
				if _, err := Connect(context.Background(), options); err == nil {
					t.Fatal("sealed inline TOML table was extended")
				}
				return
			}
			if _, err := Connect(context.Background(), options); err != nil {
				t.Fatal(err)
			}
			if scenario == "private-mode" {
				for _, file := range []string{path, filepath.Join(w.directory, "mcp-clients.json")} {
					info, err := os.Stat(file)
					if err != nil || info.Mode().Perm() != 0600 {
						t.Fatalf("generated config is not private: %s", file)
					}
				}
				return
			}
			data, _ := os.ReadFile(path)
			if scenario == "additional-owned-setting" {
				data = append(data, []byte("enabled = false\n")...)
			} else {
				data = bytes.Replace(data, []byte("# <<< phpwarden-performance <<<"), []byte("# user changed marker"), 1)
			}
			write(t, path, data)
			if _, err := Disconnect(context.Background(), options); err == nil {
				t.Fatal("modified owned table was removed")
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(data, after) {
				t.Fatal("conflicting TOML was changed")
			}
		})
	}
}
