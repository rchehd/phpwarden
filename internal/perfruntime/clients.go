package perfruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/agentskills"
	"github.com/rchehd/phpwarden/internal/cache"
	"github.com/rchehd/phpwarden/internal/runner"
)

type ClientOptions struct {
	Project, StateDirectory, Executable string
	Clients                             []string
}

type ClientConnection struct {
	Client  string `json:"client"`
	Path    string `json:"path"`
	Changed bool   `json:"changed"`
	Owned   bool   `json:"owned"`
}

type clientRecord struct {
	Path         string   `json:"path"`
	Owned        bool     `json:"owned"`
	Pending      bool     `json:"pending,omitempty"`
	Hashes       []string `json:"hashes,omitempty"`
	Blocks       []string `json:"blocks,omitempty"`
	CreatedFile  bool     `json:"created_file,omitempty"`
	CreatedMaps  []int    `json:"created_maps,omitempty"`
	CreatedDirs  []string `json:"created_dirs,omitempty"`
	ExcludeFile  string   `json:"exclude_file,omitempty"`
	ExcludeBlock string   `json:"exclude_block,omitempty"`
	ExcludeOwned bool     `json:"exclude_owned,omitempty"`
}

type clientReceipt struct {
	Version int                     `json:"version"`
	Project string                  `json:"project"`
	Clients map[string]clientRecord `json:"clients"`
}

type clientEdit struct {
	path          string
	before, after []byte
	remove        bool
}

func Connect(ctx context.Context, options ClientOptions) ([]ClientConnection, error) {
	return manageClients(ctx, options, true)
}

func Disconnect(ctx context.Context, options ClientOptions) ([]ClientConnection, error) {
	return manageClients(ctx, options, false)
}

