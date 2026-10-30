package agentinstructions

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/rchehd/phpwarden/internal/agentskills"
	"github.com/rchehd/phpwarden/internal/cache"
)

const start = "<!-- phpwarden:instructions "
const end = "<!-- /phpwarden:instructions -->"
const maxSize = 1 << 20

const guidance = `## PHPWarden

Use the available phpwarden skill for requested code checks or validation after
task edits; a project copy is .agents/skills/phpwarden/SKILL.md. If unavailable,
report missing setup. Limit checks to task files/directories or the requested
diff/commits; preserve unrelated edits and configured analyzer policy.
Use phpwarden-review for applicable project checklist questions and verified
standards. Keep native findings separate from agent observations and explain
skipped/incomplete coverage. Reuse supplied results when formatting reports.
Planning and explanations need no automatic check. Honor user limits on tools,
network, tests and builds; this instruction does not authorize hooks or collectors.
`

type Installation struct {
	Path    string   `json:"path"`
	Clients []string `json:"clients"`
	Changed bool     `json:"changed"`
	Removed bool     `json:"removed,omitempty"`
	Deleted bool     `json:"deleted,omitempty"`
}

type ownership struct {
	Version   int      `json:"version"`
	Clients   []string `json:"clients"`
	SHA256    string   `json:"sha256"`
	Separator string   `json:"separator,omitempty"`
	Created   bool     `json:"created,omitempty"`
}

type block struct {
	ownership
	from, to int
}

type snapshot struct {
	data []byte
	info os.FileInfo
}

type change struct {
	Installation
	name   string
	before snapshot
	after  []byte
}

type Plan struct {
	root    string
	clients []string
}

func Prepare(ctx context.Context, root string, clients []string) (Plan, error) {
	return prepare(ctx, root, clients, false)
}

func PrepareRemoval(ctx context.Context, root string, clients []string) (Plan, error) {
	return prepare(ctx, root, clients, true)
}

func prepare(ctx context.Context, root string, clients []string, remove bool) (Plan, error) {
	selected, err := agentskills.ParseClients(strings.Join(clients, ","))
	if err != nil {
		return Plan{}, err
	}
	root, err = filepath.Abs(root)
	if err == nil {
		root, err = filepath.EvalSymlinks(root)
	}
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{root, selected}
	if len(selected) == 0 {
		return plan, nil
	}
	release, err := cache.LockProject(ctx, root)
	if err != nil {
		return Plan{}, err
	}
	defer release()
	_, err = plan.changes(remove)
	return plan, err
}

