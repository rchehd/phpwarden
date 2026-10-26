package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	buildinfo "github.com/rchehd/phpwarden"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/toolset"
)

type processFunc func(context.Context, runner.Command) runner.Result

func (process processFunc) Run(ctx context.Context, command runner.Command) runner.Result {
	return process(ctx, command)
}

type transportFunc func(*http.Request) (*http.Response, error)

func (transport transportFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

type fixture struct {
	options  Options
	metadata buildinfo.Info
	release  Release
	archive  []byte
	sums     []byte
	calls    int
	status   int
	mutex    sync.Mutex
}

func archiveFixture(t *testing.T, tag, platform string, extra *tar.Header) ([]byte, buildinfo.Info) {
	t.Helper()
	images := map[string]string{}
	for _, runtime := range []string{"8.1", "8.2", "8.3", "8.4"} {
		images[runtime] = fmt.Sprintf("ghcr.io/rchehd/phpwarden@sha256:%x", sha256.Sum256([]byte(runtime)))
	}
	manifest, err := toolset.PublishedManifest(tag, images)
	if err != nil {
		t.Fatal(err)
	}
	metadata := buildinfo.Info{Version: tag, Commit: strings.Repeat("b", 40), Distribution: "standalone", Platform: platform, ManifestSHA256: fmt.Sprintf("%x", sha256.Sum256(manifest))}
	info, _ := json.Marshal(metadata)
	files := map[string][]byte{"phpwarden": []byte("new executable"), "COMMIT": []byte(metadata.Commit + "\n"), "release.json": info, "compatibility.json": manifest}
	var out bytes.Buffer
	zip := gzip.NewWriter(&out)
	archive := tar.NewWriter(zip)
	for _, name := range []string{"phpwarden", "COMMIT", "release.json", "compatibility.json"} {
		value := files[name]
		if err := archive.WriteHeader(&tar.Header{Name: name, Size: int64(len(value)), Mode: 0o755, Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		archive.Write(value)
	}
	if extra != nil {
		if err := archive.WriteHeader(extra); err != nil {
			t.Fatal(err)
		}
		if extra.Size > 0 {
			archive.Write(make([]byte, extra.Size))
		}
	}
	archive.Close()
	zip.Close()
	return out.Bytes(), metadata
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{}
	f.archive, f.metadata = archiveFixture(t, "v1.2.0", "linux/amd64", nil)
	f.sums = []byte(fmt.Sprintf("%x  phpwarden-linux-amd64.tar.gz\n", sha256.Sum256(f.archive)))
	f.release = Release{Tag: "v1.2.0", Assets: []Asset{{Name: "phpwarden-linux-amd64.tar.gz", URL: downloadRoot + "v1.2.0/phpwarden-linux-amd64.tar.gz", Size: int64(len(f.archive))}, {Name: "SHA256SUMS", URL: downloadRoot + "v1.2.0/SHA256SUMS", Size: int64(len(f.sums))}}}
	root := t.TempDir()
	target := filepath.Join(root, "phpwarden")
	if err := os.WriteFile(target, []byte("old executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.options = Options{Executable: target, Current: buildinfo.Info{Version: "v1.0.0", Commit: strings.Repeat("c", 40), Distribution: "standalone", Platform: "linux/amd64", ManifestSHA256: strings.Repeat("d", 64)}}
	f.options.Client = Client{HTTP: &http.Client{Transport: transportFunc(func(request *http.Request) (*http.Response, error) {
		f.mutex.Lock()
		defer f.mutex.Unlock()
		f.calls++
		if request.Context().Err() != nil {
			return nil, request.Context().Err()
		}
		status := http.StatusOK
		if f.status != 0 {
			status = f.status
		}
		var data []byte
		switch request.URL.String() {
		case apiRoot + "latest", apiRoot + "tags/" + f.release.Tag:
			data, _ = json.Marshal(f.release)
		case f.release.Assets[0].URL:
			data = f.archive
		case f.release.Assets[1].URL:
			data = f.sums
		default:
			return nil, fmt.Errorf("unexpected URL %s", request.URL)
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data)), Header: http.Header{}}, nil
	})}}
	f.options.Process = processFunc(func(_ context.Context, command runner.Command) runner.Result {
		value := f.metadata
		if command.Name == target {
			data, _ := os.ReadFile(target)
			value = f.options.Current
			if string(data) == "new executable" {
				value = f.metadata
			}
		}
		data, _ := json.Marshal(value)
		return runner.Result{Stdout: data}
	})
	return f
}

func TestAtomicUpdateAndCheckOnly(t *testing.T) {
	for _, check := range []bool{false, true} {
		t.Run(fmt.Sprint(check), func(t *testing.T) {
			f := newFixture(t)
			f.options.Check = check
			result := Run(context.Background(), f.options)
			if result.ExitCode != 0 || !result.Available || result.Updated == check {
				t.Fatalf("%+v", result)
			}
			want := "new executable"
			if check {
				want = "old executable"
			}
			data, _ := os.ReadFile(f.options.Executable)
			if string(data) != want {
				t.Fatalf("installed %s", data)
			}
			entries, _ := os.ReadDir(filepath.Dir(f.options.Executable))
			if len(entries) != 1 {
				t.Fatal("temporary update files remain")
			}
			if check && f.calls != 1 {
				t.Fatalf("check downloaded assets: %d", f.calls)
			}
		})
	}
}

func TestFailedUpdatesPreserveInstalledCLI(t *testing.T) {
	for _, which := range []string{"checksum", "invalid archive", "wrong platform", "probe failure", "missing asset", "api digest", "404", "rate limit", "managed", "source", "invalid version", "changed during update"} {
		t.Run(which, func(t *testing.T) {
			f := newFixture(t)
			switch which {
			case "checksum":
				f.sums = []byte(strings.Repeat("0", 64) + "  phpwarden-linux-amd64.tar.gz\n")
			case "invalid archive":
				f.archive = []byte("not a gzip archive")
				f.sums = []byte(fmt.Sprintf("%x  phpwarden-linux-amd64.tar.gz\n", sha256.Sum256(f.archive)))
			case "wrong platform":
				f.archive, _ = archiveFixture(t, "v1.2.0", "linux/arm64", nil)
				f.sums = []byte(fmt.Sprintf("%x  phpwarden-linux-amd64.tar.gz\n", sha256.Sum256(f.archive)))
			case "probe failure":
				f.options.Process = processFunc(func(context.Context, runner.Command) runner.Result { return runner.Result{ExitCode: 1} })
			case "missing asset":
				f.release.Assets[0].Name = "another-platform.tar.gz"
			case "api digest":
				f.release.Assets[0].Digest = "sha256:" + strings.Repeat("0", 64)
			case "404":
				f.status = 404
			case "rate limit":
				f.status = 403
			case "managed":
				f.options.Current.Distribution = "managed"
			case "source":
				f.options.Current.Distribution = "source"
			case "invalid version":
				f.options.To = "1.2.0-01"
			case "changed during update":
				base := f.options.Process
				f.options.Process = processFunc(func(ctx context.Context, command runner.Command) runner.Result {
					result := base.Run(ctx, command)
					if command.Name != f.options.Executable {
						os.WriteFile(f.options.Executable, []byte("external change"), 0o755)
					}
					return result
				})
			}
			f.release.Assets[0].Size = int64(len(f.archive))
			f.release.Assets[1].Size = int64(len(f.sums))
			result := Run(context.Background(), f.options)
			if result.ExitCode == 0 || result.Updated {
				t.Fatalf("%+v", result)
			}
			data, _ := os.ReadFile(f.options.Executable)
			want := "old executable"
			if which == "changed during update" {
				want = "external change"
			}
			if string(data) != want {
				t.Fatalf("failed update changed CLI: %s", data)
			}
		})
	}
}

func TestVersionSemanticsAndExplicitDowngrade(t *testing.T) {
	for _, pair := range [][2]string{{"v1.0.0-rc.2", "v1.0.0-rc.10"}, {"v1.0.0-rc.10", "v1.0.0"}, {"v1.9.9", "v1.10.0"}, {"v1.0.0-2", "v1.0.0-alpha"}} {
		if Compare(pair[0], pair[1]) >= 0 {
			t.Fatalf("wrong semver ordering %v", pair)
		}
	}
	f := newFixture(t)
	f.options.Current.Version = "v2.0.0"
	result := Run(context.Background(), f.options)
	if result.Available || result.Updated || result.ExitCode != 0 {
		t.Fatalf("automatic downgrade: %+v", result)
	}
	f.options.To = f.release.Tag
	result = Run(context.Background(), f.options)
	if !result.Updated || result.ExitCode != 0 {
		t.Fatalf("explicit downgrade failed: %+v", result)
	}
}

func TestConcurrentUpdateDetectsReplacedInstallation(t *testing.T) {
	f := newFixture(t)
	var group sync.WaitGroup
	results := make([]Report, 2)
	for index := range results {
		group.Go(func() { results[index] = Run(context.Background(), f.options) })
	}
	group.Wait()
	updated := 0
	for _, result := range results {
		if result.Updated {
			updated++
		}
	}
	if updated != 1 {
		t.Fatalf("concurrent replacements: %+v", results)
	}
}

func TestCancellationAndArchivePathValidation(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result := Run(ctx, f.options); result.ExitCode != 130 || result.Updated {
		t.Fatalf("%+v", result)
	}
	for _, header := range []*tar.Header{{Name: "../outside", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg}, {Name: "link", Linkname: "/tmp/outside", Typeflag: tar.TypeSymlink}, {Name: "phpwarden", Mode: 0o755, Size: 1, Typeflag: tar.TypeReg}} {
		data, _ := archiveFixture(t, "v1.2.0", "linux/amd64", header)
		if _, _, err := unpack(data, "v1.2.0", "linux/amd64"); err == nil {
			t.Fatalf("unsafe header accepted %+v", header)
		}
	}
	transport := NewClient().HTTP.CheckRedirect
	request, _ := http.NewRequest("GET", "http://evil.test/archive", nil)
	if transport(request, nil) == nil {
		t.Fatal("unsafe redirect accepted")
	}
	f.options.Client.HTTP.Transport = transportFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })
	if result := Run(context.Background(), f.options); result.ExitCode != 3 {
		t.Fatalf("offline %+v", result)
	}
}

func TestReleaseIdentityAndAssetValidation(t *testing.T) {
	for _, which := range []string{"draft", "prerelease", "prerelease tag", "wrong URL", "duplicate asset", "wrong requested tag"} {
		t.Run(which, func(t *testing.T) {
			f := newFixture(t)
			f.options.Check = true
			switch which {
			case "draft":
				f.release.Draft = true
			case "prerelease":
				f.release.Prerelease = true
			case "prerelease tag":
				f.release.Tag = "v1.2.0-rc.1"
			case "wrong URL":
				f.release.Assets[0].URL = "https://example.test/binary"
			case "duplicate asset":
				f.release.Assets = append(f.release.Assets, f.release.Assets[0])
			case "wrong requested tag":
				f.options.To = "v1.3.0"
			}
			if result := Run(context.Background(), f.options); result.ExitCode == 0 || result.Updated {
				t.Fatalf("%+v", result)
			}
		})
	}
	for _, name := range []string{"/usr/bin/phpwarden", "/bin/phpwarden", "/project/vendor/bin/phpwarden", "/project/node_modules/phpwarden/bin/phpwarden", "/nix/store/a/bin/phpwarden", "/opt/homebrew/Cellar/phpwarden/1/bin/phpwarden"} {
		if !managedPath(name) {
			t.Fatalf("managed path not recognized %s", name)
		}
	}
	if managedPath("/home/user/.local/bin/phpwarden") {
		t.Fatal("user-owned installation rejected")
	}
}