func manageClients(ctx context.Context, options ClientOptions, connect bool) ([]ClientConnection, error) {
	clients, err := agentskills.ParseClients(strings.Join(options.Clients, ","))
	if err != nil || len(clients) == 0 {
		return nil, fmt.Errorf("choose --client all or a comma-separated selection of codex,cursor,claude")
	}
	w, err := locate(Options{Project: options.Project, StateDirectory: options.StateDirectory, Process: runner.Exec{}})
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(w.directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("private performance state is unavailable; run performance setup first")
	}
	unlock, err := cache.LockProject(ctx, w.directory)
	if err != nil {
		return nil, err
	}
	defer unlock()
	var entry map[string]any
	if connect {
		if err := w.load(); err != nil || w.state.Phase == "removing" {
			return nil, fmt.Errorf("saved performance setup is unavailable or incomplete")
		}
		reports := filepath.Join(w.directory, "reports")
		if info, err := os.Lstat(reports); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("saved reports directory is unavailable")
		}
		executable := options.Executable
		if executable == "" {
			executable, err = os.Executable()
		}
		if err != nil || !filepath.IsAbs(executable) || strings.ContainsAny(executable, "\x00\r\n") {
			return nil, fmt.Errorf("MCP executable must be an absolute single-line path")
		}
		if info, err := os.Stat(executable); err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			return nil, fmt.Errorf("MCP executable must be an installed executable file")
		}
		entry = map[string]any{"command": executable, "args": []string{"performance", "mcp", "--reports", reports, "--producer", w.state.Producer, "--project", w.project, "--container-root", w.state.Binding.ContainerRoot}}
	}
	receiptPath := filepath.Join(w.directory, "mcp-clients.json")
	prior, err := readOptional(receiptPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read MCP ownership receipt")
	}
	receipt := clientReceipt{Version: 1, Project: w.project, Clients: map[string]clientRecord{}}
	if prior != nil {
		if err := strictClientJSON(prior, &receipt); err != nil || receipt.Version != 1 || receipt.Project != w.project || receipt.Clients == nil {
			return nil, fmt.Errorf("invalid MCP ownership receipt")
		}
		for id := range receipt.Clients {
			if !slices.Contains([]string{"codex", "cursor", "claude"}, id) {
				return nil, fmt.Errorf("unknown client in MCP ownership receipt")
			}
		}
	}
	paths := map[string]string{}
	lockDirs := []string{}
	for _, id := range clients {
		path, err := w.clientPath(id)
		if err != nil {
			return nil, err
		}
		paths[id] = path
		if err := safeParents(path); err != nil {
			return nil, err
		}
		if saved, ok := receipt.Clients[id]; ok {
			if saved.Path != path {
				return nil, fmt.Errorf("%s config location changed; disconnect using its original configuration directory", id)
			}
			if err := validateClientRecord(saved, path, w.project, id); err != nil {
				return nil, err
			}
		}
		if id == "claude" {
			parent, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			lockDirs = append(lockDirs, parent)
		}
	}
	var excludeFile string
	if slices.Contains(clients, "codex") || slices.Contains(clients, "cursor") {
		data, err := w.command(ctx, "git", "rev-parse", "--path-format=absolute", "--git-path", "info/exclude")
		if err != nil {
			return nil, fmt.Errorf("Codex/Cursor local connections require a Git checkout")
		}
		excludeFile = strings.TrimSpace(string(data))
		if !filepath.IsAbs(excludeFile) || strings.ContainsAny(excludeFile, "\x00\r\n") {
			return nil, fmt.Errorf("invalid Git exclude location")
		}
		if err := safeParents(excludeFile); err != nil {
			return nil, err
		}
		lockDirs = append(lockDirs, filepath.Dir(excludeFile))
	}
	slices.Sort(lockDirs)
	for _, parent := range slices.Compact(lockDirs) {
		unlock, err := cache.LockProject(ctx, parent)
		if err != nil {
			return nil, err
		}
		defer unlock()
	}
	excludeBefore, err := readOptional(excludeFile)
	if err != nil {
		return nil, fmt.Errorf("cannot safely read Git exclude file")
	}
	excludeAfter := bytes.Clone(excludeBefore)
	edits := []clientEdit{}
	results := []ClientConnection{}
	final := clientReceipt{Version: 1, Project: w.project, Clients: map[string]clientRecord{}}
	for id, record := range receipt.Clients {
		final.Clients[id] = record
	}
	for _, id := range clients {
		path := paths[id]
		before, err := readOptional(path)
		if err != nil {
			return nil, fmt.Errorf("cannot safely read %s configuration", id)
		}
		saved, known := receipt.Clients[id]
		if !connect && !known {
			results = append(results, ClientConnection{Client: id, Path: path})
			continue
		}
		if id != "claude" {
			if err := w.localConfig(ctx, path, before != nil); err != nil {
				return nil, err
			}
			if known && saved.ExcludeFile != "" && saved.ExcludeFile != excludeFile {
				return nil, fmt.Errorf("Git exclude location changed")
			}
		}
		next, record, err := planClient(id, w.project, path, before, saved, known, entry, connect)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		record.Pending = false
		retainLocal := !connect && next != nil && saved.ExcludeOwned
		if id != "claude" {
			block, err := w.clientExclude(ctx, path, id)
			if err != nil {
				return nil, err
			}
			count := bytes.Count(excludeAfter, []byte(block))
			if saved.ExcludeOwned && (saved.ExcludeBlock != block || count != 1 && !(saved.Pending && count == 0)) {
				return nil, fmt.Errorf("owned %s Git exclusion was changed", id)
			}
			if connect && (before == nil || saved.ExcludeOwned && saved.Pending) && !bytes.Contains(excludeAfter, []byte(block)) {
				if len(excludeAfter) > 0 && excludeAfter[len(excludeAfter)-1] != '\n' {
					return nil, fmt.Errorf("Git exclude must end in a newline")
				}
				excludeAfter = append(excludeAfter, block...)
				record.ExcludeOwned = true
				record.ExcludeFile = excludeFile
				record.ExcludeBlock = block
			}
			if !connect && saved.ExcludeOwned && !retainLocal {
				excludeAfter = bytes.Replace(excludeAfter, []byte(block), nil, 1)
			}
		}
		changed := !bytes.Equal(before, next) || (before == nil) != (next == nil)
		if changed {
			edits = append(edits, clientEdit{path: path, before: before, after: next, remove: next == nil})
		}
		results = append(results, ClientConnection{Client: id, Path: path, Changed: changed, Owned: record.Owned})
		if connect {
			final.Clients[id] = record
			pending := record
			pending.Pending = true
			pending.Hashes = slices.Compact(append(slices.Clone(saved.Hashes), record.Hashes...))
			pending.Blocks = slices.Compact(append(slices.Clone(saved.Blocks), record.Blocks...))
			receipt.Clients[id] = pending
		} else {
			saved.Pending = true
			receipt.Clients[id] = saved
			delete(final.Clients, id)
			if retainLocal {
				record.Owned = false
				record.Hashes = nil
				record.Blocks = nil
				final.Clients[id] = record
			}
		}
	}
	if !bytes.Equal(excludeBefore, excludeAfter) {
		excludeEdit := clientEdit{path: excludeFile, before: excludeBefore, after: excludeAfter}
		if connect {
			edits = append([]clientEdit{excludeEdit}, edits...)
		} else {
			edits = append(edits, excludeEdit)
		}
	}
	pending, _ := json.MarshalIndent(receipt, "", "  ")
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := atomic(receiptPath, append(pending, '\n')); err != nil {
		return nil, fmt.Errorf("cannot save MCP ownership before updating clients")
	}
	if err := applyClientEdits(ctx, edits); err != nil {
		return nil, err
	}
	data, _ := json.MarshalIndent(final, "", "  ")
	if err := atomic(receiptPath, append(data, '\n')); err != nil {
		return nil, fmt.Errorf("connections updated; retry to finish saving ownership")
	}
	if !connect {
		for _, id := range clients {
			for _, dir := range receipt.Clients[id].CreatedDirs {
				_ = os.Remove(dir)
			}
		}
	}
	return results, nil
}

