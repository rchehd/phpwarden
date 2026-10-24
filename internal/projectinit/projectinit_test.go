package projectinit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
)

func writeFile(t *testing.T, root, name, data string) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// drupalProject creates a Drupal 11 layout whose locked acme/lib package requires libraryPHP.
func drupalProject(t *testing.T, libraryPHP string, files map[string]string) string {
	t.Helper()
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("development images exist only for linux/amd64")
	}
	root := t.TempDir()
	writeFile(t, root, "composer.json", `{"require":{"drupal/core":"^11"}}`)
	writeFile(t, root, "composer.lock", `{"packages":[{"name":"drupal/core","version":"11.3.16","require":{"php":">=8.1"}},`+
		`{"name":"acme/lib","version":"1.0.0","require":{"php":"`+libraryPHP+`"}}],"packages-dev":[]}`)
	writeFile(t, root, "web/core/lib/Drupal.php", "<?php\n")
	if err := os.MkdirAll(filepath.Join(root, "web/modules/custom"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		writeFile(t, root, name, data)
	}
	return root
}

func TestDetectsRuntimeAndTarget(t *testing.T) {
	platform := func(version string) string {
		return `{"require":{"drupal/core":"^11"},"config":{"platform":{"php":"` + version + `"}}}`
	}
	for _, test := range []struct {
		name, library string
		files         map[string]string
		options       Options
		runtime       Choice
		target        Choice
	}{
		{"DDEV override wins over platform", ">=8.1", map[string]string{".ddev/config.yaml": "php_version: \"8.3\"\n", ".ddev/config.local.yaml": "php_version: 8.2\n", "composer.json": platform("8.1.0")}, Options{},
			Choice{"8.2", ".ddev/config.local.yaml php_version"}, Choice{"8.1", "composer.json config.platform.php"}},
		{"platform", ">=8.1", map[string]string{".ddev/config.yaml": "name: site\n", "composer.json": platform("8.2.0")}, Options{},
			Choice{"8.2", "composer.json config.platform.php"}, Choice{"8.2", "composer.json config.platform.php"}},
		{"lowest runtime the lock allows", ">=8.2", nil, Options{},
			Choice{"8.2", "composer.lock, the lowest supported PHP all packages allow"}, Choice{"8.2", "php.runtime"}},
		{"flags", ">=8.1", map[string]string{".ddev/config.yaml": "php_version: \"8.2\"\n"}, Options{Runtime: "8.4", Target: "8.3"},
			Choice{"8.4", "--php-runtime"}, Choice{"8.3", "--php-target"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := drupalProject(t, test.library, test.files)
			test.options.CWD = filepath.Join(root, "web/modules/custom")
			test.options.Project = root
			result, err := Run(context.Background(), test.options)
			if err != nil {
				t.Fatal(err)
			}
			if !result.Created || *result.Runtime != test.runtime || *result.Target != test.target || result.Image != "phpwarden:static-php"+test.runtime.Version+"-dev" {
				t.Fatalf("wrong result: %+v %+v %+v", result, result.Runtime, result.Target)
			}
			effective, err := config.Load(result.Location)
			if err != nil {
				t.Fatal(err)
			}
			php := effective.Config.PHP
			if php.Runtime != test.runtime.Version || php.Target.Min != test.target.Version || php.Target.Max != test.target.Version {
				t.Fatalf("written configuration differs: %+v", php)
			}
			if !effective.Config.Tools.DeadCode.Enabled {
				t.Fatal("init disabled the profile's dead-code check")
			}
		})
	}
}

