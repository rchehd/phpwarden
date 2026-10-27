package perfruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rchehd/phpwarden/internal/runner"
	"go.yaml.in/yaml/v3"
)

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
var imagePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_./:@-]*$`)
var userPattern = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.:-]*$`)

func (w *workspace) command(ctx context.Context, name string, args ...string) ([]byte, error) {
	result := w.process.Run(ctx, runner.Command{Name: name, Args: args, Directory: w.project})
	if result.Err != nil || result.ExitCode != 0 {
		return nil, fmt.Errorf("%s operation failed; check the selected local runtime separately", name)
	}
	if len(result.Stdout) > 4<<20 {
		return nil, fmt.Errorf("runtime response exceeds its limit")
	}
	return result.Stdout, nil
}

func (w *workspace) localDocker(ctx context.Context) error {
	endpoint := os.Getenv("DOCKER_HOST")
	if endpoint == "" || os.Getenv("DOCKER_CONTEXT") != "" {
		data, err := w.command(ctx, "docker", "context", "inspect", "--format", "{{json .Endpoints.docker.Host}}")
		if err != nil {
			return err
		}
		if json.Unmarshal(data, &endpoint) != nil {
			return fmt.Errorf("cannot resolve Docker endpoint")
		}
	}
	if !strings.HasPrefix(endpoint, "unix:///") {
		return fmt.Errorf("performance integration requires a local Docker Unix socket")
	}
	return nil
}

func (w *workspace) composeArgs(overlay bool) []string {
	b := w.state.Binding
	args := []string{"compose", "--project-directory", w.project}
	for _, file := range b.ComposeFiles {
		args = append(args, "-f", file)
	}
	if overlay {
		args = append(args, "-f", filepath.Join(w.directory, "compose.yaml"))
	}
	if b.ComposeProject != "" {
		args = append(args, "-p", b.ComposeProject)
	}
	for _, file := range b.EnvFiles {
		args = append(args, "--env-file", file)
	}
	for _, profile := range b.Profiles {
		args = append(args, "--profile", profile)
	}
	return args
}

type composeConfig struct {
	Name     string `json:"name"`
	Services map[string]struct {
		Image            string `json:"image"`
		User             string `json:"user"`
		WorkingDirectory string `json:"working_dir"`
	} `json:"services"`
}

func (w *workspace) composeConfig(ctx context.Context) (composeConfig, string, error) {
	var result composeConfig
	data, err := w.command(ctx, "docker", append(w.composeArgs(false), "config", "--format", "json")...)
	if err != nil {
		return result, "", err
	}
	var normalized any
	if json.Unmarshal(data, &normalized) != nil || json.Unmarshal(data, &result) != nil {
		return result, "", fmt.Errorf("invalid Compose configuration response")
	}
	canonical, _ := json.Marshal(normalized)
	return result, digest(canonical), nil
}

