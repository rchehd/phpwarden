package perfruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/rchehd/phpwarden/internal/cache"
)

func digest(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

func canonical(value string) (string, error) {
	value, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(value)
}

func inside(base, name string) bool {
	rel, err := filepath.Rel(base, name)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func locate(options Options) (workspace, error) {
	var w workspace
	project := options.Project
	if project == "" {
		project = "."
	}
	var err error
	w.project, err = canonical(project)
	if err != nil {
		return w, fmt.Errorf("cannot open the project directory")
	}
	info, err := os.Stat(w.project)
	if err != nil || !info.IsDir() {
		return w, fmt.Errorf("project must be a directory")
	}
	base := options.StateDirectory
	if base == "" {
		base, err = cache.Directory("performance")
		if err != nil {
			return w, err
		}
	}
	base, err = filepath.Abs(base)
	if err != nil {
		return w, err
	}
	parent := base
	for {
		if _, err := os.Lstat(parent); err == nil {
			break
		}
		next := filepath.Dir(parent)
		if next == parent {
			return w, fmt.Errorf("cannot resolve the state directory")
		}
		parent = next
	}
	physical, err := canonical(parent)
	if err != nil {
		return w, err
	}
	rest, err := filepath.Rel(parent, base)
	if err != nil {
		return w, err
	}
	base = filepath.Join(physical, rest)
	if inside(w.project, base) {
		return w, fmt.Errorf("runtime state must be outside the project")
	}
	w.owner = cache.ProjectKey(w.project)[:24]
	w.directory = filepath.Join(base, w.owner)
	for _, value := range []string{w.project, w.directory} {
		if strings.IndexFunc(value, func(r rune) bool { return unicode.IsControl(r) || unicode.In(r, unicode.Cf) }) >= 0 {
			return w, fmt.Errorf("runtime paths cannot contain terminal controls")
		}
	}
	w.process = options.Process
	return w, nil
}

func privateDirectory(name string) error {
	if err := os.MkdirAll(name, 0700); err != nil {
		return fmt.Errorf("cannot create private runtime state")
	}
	info, err := os.Lstat(name)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("runtime state directory must be private and cannot be a symlink")
	}
	return nil
}

func readRegular(name string, limit int64) ([]byte, error) {
	info, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("unsafe or oversized runtime input")
	}
	file, err := openInput(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("runtime input changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, fmt.Errorf("runtime input exceeds its limit")
	}
	after, err := os.Lstat(name)
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(info, after) || !info.ModTime().Equal(after.ModTime()) || info.Size() != after.Size() {
		return nil, fmt.Errorf("runtime input changed while reading")
	}
	return data, nil
}

func create(name string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("refusing to overwrite an existing runtime adapter")
	}
	_, err = file.Write(data)
	closed := file.Close()
	if err != nil {
		return err
	}
	return closed
}

func atomic(name string, data []byte) error {
	mode := os.FileMode(0600)
	if info, err := os.Lstat(name); err == nil {
		mode = info.Mode().Perm()
	}
	if info, err := os.Lstat(name); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("refusing an unsafe runtime state file")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(name), ".phpwarden-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return err
	}
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	closed := file.Close()
	if err != nil {
		return err
	}
	if closed != nil {
		return closed
	}
	return os.Rename(file.Name(), name)
}

func (w *workspace) load() error {
	data, err := readRegular(filepath.Join(w.directory, "state.json"), 128<<10)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&w.state) != nil || decoder.Decode(new(any)) != io.EOF || w.state.Version != 1 || w.state.Project != w.project || w.state.Owner != w.owner || w.state.Producer != source().Commit {
		return fmt.Errorf("invalid runtime state identity or version")
	}
	if w.state.Phase != "prepared" && w.state.Phase != "removing" && w.state.Phase != "removed" {
		return fmt.Errorf("invalid runtime state phase")
	}
	for name := range w.state.Files {
		if !slices.Contains(projectNames, name) && !slices.Contains(privateNames, name) {
			return fmt.Errorf("runtime state contains an unknown owned path")
		}
	}
	b := w.state.Binding
	if b.Environment != "ddev" && b.Environment != "compose" && b.Environment != "docker" {
		return fmt.Errorf("invalid saved runtime environment")
	}
	if b.PHP != "8.1" && b.PHP != "8.2" && b.PHP != "8.3" && b.PHP != "8.4" {
		return fmt.Errorf("invalid saved PHP runtime")
	}
	if !filepath.IsAbs(b.ContainerRoot) || strings.ContainsAny(b.ContainerRoot, "\r\n\x00") {
		return fmt.Errorf("invalid saved source root")
	}
	if b.Environment == "ddev" && (b.Service != "web" || !namePattern.MatchString(b.ComposeProject)) {
		return fmt.Errorf("invalid saved DDEV target")
	}
	if b.Environment == "compose" && (!namePattern.MatchString(b.Service) || !namePattern.MatchString(b.ComposeProject) || len(b.ComposeFiles) == 0) {
		return fmt.Errorf("invalid saved Compose target")
	}
	if b.Environment == "docker" && !namePattern.MatchString(b.Container) {
		return fmt.Errorf("invalid saved Docker target")
	}
	for _, profile := range b.Profiles {
		if !namePattern.MatchString(profile) {
			return fmt.Errorf("invalid saved profile")
		}
	}
	for _, file := range append(append([]string{}, b.ComposeFiles...), b.EnvFiles...) {
		if !filepath.IsAbs(file) || strings.ContainsAny(file, "\r\n\x00") {
			return fmt.Errorf("invalid saved Compose path")
		}
	}
	if b.Environment != "ddev" && !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(b.BaseID) {
		return fmt.Errorf("invalid saved base image")
	}
	if b.BaseUser != "" && !userPattern.MatchString(b.BaseUser) {
		return fmt.Errorf("invalid saved image user")
	}
	for _, name := range []string{"Dockerfile", "build.sh", "spx.tar.gz", "spx.ini", ".dockerignore"} {
		if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(w.state.Files[name]) {
			return fmt.Errorf("runtime receipt omits a required artifact")
		}
	}
	return nil
}