func TestRejectsProjectsItCannotConfigure(t *testing.T) {
	for _, test := range []struct {
		name, library string
		files         map[string]string
		options       Options
		want          string
	}{
		{"detected runtime conflicts with the lock", "~8.1.0", map[string]string{".ddev/config.yaml": "php_version: \"8.3\"\n"}, Options{}, "fit runtime 8.1; pass --php-runtime 8.1"},
		{"unsupported runtime", ">=8.1", nil, Options{Runtime: "8.5"}, "pass --php-runtime 8.1"},
		{"target above runtime", ">=8.1", nil, Options{Runtime: "8.2", Target: "8.3"}, "pass --php-runtime 8.3"},
		{"malformed DDEV version", ">=8.1", map[string]string{".ddev/config.yaml": "php_version: \"8\"\n"}, Options{}, "php_version must be MAJOR.MINOR"},
		{"no supported runtime fits", "<8.1", nil, Options{}, "no supported runtime 8.1, 8.2, 8.3, 8.4"},
		{"not Drupal", ">=8.1", map[string]string{"composer.lock": `{"packages":[]}`}, Options{}, "not a Drupal project"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := drupalProject(t, test.library, test.files)
			if test.name == "not Drupal" {
				if err := os.RemoveAll(filepath.Join(root, "web")); err != nil {
					t.Fatal(err)
				}
			}
			test.options.CWD = root
			_, err := Run(context.Background(), test.options)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q, got %v", test.want, err)
			}
			if _, err := os.Lstat(filepath.Join(root, config.Filename)); !os.IsNotExist(err) {
				t.Fatal("configuration written for a rejected project")
			}
		})
	}
	if _, err := Run(context.Background(), Options{CWD: t.TempDir()}); err == nil || !strings.Contains(err.Error(), "not a Drupal project") {
		t.Fatalf("directory without composer.json accepted: %v", err)
	}
}

func TestNeverOverwritesConfiguration(t *testing.T) {
	root := drupalProject(t, ">=8.1", nil)
	invalid := "version: 1\nprofile: laravel\n"
	writeFile(t, root, config.Filename, invalid)
	if _, err := Run(context.Background(), Options{CWD: root}); err == nil || !strings.Contains(err.Error(), "existing .phpwarden.yaml is invalid") {
		t.Fatalf("invalid configuration accepted: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(root, config.Filename)); string(data) != invalid {
		t.Fatal("invalid configuration was overwritten")
	}
	if err := os.Remove(filepath.Join(root, config.Filename)); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	results := make([]Result, 8)
	errs := make([]error, 8)
	for i := range results {
		group.Go(func() { results[i], errs[i] = Run(context.Background(), Options{CWD: root}) })
	}
	group.Wait()
	created := 0
	for i, result := range results {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		if result.Created {
			created++
		}
	}
	entries, _ := os.ReadDir(root)
	info, err := os.Stat(filepath.Join(root, config.Filename))
	if created != 1 || len(entries) != 4 || err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("concurrent init created %d files, left %d entries: %v", created, len(entries), err)
	}
}

func TestPreCommitOptInCreatesConfigAndHookTogether(t *testing.T) {
	root := drupalProject(t, ">=8.1", nil)
	gitRepository(t, root)
	result, err := Run(context.Background(), Options{CWD: root, PreCommit: true})
	if err != nil || !result.Created || result.PreCommit == nil || !result.PreCommit.Created {
		t.Fatalf("initialization with hook: %+v %v", result, err)
	}
	before, err := os.ReadFile(result.PreCommit.Path)
	if err != nil {
		t.Fatal(err)
	}
	withoutHook, err := Run(context.Background(), Options{CWD: root})
	if err != nil || withoutHook.Created || withoutHook.PreCommit != nil {
		t.Fatalf("plain init changed hook installation state: %+v %v", withoutHook, err)
	}
	if after, _ := os.ReadFile(result.PreCommit.Path); string(before) != string(after) {
		t.Fatal("plain init changed the installed hook")
	}
}