func read(root *os.Root, name string) (snapshot, error) {
	info, err := root.Lstat(name)
	if os.IsNotExist(err) {
		return snapshot{}, nil
	}
	if err != nil {
		return snapshot{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxSize {
		return snapshot{}, fmt.Errorf("instructions %s must be a regular file without symlinks, at most 1 MiB", name)
	}
	file, err := root.Open(name)
	if err != nil {
		return snapshot{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return snapshot{}, fmt.Errorf("instructions %s changed while reading", name)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil {
		return snapshot{}, err
	}
	if len(data) > maxSize || !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return snapshot{}, fmt.Errorf("instructions %s must contain bounded UTF-8 text", name)
	}
	return snapshot{data, info}, nil
}

func owned(data []byte) (*block, error) {
	starts, ends := bytes.Count(data, []byte(start)), bytes.Count(data, []byte(end))
	if starts == 0 && ends == 0 {
		return nil, nil
	}
	if starts != 1 || ends != 1 {
		return nil, fmt.Errorf("ambiguous PHPWarden instruction markers; preserve and reconcile the file manually")
	}
	a := bytes.Index(data, []byte(start))
	b := bytes.Index(data[a:], []byte("\n"))
	z := bytes.Index(data, []byte(end))
	if b < 0 || z < a+b || z == 0 || data[z-1] != '\n' || a > 0 && data[a-1] != '\n' {
		return nil, fmt.Errorf("invalid PHPWarden instruction block")
	}
	header := strings.TrimSuffix(string(data[a+len(start):a+b]), "\r")
	if !strings.HasSuffix(header, " -->") {
		return nil, fmt.Errorf("invalid PHPWarden ownership header")
	}
	var meta ownership
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSuffix(header, " -->")))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&meta) != nil || meta.Version != 1 || len(meta.Clients) == 0 {
		return nil, fmt.Errorf("unsupported PHPWarden instruction ownership")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing PHPWarden ownership data")
	}
	selected, err := agentskills.ParseClients(strings.Join(meta.Clients, ","))
	if err != nil || !slices.Equal(selected, meta.Clients) {
		return nil, fmt.Errorf("invalid PHPWarden instruction clients")
	}
	body := data[a+b+1 : z]
	if meta.SHA256 != fmt.Sprintf("%x", sha256.Sum256(body)) {
		return nil, fmt.Errorf("PHPWarden instruction block was customized; leaving it unchanged")
	}
	if !slices.Contains([]string{"", "\n", "\n\n", "\r\n", "\r\n\r\n"}, meta.Separator) || a < len(meta.Separator) || string(data[a-len(meta.Separator):a]) != meta.Separator {
		return nil, fmt.Errorf("PHPWarden instruction separator changed; leaving it unchanged")
	}
	to := z + len(end)
	if bytes.HasPrefix(data[to:], []byte("\r\n")) {
		to += 2
	} else if to < len(data) && data[to] == '\n' {
		to++
	} else if to != len(data) {
		return nil, fmt.Errorf("invalid PHPWarden closing marker")
	}
	return &block{meta, a - len(meta.Separator), to}, nil
}

func render(before snapshot, previous *block, clients []string, body string) []byte {
	if len(clients) == 0 {
		if previous == nil {
			return before.data
		}
		return append(bytes.Clone(before.data[:previous.from]), before.data[previous.to:]...)
	}
	newline := "\n"
	if bytes.Contains(before.data, []byte("\r\n")) {
		newline = "\r\n"
	}
	meta := ownership{Version: 1, Clients: clients, Created: before.info == nil}
	from, to := len(before.data), len(before.data)
	if previous != nil {
		meta.Separator, meta.Created = previous.Separator, previous.Created
		from, to = previous.from, previous.to
	} else if len(before.data) > 0 {
		meta.Separator = newline
		if before.data[len(before.data)-1] != '\n' {
			meta.Separator += newline
		}
	}
	body = strings.ReplaceAll(body, "\n", newline)
	meta.SHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
	header, _ := json.Marshal(meta)
	managed := meta.Separator + start + string(header) + " -->" + newline + body + end + newline
	after := append(bytes.Clone(before.data[:from]), []byte(managed)...)
	return append(after, before.data[to:]...)
}

func importsAgents(data []byte) (bool, error) {
	var fence byte
	width, imported := 0, false
	for _, raw := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if strings.HasPrefix(raw, "\t") || strings.HasPrefix(raw, "    ") {
			continue
		}
		line := strings.TrimSpace(raw)
		if len(line) >= 3 && (line[0] == '`' || line[0] == '~') {
			n := 0
			for n < len(line) && line[n] == line[0] {
				n++
			}
			if n >= 3 {
				if fence == 0 && line[0] == '`' && strings.Contains(line[n:], "`") {
					continue
				}
				if fence == 0 {
					fence, width = line[0], n
				} else if fence == line[0] && n >= width && strings.TrimSpace(line[n:]) == "" {
					fence, width = 0, 0
				}
				continue
			}
		}
		if fence == 0 && line == "@AGENTS.md" {
			imported = true
		}
	}
	if fence != 0 {
		return false, fmt.Errorf("close the unclosed Markdown code fence before installing instructions")
	}
	return imported, nil
}