func (w workspace) clientPath(id string) (string, error) {
	switch id {
	case "codex":
		return filepath.Join(w.project, ".codex", "config.toml"), nil
	case "cursor":
		return filepath.Join(w.project, ".cursor", "mcp.json"), nil
	case "claude":
		if base := os.Getenv("CLAUDE_CONFIG_DIR"); base != "" {
			if !filepath.IsAbs(base) || strings.ContainsAny(base, "\x00\r\n") || inside(w.project, base) {
				return "", fmt.Errorf("CLAUDE_CONFIG_DIR must be an absolute private directory outside the project")
			}
			return filepath.Join(base, ".claude.json"), nil
		}
		base, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(base, ".claude.json"), nil
	}
	return "", fmt.Errorf("unknown MCP client")
}

func (w workspace) localConfig(ctx context.Context, name string, exists bool) error {
	data, err := w.command(ctx, "git", "ls-files", "-z", "--", name)
	if err != nil || len(data) != 0 {
		return fmt.Errorf("refusing tracked client config; %s must stay local", name)
	}
	if exists {
		data, err := w.command(ctx, "git", "check-ignore", "--no-index", "--", name)
		if err != nil || len(data) == 0 {
			return fmt.Errorf("existing client config must already be locally ignored: %s", name)
		}
	}
	return nil
}

func (w workspace) clientExclude(ctx context.Context, name, id string) (string, error) {
	root, err := w.command(ctx, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(strings.TrimSpace(string(root)), name)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("client settings are outside the checkout")
	}
	pattern := filepath.ToSlash(rel)
	for _, char := range []string{"\\", "[", "*", "?", " "} {
		pattern = strings.ReplaceAll(pattern, char, "\\"+char)
	}
	return "# >>> phpwarden-mcp " + w.owner + " " + id + " >>>\n/" + pattern + "\n# <<< phpwarden-mcp " + w.owner + " " + id + " <<<\n", nil
}