type imageInfo struct {
	ID           string `json:"Id"`
	OS           string `json:"Os"`
	Architecture string `json:"Architecture"`
	Config       struct {
		User   string            `json:"User"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
}

func (w *workspace) inspectImage(ctx context.Context, name string) (imageInfo, error) {
	var result []imageInfo
	if !imagePattern.MatchString(name) {
		return imageInfo{}, fmt.Errorf("invalid local image reference")
	}
	data, err := w.command(ctx, "docker", "image", "inspect", name)
	if err != nil {
		return imageInfo{}, err
	}
	if json.Unmarshal(data, &result) != nil || len(result) != 1 || !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(result[0].ID) || result[0].OS != "linux" || result[0].Architecture != "amd64" {
		return imageInfo{}, fmt.Errorf("SPX runtime currently requires a local Linux amd64 image")
	}
	if result[0].Config.Labels[producerLabel] != "" {
		return imageInfo{}, fmt.Errorf("the base image already contains a managed SPX build")
	}
	if result[0].Config.User != "" && !userPattern.MatchString(result[0].Config.User) {
		return imageInfo{}, fmt.Errorf("unsupported base image user")
	}
	return result[0], nil
}

type containerInfo struct {
	ID    string `json:"Id"`
	Image string `json:"Image"`
	State struct {
		Running bool `json:"Running"`
	} `json:"State"`
	Config struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	Mounts []struct {
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
		RW          bool   `json:"RW"`
	} `json:"Mounts"`
}

func (w *workspace) container(ctx context.Context) (containerInfo, error) {
	b := w.state.Binding
	name := b.Container
	if b.Environment == "ddev" {
		name = "ddev-" + b.ComposeProject + "-web"
	}
	if b.Environment == "compose" {
		data, err := w.command(ctx, "docker", append(w.composeArgs(false), "ps", "--all", "--quiet", b.Service)...)
		if err != nil {
			return containerInfo{}, err
		}
		ids := strings.Fields(string(data))
		if len(ids) == 0 {
			return containerInfo{}, nil
		}
		if len(ids) != 1 {
			return containerInfo{}, fmt.Errorf("select a single PHP container; replicated services are not supported yet")
		}
		name = ids[0]
	}
	if name == "" {
		return containerInfo{}, nil
	}
	result := w.process.Run(ctx, runner.Command{Name: "docker", Args: []string{"container", "inspect", name}, Directory: w.project})
	if result.Err != nil || result.ExitCode != 0 {
		if strings.Contains(string(result.Stderr), "No such container") || strings.Contains(string(result.Stderr), "No such object") {
			return containerInfo{}, nil
		}
		return containerInfo{}, fmt.Errorf("cannot inspect the selected PHP container")
	}
	var rows []containerInfo
	if len(result.Stdout) > 4<<20 || json.Unmarshal(result.Stdout, &rows) != nil || len(rows) != 1 {
		return containerInfo{}, fmt.Errorf("invalid container inspection response")
	}
	if b.Environment == "ddev" && rows[0].Config.Labels["com.ddev.approot"] != w.project {
		return containerInfo{}, fmt.Errorf("DDEV container belongs to another checkout")
	}
	return rows[0], nil
}

func absoluteFiles(project string, names []string) ([]string, error) {
	result := []string{}
	for _, name := range names {
		if !filepath.IsAbs(name) {
			name = filepath.Join(project, name)
		}
		path, err := canonical(name)
		if err != nil {
			return nil, fmt.Errorf("cannot resolve a runtime input file")
		}
		result = append(result, path)
	}
	return result, nil
}

func (w *workspace) discover(ctx context.Context, options Options) error {
	b := binding{Environment: options.Environment, PHP: options.PHP, Container: options.Container, Service: options.Service, Profiles: options.Profiles, ComposeProject: options.ComposeProject, ContainerRoot: options.ContainerRoot, HTTP: options.HTTP, HTTPAllow: options.HTTPAllow}
	if b.Environment == "" {
		if _, err := os.Stat(filepath.Join(w.project, ".ddev/config.yaml")); err == nil {
			b.Environment = "ddev"
		} else if len(options.ComposeFiles) > 0 {
			b.Environment = "compose"
		} else if options.Container != "" {
			b.Environment = "docker"
		} else {
			return fmt.Errorf("select --environment ddev, compose or docker explicitly")
		}
	}
	if b.Environment != "ddev" && b.Environment != "compose" && b.Environment != "docker" {
		return fmt.Errorf("unsupported performance environment")
	}
	if b.Environment == "ddev" {
		if options.Image != "" || options.Container != "" || options.Service != "" || len(options.ComposeFiles) > 0 || len(options.EnvFiles) > 0 || len(options.Profiles) > 0 || options.ComposeProject != "" {
			return fmt.Errorf("DDEV manages its own PHP service and Compose invocation")
		}
		data, err := readRegular(filepath.Join(w.project, ".ddev/config.yaml"), 128<<10)
		if err != nil {
			return fmt.Errorf("cannot read DDEV configuration")
		}
		var config struct {
			Name string `yaml:"name"`
			PHP  string `yaml:"php_version"`
		}
		if yaml.Unmarshal(data, &config) != nil || !namePattern.MatchString(config.Name) {
			return fmt.Errorf("DDEV configuration needs an explicit project name")
		}
		if b.PHP != "" && b.PHP != config.PHP {
			return fmt.Errorf("--php conflicts with DDEV configuration")
		}
		b.PHP = config.PHP
		b.Service = "web"
		b.ComposeProject = config.Name
		if b.ContainerRoot == "" {
			b.ContainerRoot = "/var/www/html"
		}
		inputs, err := filepath.Glob(filepath.Join(w.project, ".ddev/config*.yaml"))
		if err != nil {
			return err
		}
		for _, name := range inputs {
			raw, err := readRegular(name, 128<<10)
			if err != nil {
				return fmt.Errorf("unsafe DDEV configuration")
			}
			var extra struct {
				PHP  string `yaml:"php_version"`
				Name string `yaml:"name"`
			}
			if yaml.Unmarshal(raw, &extra) != nil || extra.PHP != "" && extra.PHP != b.PHP || extra.Name != "" && extra.Name != b.ComposeProject {
				return fmt.Errorf("DDEV overrides change PHP or project identity; reconcile them before setup")
			}
		}
		if err := w.fingerprint(inputs); err != nil {
			return err
		}
		parent := filepath.Join(w.project, ".ddev/web-build")
		if err := os.Mkdir(parent, 0755); err != nil && !os.IsExist(err) {
			return fmt.Errorf("cannot create the DDEV build directory")
		}
		tracked, err := w.command(ctx, "git", append([]string{"ls-files", "-z", "--"}, projectNames...)...)
		if err != nil {
			return fmt.Errorf("DDEV integration requires a Git checkout for local exclusions")
		}
		if len(tracked) > 0 {
			return fmt.Errorf("a reserved DDEV adapter path is tracked; it will not be replaced")
		}
		gitRoot, err := w.command(ctx, "git", "rev-parse", "--show-toplevel")
		if err != nil {
			return err
		}
		prefix, err := filepath.Rel(strings.TrimSpace(string(gitRoot)), w.project)
		if err != nil || !inside(strings.TrimSpace(string(gitRoot)), w.project) {
			return fmt.Errorf("cannot resolve the checkout-relative DDEV path")
		}
		if prefix != "." {
			w.state.RepoPrefix = filepath.ToSlash(prefix) + "/"
		}
	} else {
		if err := w.localDocker(ctx); err != nil {
			return err
		}
		if b.Environment == "compose" {
			if b.Container != "" {
				return fmt.Errorf("Compose uses --service rather than --container")
			}
			if len(options.ComposeFiles) == 0 || !namePattern.MatchString(b.Service) {
				return fmt.Errorf("Compose requires explicit --compose-file and --service")
			}
			if os.Getenv("COMPOSE_FILE") != "" || os.Getenv("COMPOSE_PROFILES") != "" {
				return fmt.Errorf("pass Compose files/profiles explicitly and unset COMPOSE_FILE/COMPOSE_PROFILES")
			}
			var err error
			b.ComposeFiles, err = absoluteFiles(w.project, options.ComposeFiles)
			if err != nil {
				return err
			}
			b.EnvFiles, err = absoluteFiles(w.project, options.EnvFiles)
			if err != nil {
				return err
			}
			if len(b.EnvFiles) == 0 {
				if _, err := os.Lstat(filepath.Join(w.project, ".env")); err == nil {
					b.EnvFiles = []string{filepath.Join(w.project, ".env")}
				}
			}
			w.state.Binding = b
			config, hash, err := w.composeConfig(ctx)
			if err != nil {
				return err
			}
			service, ok := config.Services[b.Service]
			if !ok {
				return fmt.Errorf("the selected Compose service does not exist")
			}
			if !namePattern.MatchString(config.Name) {
				return fmt.Errorf("Compose needs a valid project name")
			}
			b.ComposeProject = config.Name
			b.ComposeHash = hash
			if b.ContainerRoot == "" {
				b.ContainerRoot = service.WorkingDirectory
			}
			if options.Image == "" {
				options.Image = service.Image
			}
			if err := w.fingerprint(append(append([]string{}, b.ComposeFiles...), b.EnvFiles...)); err != nil {
				return err
			}
			w.state.Binding = b
		}
		if b.Environment == "docker" {
			if !namePattern.MatchString(b.Container) || b.Service != "" || len(options.ComposeFiles) > 0 || len(options.EnvFiles) > 0 || len(b.Profiles) > 0 || b.ComposeProject != "" {
				return fmt.Errorf("bare Docker requires an explicit --container and no Compose options")
			}
		}
		w.state.Binding = b
		if options.Image == "" {
			container, err := w.container(ctx)
			if err != nil {
				return err
			}
			options.Image = container.Image
		}
		if options.Image == "" {
			return fmt.Errorf("provide --image or an existing selected PHP container")
		}
		image, err := w.inspectImage(ctx, options.Image)
		if err != nil {
			return err
		}
		b.BaseID = image.ID
		b.BaseUser = image.Config.User
	}
	if b.PHP != "8.1" && b.PHP != "8.2" && b.PHP != "8.3" && b.PHP != "8.4" {
		return fmt.Errorf("select a supported PHP minor with --php (8.1–8.4)")
	}
	if b.ContainerRoot == "" || !filepath.IsAbs(b.ContainerRoot) || strings.ContainsAny(b.ContainerRoot, "\x00\r\n\\") {
		return fmt.Errorf("provide an absolute --container-root for source mapping")
	}
	if b.HTTPAllow == "" {
		b.HTTPAllow = "127.0.0.1,::1"
	}
	if b.HTTPAllow != "127.0.0.1,::1" && !b.HTTP {
		return fmt.Errorf("--http-allow requires --http")
	}
	for _, value := range strings.Split(b.HTTPAllow, ",") {
		if value == "*" {
			continue
		}
		if _, err := netip.ParseAddr(value); err == nil {
			continue
		}
		if _, err := netip.ParsePrefix(value); err != nil {
			return fmt.Errorf("HTTP allow list must contain IP addresses or CIDR ranges")
		}
	}
	if b.ComposeProject != "" && !namePattern.MatchString(b.ComposeProject) {
		return fmt.Errorf("invalid Compose project name")
	}
	for _, profile := range b.Profiles {
		if !namePattern.MatchString(profile) {
			return fmt.Errorf("invalid Compose profile")
		}
	}
	w.state.Binding = b
	return nil
}