func (w *workspace) save() error {
	data, err := json.MarshalIndent(w.state, "", "  ")
	if err != nil {
		return err
	}
	return atomic(filepath.Join(w.directory, "state.json"), append(data, '\n'))
}

func (w *workspace) filename(name string) (string, error) {
	if slices.Contains(privateNames, name) {
		return filepath.Join(w.directory, name), nil
	}
	if !slices.Contains(projectNames, name) {
		return "", fmt.Errorf("unknown adapter path")
	}
	parts := strings.Split(name, "/")
	current := w.project
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("DDEV adapter parents must be real directories")
		}
	}
	return filepath.Join(w.project, filepath.FromSlash(name)), nil
}

func (w *workspace) verifyFiles(allowMissing bool) error {
	for name, expected := range w.state.Files {
		filename, err := w.filename(name)
		if err != nil {
			return err
		}
		data, err := readRegular(filename, 2<<20)
		if allowMissing && os.IsNotExist(err) {
			continue
		}
		if err != nil || digest(data) != expected {
			return fmt.Errorf("an owned adapter was changed; preserve or restore it before continuing")
		}
	}
	return nil
}

func (w *workspace) verifyInputs() error {
	for name, expected := range w.state.Inputs {
		data, err := readRegular(name, 4<<20)
		if err != nil || digest(data) != expected {
			return fmt.Errorf("project runtime inputs changed; prepare the integration again after teardown")
		}
	}
	return nil
}

func (w *workspace) fingerprint(names []string) error {
	for _, name := range names {
		data, err := readRegular(name, 4<<20)
		if err != nil {
			return fmt.Errorf("cannot read runtime input")
		}
		w.state.Inputs[name] = digest(data)
	}
	return nil
}

func (w *workspace) gitSnapshot(ctx context.Context) (string, error) {
	var snapshot []byte
	for _, args := range [][]string{{"status", "--porcelain=v1", "-z", "--untracked-files=all"}, {"diff", "--no-ext-diff", "--binary"}, {"diff", "--cached", "--no-ext-diff", "--binary"}} {
		data, err := w.command(ctx, "git", args...)
		if err != nil {
			return "", fmt.Errorf("cannot inspect Git status")
		}
		snapshot = append(snapshot, data...)
		snapshot = append(snapshot, 0)
	}
	return digest(snapshot), nil
}

func (w *workspace) excludeBlock() []byte {
	lines := []string{"# >>> phpwarden-spx " + w.owner + " >>>"}
	for _, name := range projectNames {
		pattern := w.state.RepoPrefix + name
		for _, char := range []string{"\\", "[", "*", "?"} {
			pattern = strings.ReplaceAll(pattern, char, "\\"+char)
		}
		lines = append(lines, "/"+pattern)
	}
	lines = append(lines, "# <<< phpwarden-spx "+w.owner+" <<<")
	return []byte(strings.Join(lines, "\n") + "\n")
}

func (w *workspace) exclude(ctx context.Context, add bool) error {
	data, err := w.command(ctx, "git", "rev-parse", "--path-format=absolute", "--git-path", "info/exclude")
	if err != nil {
		return fmt.Errorf("DDEV integration requires a Git checkout for local exclusions")
	}
	name := strings.TrimSpace(string(data))
	if !filepath.IsAbs(name) || strings.ContainsAny(name, "\r\n\x00") {
		return fmt.Errorf("invalid Git exclude path")
	}
	if w.state.ExcludeFile != "" && w.state.ExcludeFile != name {
		return fmt.Errorf("Git exclude location changed")
	}
	w.state.ExcludeFile = name
	current, err := readRegular(name, 1<<20)
	if os.IsNotExist(err) {
		current = nil
	} else if err != nil {
		return fmt.Errorf("cannot safely read Git exclude file")
	}
	block := w.excludeBlock()
	if add {
		if bytes.Contains(current, block) {
			return nil
		}
		if len(current) > 0 && current[len(current)-1] != '\n' {
			return fmt.Errorf("Git exclude must end in a newline before adding a local block")
		}
		w.state.ExcludeAdded = true
		return atomic(name, append(current, block...))
	}
	if w.state.ExcludeAdded {
		if !bytes.Contains(current, block) {
			return fmt.Errorf("owned Git exclusion block was changed")
		}
		if err := atomic(name, bytes.Replace(current, block, nil, 1)); err != nil {
			return err
		}
		w.state.ExcludeAdded = false
	}
	return nil
}