func (plan Plan) changes(remove bool) ([]change, error) {
	if len(plan.clients) == 0 {
		return nil, nil
	}
	root, err := os.OpenRoot(plan.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	before, err := read(root, "AGENTS.md")
	if err != nil {
		return nil, err
	}
	previous, err := owned(before.data)
	if err != nil {
		return nil, err
	}
	if !remove {
		if _, err := importsAgents(before.data); err != nil {
			return nil, fmt.Errorf("AGENTS.md: %w", err)
		}
	}
	var clients []string
	for _, client := range []string{"codex", "cursor", "claude"} {
		had := previous != nil && slices.Contains(previous.Clients, client)
		keep := had
		if slices.Contains(plan.clients, client) {
			keep = !remove
		}
		if keep {
			clients = append(clients, client)
		}
	}
	makeChange := func(name string, before snapshot, previous *block, clients []string, body string) change {
		after := render(before, previous, clients, body)
		removed := previous != nil && len(clients) == 0
		deleted := removed && previous.Created && len(after) == 0
		return change{Installation{filepath.Join(plan.root, name), clients, !bytes.Equal(before.data, after), removed, deleted}, name, before, after}
	}
	rows := []change{makeChange("AGENTS.md", before, previous, clients, guidance)}
	if slices.Contains(plan.clients, "claude") {
		before, err = read(root, "CLAUDE.md")
		if err != nil {
			return nil, err
		}
		previous, err = owned(before.data)
		if err != nil {
			return nil, err
		}
		imported, scanErr := importsAgents(before.data)
		if !remove && scanErr != nil {
			return nil, fmt.Errorf("CLAUDE.md: %w", scanErr)
		}
		var selected []string
		if !remove {
			selected = []string{"claude"}
		}
		if previous == nil && imported {
			rows = append(rows, change{Installation: Installation{Path: filepath.Join(plan.root, "CLAUDE.md"), Clients: selected}})
		} else {
			rows = append(rows, makeChange("CLAUDE.md", before, previous, selected, "@AGENTS.md\n"))
		}
	}
	for _, row := range rows {
		if len(row.after) > maxSize {
			return nil, fmt.Errorf("instructions %s would exceed 1 MiB", row.name)
		}
	}
	return rows, nil
}

func (plan Plan) Install(ctx context.Context) ([]Installation, error) { return plan.apply(ctx, false) }
func (plan Plan) Remove(ctx context.Context) ([]Installation, error)  { return plan.apply(ctx, true) }

func (plan Plan) apply(ctx context.Context, remove bool) ([]Installation, error) {
	if len(plan.clients) == 0 {
		return nil, nil
	}
	release, err := cache.LockProject(ctx, plan.root)
	if err != nil {
		return nil, err
	}
	defer release()
	rows, err := plan.changes(remove)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(plan.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	var result []Installation
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if row.Changed {
			if err := publish(root, row); err != nil {
				return result, fmt.Errorf("instructions %s: %w", row.name, err)
			}
		}
		result = append(result, row.Installation)
	}
	return result, nil
}

func publish(root *os.Root, row change) error {
	name := ".phpwarden-instructions-" + rand.Text()
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer root.Remove(name)
	mode := os.FileMode(0o644)
	if row.before.info != nil {
		mode = row.before.info.Mode().Perm()
	}
	_, err = file.Write(row.after)
	if err == nil {
		err = file.Chmod(mode)
	}
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	current, err := read(root, row.name)
	if err != nil || !bytes.Equal(current.data, row.before.data) || (current.info == nil) != (row.before.info == nil) || current.info != nil && (!os.SameFile(current.info, row.before.info) || current.info.Mode() != row.before.info.Mode()) {
		return fmt.Errorf("file changed during installation; leaving it unchanged")
	}
	if row.Deleted {
		return root.Remove(row.name)
	}
	if row.before.info == nil {
		return root.Link(name, row.name)
	}
	return root.Rename(name, row.name)
}
