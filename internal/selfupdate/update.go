package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	buildinfo "github.com/rchehd/phpwarden"
	"github.com/rchehd/phpwarden/internal/cache"
	"github.com/rchehd/phpwarden/internal/progress"
	"github.com/rchehd/phpwarden/internal/runner"
)

type Options struct {
	Check      bool
	To         string
	Executable string
	Current    buildinfo.Info
	Client     Client
	Process    runner.Process
}

type Report struct {
	Version      int    `json:"version"`
	Command      string `json:"command"`
	Current      string `json:"current_version"`
	Target       string `json:"target_version,omitempty"`
	Available    bool   `json:"available"`
	Updated      bool   `json:"updated"`
	Installation string `json:"installation"`
	ExitCode     int    `json:"exit_code"`
	Message      string `json:"message"`
}

func Run(ctx context.Context, options Options) Report {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	result := Report{Version: 1, Command: "update", Current: options.Current.Version, Installation: options.Current.Distribution}
	fail := func(err error) Report {
		result.ExitCode = 3
		result.Message = err.Error()
		if ctx.Err() == context.Canceled {
			result.ExitCode = 130
		}
		return result
	}
	if options.To != "" {
		var err error
		options.To, err = Version(options.To)
		if err != nil {
			result.ExitCode = 2
			result.Message = err.Error()
			return result
		}
	}
	if options.Current.Platform != "linux/amd64" {
		return fail(fmt.Errorf("standalone updates currently support linux/amd64 only"))
	}
	if options.Client.HTTP == nil {
		options.Client = NewClient()
	}
	if options.Process == nil {
		options.Process = runner.Exec{}
	}
	var target string
	if !options.Check {
		if options.Current.Distribution != "standalone" || options.Current.ManifestSHA256 == "" {
			return fail(fmt.Errorf("this %s installation is not self-managed; update it through its source checkout, package manager or container image", options.Current.Distribution))
		}
		if _, err := Version(options.Current.Version); err != nil {
			return fail(fmt.Errorf("installed CLI has no valid release version"))
		}
		executable := options.Executable
		var err error
		if executable == "" {
			executable, err = os.Executable()
			if err != nil {
				return fail(err)
			}
		}
		target, err = filepath.EvalSymlinks(executable)
		if err != nil {
			return fail(err)
		}
		if managedPath(target) {
			return fail(fmt.Errorf("this path belongs to a managed installation; use its package manager"))
		}
	}
	progress.Stage(ctx, "Checking PHPWarden releases")
	release, err := options.Client.Release(ctx, options.To)
	if err != nil {
		return fail(err)
	}
	result.Target = release.Tag
	archiveName := "phpwarden-" + strings.ReplaceAll(options.Current.Platform, "/", "-") + ".tar.gz"
	archiveAsset, err := release.asset(archiveName)
	if err != nil {
		return fail(err)
	}
	checksumAsset, err := release.asset("SHA256SUMS")
	if err != nil {
		return fail(err)
	}
	current, currentErr := Version(options.Current.Version)
	result.Available = currentErr != nil || Compare(release.Tag, current) > 0
	if options.To != "" {
		result.Available = currentErr != nil || release.Tag != current
	}
	if options.Check {
		result.Message = "Release information checked; no files were changed."
		return result
	}
	if !result.Available {
		result.Message = "The installed CLI already satisfies this update request."
		return result
	}
	parent, name := filepath.Dir(target), filepath.Base(target)
	progress.Stage(ctx, "Waiting for CLI update lock")
	unlock, err := cache.LockProject(ctx, parent)
	if err != nil {
		return fail(fmt.Errorf("cannot lock installation directory: %w", err))
	}
	defer unlock()
	root, err := os.OpenRoot(parent)
	if err != nil {
		return fail(err)
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Size() > MaxArchive || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return fail(fmt.Errorf("installed executable must be a regular file without special permissions"))
	}
	before, err := root.ReadFile(name)
	if err != nil {
		return fail(err)
	}
	if len(before) > MaxArchive {
		return fail(fmt.Errorf("installed executable exceeds update limit"))
	}
	beforeHash := sha256.Sum256(before)
	if !probeIdentity(ctx, options.Process, target, parent, options.Current) {
		return fail(fmt.Errorf("installed CLI identity changed; rerun update using the current executable"))
	}
	temporary, err := os.CreateTemp(parent, ".phpwarden-update-*")
	if err != nil {
		return fail(fmt.Errorf("installation directory is not writable; install into a user-owned bin directory or use its installer"))
	}
	tempName := temporary.Name()
	defer os.Remove(tempName)
	defer temporary.Close()
	progress.Stage(ctx, "Downloading release checksums")
	sums, err := options.Client.Download(ctx, checksumAsset, 64<<10)
	if err != nil {
		return fail(err)
	}
	expected, err := checksum(sums, archiveName)
	if err != nil {
		return fail(err)
	}
	progress.Stage(ctx, "Downloading PHPWarden "+release.Tag)
	archive, err := options.Client.Download(ctx, archiveAsset, MaxArchive)
	if err != nil {
		return fail(err)
	}
	if err := verifyArchive(archive, expected, archiveAsset.Digest); err != nil {
		return fail(err)
	}
	progress.Stage(ctx, "Validating downloaded CLI and compatibility manifest")
	binary, metadata, err := unpack(archive, release.Tag, options.Current.Platform)
	if err != nil {
		return fail(err)
	}
	if _, err := temporary.Write(binary); err != nil {
		return fail(err)
	}
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		return fail(err)
	}
	if err := temporary.Sync(); err != nil {
		return fail(err)
	}
	if err := temporary.Close(); err != nil {
		return fail(err)
	}
	if !probeIdentity(ctx, options.Process, tempName, parent, metadata) {
		return fail(fmt.Errorf("downloaded CLI identity does not match its release metadata"))
	}
	currentBytes, err := root.ReadFile(name)
	currentInfo, statErr := root.Lstat(name)
	if err != nil || statErr != nil || !os.SameFile(info, currentInfo) || currentInfo.Mode() != info.Mode() || sha256.Sum256(currentBytes) != beforeHash {
		return fail(fmt.Errorf("installed executable changed during update; rerun using the current CLI"))
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	progress.Stage(ctx, "Installing verified PHPWarden release")
	if err := root.Rename(filepath.Base(tempName), name); err != nil {
		return fail(fmt.Errorf("cannot atomically replace installed CLI: %w", err))
	}
	result.Updated = true
	result.Message = "PHPWarden updated. Project settings and agent files were preserved; the next check downloads its required worker image if missing."
	return result
}

func managedPath(name string) bool {
	name = filepath.ToSlash(name)
	for _, part := range []string{"/vendor/", "/node_modules/", "/Cellar/", "/nix/store/", "/snap/"} {
		if strings.Contains(name, part) {
			return true
		}
	}
	return filepath.Dir(name) == "/usr/bin" || filepath.Dir(name) == "/bin"
}

func SameInfo(data []byte, want buildinfo.Info) bool {
	var actual buildinfo.Info
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&actual) != nil || actual != want {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}

func probeIdentity(ctx context.Context, process runner.Process, executable, directory string, expected buildinfo.Info) bool {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result := process.Run(ctx, runner.Command{Name: executable, Args: []string{"--build-info"}, Directory: directory})
	return result.Err == nil && result.ExitCode == 0 && !result.Unavailable && !result.TimedOut && ctx.Err() == nil && len(result.Stdout) <= 1<<20 && len(result.Stderr) == 0 && SameInfo(result.Stdout, expected)
}
