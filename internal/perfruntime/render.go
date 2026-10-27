package perfruntime

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	buildinfo "github.com/rchehd/phpwarden"
	"github.com/rchehd/phpwarden/internal/performance"
)

//go:embed build.sh
var buildScript []byte

func source() buildinfo.SourcePin {
	pin := buildinfo.SPXSource()
	if pin.Commit != performance.Producer || len(pin.SHA256) != 64 || pin.URL != "https://codeload.github.com/NoiseByNorthwest/php-spx/tar.gz/"+pin.Commit {
		panic("invalid bundled SPX source identity")
	}
	return pin
}

func archive(ctx context.Context, name string) ([]byte, error) {
	var data []byte
	var err error
	if name != "" {
		data, err = readRegular(name, 2<<20)
	} else {
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, source().URL, nil)
		if requestErr != nil {
			return nil, requestErr
		}
		client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" || req.URL.Host != "codeload.github.com" || len(via) > 3 {
				return fmt.Errorf("unexpected source redirect")
			}
			return nil
		}}
		response, requestErr := client.Do(request)
		if requestErr != nil {
			return nil, fmt.Errorf("cannot download pinned SPX source; use --spx-archive for offline setup")
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			return nil, fmt.Errorf("pinned SPX source download failed")
		}
		data, err = io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	}
	if err != nil || len(data) > 2<<20 || digest(data) != source().SHA256 {
		return nil, fmt.Errorf("SPX source archive failed its pinned SHA256 verification")
	}
	return data, nil
}

func (w *workspace) imageTag() string { return "phpwarden-spx:" + w.owner }
func (w *workspace) baseTag() string  { return "phpwarden-spx-base:" + w.owner }

func (w *workspace) ini(key string) []byte {
	lines := []string{"extension=/opt/phpwarden-spx/spx.so", "spx.data_dir=" + containerReports, "spx.http_enabled=0"}
	if w.state.Binding.HTTP {
		lines[2] = "spx.http_enabled=1"
		lines = append(lines, "spx.http_key="+key, "spx.http_ip_whitelist="+w.state.Binding.HTTPAllow)
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

func (w *workspace) mounts() []map[string]any {
	mounts := []map[string]any{{"type": "bind", "source": filepath.Join(w.directory, "reports"), "target": containerReports, "bind": map[string]any{"create_host_path": false}}}
	paths := []string{"/usr/local/etc/php/conf.d/99-phpwarden-spx.ini"}
	if w.state.Binding.Environment == "ddev" {
		minor := w.state.Binding.PHP
		paths = []string{"/etc/php/" + minor + "/cli/conf.d/99-phpwarden-spx.ini", "/etc/php/" + minor + "/fpm/conf.d/99-phpwarden-spx.ini"}
	}
	for _, name := range paths {
		mounts = append(mounts, map[string]any{"type": "bind", "source": filepath.Join(w.directory, "spx.ini"), "target": name, "read_only": true, "bind": map[string]any{"create_host_path": false}})
	}
	for _, mount := range mounts {
		mount["source"] = strings.ReplaceAll(mount["source"].(string), "$", "$$")
	}
	return mounts
}

func (w *workspace) overlay() []byte {
	service := map[string]any{"volumes": w.mounts(), "labels": map[string]string{ownerLabel: w.owner, producerLabel: source().Commit}}
	if w.state.Binding.Environment == "compose" {
		service["image"] = w.imageTag()
		service["pull_policy"] = "never"
	}
	data, _ := json.MarshalIndent(map[string]any{"services": map[string]any{w.state.Binding.Service: service}}, "", "  ")
	return append(data, '\n')
}

func (w *workspace) dockerfile() []byte {
	mode, archiveName, scriptName := "official", "spx.tar.gz", "build.sh"
	lines := []string{"FROM " + w.baseTag(), "USER root"}
	if w.state.Binding.Environment == "ddev" {
		mode = "ddev"
		archiveName = "phpwarden-spx.tar.gz"
		scriptName = "phpwarden-spx-build.sh"
		lines = []string{"USER root", "RUN test \"$DDEV_PHP_VERSION\" = \"" + w.state.Binding.PHP + "\""}
	}
	lines = append(lines, "COPY "+archiveName+" /tmp/phpwarden-spx.tar.gz", "COPY "+scriptName+" /tmp/phpwarden-spx-build.sh", fmt.Sprintf("RUN sh /tmp/phpwarden-spx-build.sh %s %s %s", mode, w.state.Binding.PHP, source().SHA256), "RUN echo "+source().Commit+" > /opt/phpwarden-spx/producer", "LABEL "+ownerLabel+"="+w.owner+" "+producerLabel+"="+source().Commit)
	if mode != "ddev" && w.state.Binding.BaseUser != "" {
		lines = append(lines, "USER "+w.state.Binding.BaseUser)
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

func (w *workspace) prepareFiles(ctx context.Context, options Options) error {
	readArchive := options.readArchive
	if readArchive == nil {
		readArchive = archive
	}
	data, err := readArchive(ctx, options.SourceArchive)
	if err != nil {
		return err
	}
	key := ""
	if w.state.Binding.HTTP {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return err
		}
		key = hex.EncodeToString(secret)
	}
	files := map[string][]byte{"build.sh": buildScript, "spx.tar.gz": data, "spx.ini": w.ini(key), "Dockerfile": w.dockerfile(), ".dockerignore": []byte("**\n!Dockerfile\n!build.sh\n!spx.tar.gz\n")}
	if key != "" {
		files["http.key"] = []byte(key + "\n")
	}
	if w.state.Binding.Environment != "docker" {
		files["compose.yaml"] = w.overlay()
	}
	if w.state.Binding.Environment == "ddev" {
		files[projectNames[0]] = files["Dockerfile"]
		files[projectNames[1]] = buildScript
		files[projectNames[2]] = data
		files[projectNames[3]] = files["compose.yaml"]
	}
	for name := range files {
		path, err := w.filename(name)
		if err != nil {
			return err
		}
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return fmt.Errorf("an adapter path already exists; nothing will be overwritten")
		}
	}
	reports := filepath.Join(w.directory, "reports")
	if err := os.Mkdir(reports, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	info, err := os.Lstat(reports)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unsafe reports directory")
	}
	if err := os.Chmod(reports, os.ModeSticky|0777); err != nil {
		return err
	}
	for name, data := range files {
		w.state.Files[name] = digest(data)
	}
	if err := w.save(); err != nil {
		return err
	}
	created := []string{}
	for name, data := range files {
		path, _ := w.filename(name)
		mode := os.FileMode(0600)
		if name == "spx.ini" {
			mode = 0644
		}
		if err := create(path, data, mode); err != nil {
			for _, path := range created {
				_ = os.Remove(path)
			}
			return err
		}
		created = append(created, path)
	}
	if w.state.Binding.Environment == "ddev" {
		if err := w.exclude(ctx, true); err != nil {
			for _, path := range created {
				_ = os.Remove(path)
			}
			return err
		}
	}
	return w.save()
}