func TestHookConflictIsDetectedBeforeWritingNewConfiguration(t *testing.T) {
	root := drupalProject(t, ">=8.1", nil)
	gitRepository(t, root)
	foreign := "#!/bin/sh\nexit 0\n"
	writeFile(t, root, ".git/hooks/pre-commit", foreign)
	if _, err := Run(context.Background(), Options{CWD: root, PreCommit: true}); err == nil || !strings.Contains(err.Error(), "existing pre-commit hook") {
		t.Fatalf("hook conflict was not reported: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, config.Filename)); !os.IsNotExist(err) {
		t.Fatal("a hook conflict still created the configuration")
	}
	if data, _ := os.ReadFile(filepath.Join(root, ".git/hooks/pre-commit")); string(data) != foreign {
		t.Fatal("hook conflict overwrote the existing hook")
	}
}

func gitRepository(t *testing.T, repository string) {
	t.Helper()
	for _, args := range [][]string{{"init", "--quiet"}, {"add", "--all"}, {"-c", "user.name=test", "-c", "user.email=test@example.com", "commit", "--quiet", "--allow-empty", "-m", "init"},
		{"update-ref", "refs/remotes/origin/develop", "HEAD"}, {"symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/develop"}} {
		if output, err := exec.Command("git", append([]string{"-C", repository}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, output)
		}
	}
}

func TestMonorepoConfigAtGitRootWithComposerRoot(t *testing.T) {
	repository := t.TempDir()
	root := filepath.Join(repository, "platform")
	source := drupalProject(t, ">=8.1", map[string]string{
		"composer.json": `{"require":{"drupal/core":"^11"},"repositories":{"profile":{"type":"path","url":"web/profiles/custom/demo_profile"},` +
			`"packages":{"type":"path","url":"../packages/*"},"outside":{"type":"path","url":"../.."}}}`,
		"phpcs.xml.dist": "<ruleset/>\n", "phpstan.neon": "parameters: {}\n",
	})
	if err := os.Rename(source, root); err != nil {
		t.Fatal(err)
	}
	writeFile(t, repository, ".ddev/config.yaml", "php_version: \"8.3\"\n")
	writeFile(t, repository, "profile/composer.json", `{"name":"acme/demo_profile","type":"drupal-custom-profile"}`)
	writeFile(t, repository, "packages/feature/composer.json", `{"name":"acme/feature","type":"drupal-custom-module"}`)
	writeFile(t, repository, "packages/plugin/composer.json", `{"name":"acme/plugin","type":"composer-plugin"}`)
	if err := os.MkdirAll(filepath.Join(root, "web/profiles/custom"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../../../profile", filepath.Join(root, "web/profiles/custom/demo_profile")); err != nil {
		t.Fatal(err)
	}
	gitRepository(t, repository)
	result, err := Run(context.Background(), Options{CWD: filepath.Join(root, "web/modules/custom")})
	if err != nil {
		t.Fatal(err)
	}
	resolved, _ := filepath.EvalSymlinks(repository)
	natives := []NativeConfig{{"phpcs", "platform/phpcs.xml.dist"}, {"phpstan", "platform/phpstan.neon"}}
	if result.Location.Root != resolved || result.ComposerRoot != "platform" || *result.Runtime != (Choice{"8.3", ".ddev/config.yaml php_version"}) ||
		result.Base == nil || *result.Base != "origin/develop" || !slices.Equal(result.NativeConfigs, natives) {
		t.Fatalf("wrong monorepo result: %+v %+v", result, result.Runtime)
	}
	if !slices.Equal(result.Project.SourceRoots, []string{"packages/feature", "platform/web/modules/custom", "platform/web/profiles/custom", "profile"}) ||
		!slices.ContainsFunc(result.Project.Warnings, func(warning string) bool { return strings.Contains(warning, `"../.." is outside the project root`) }) {
		t.Fatalf("path repositories not discovered: %+v", result.Project)
	}
	effective, err := config.Load(result.Location)
	if err != nil || effective.Config.ComposerRoot != "platform" {
		t.Fatalf("written configuration lacks composer_root: %+v %v", effective.Config, err)
	}
	if result, err := Run(context.Background(), Options{CWD: repository}); err != nil || result.Created {
		t.Fatalf("second init must keep the configuration: %+v %v", result, err)
	}
	other := drupalProject(t, ">=8.1", nil)
	if err := os.Rename(other, filepath.Join(repository, "other")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(repository, config.Filename)); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), Options{CWD: repository}); err == nil || !strings.Contains(err.Error(), "several Drupal Composer projects found (other, platform)") {
		t.Fatalf("ambiguous monorepo accepted: %v", err)
	}
	if result, err := Run(context.Background(), Options{CWD: filepath.Join(repository, "other", "web")}); err != nil || result.ComposerRoot != "other" {
		t.Fatalf("working directory did not choose the project: %+v %v", result, err)
	}
}
