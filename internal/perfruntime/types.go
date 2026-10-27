package perfruntime

import (
	"context"
	"github.com/rchehd/phpwarden/internal/runner"
)

const containerReports = "/var/lib/phpwarden-spx"
const ownerLabel = "io.phpwarden.spx.owner"
const producerLabel = "io.phpwarden.spx.producer"

type Options struct {
	Project, StateDirectory, Environment, Service, Container, PHP, Image, SourceArchive string
	ComposeFiles, EnvFiles, Profiles                                                    []string
	ComposeProject, ContainerRoot, HTTPAllow                                            string
	HTTP, Apply                                                                         bool
	Process                                                                             runner.Process
	readArchive                                                                         func(context.Context, string) ([]byte, error)
}

type Report struct {
	Version         int      `json:"version"`
	Command         string   `json:"command"`
	State           string   `json:"state"`
	Environment     string   `json:"environment,omitempty"`
	Prepared        bool     `json:"prepared"`
	Loaded          bool     `json:"loaded"`
	RuntimeChecked  bool     `json:"runtime_checked"`
	ExitCode        int      `json:"exit_code"`
	Project         string   `json:"project,omitempty"`
	Reports         string   `json:"reports,omitempty"`
	HTTPKeyFile     string   `json:"http_key_file,omitempty"`
	Image           string   `json:"image,omitempty"`
	LaunchArguments []string `json:"launch_arguments,omitempty"`
	ReadCommand     []string `json:"read_command,omitempty"`
	NextSteps       []string `json:"next_steps"`
	Diagnostics     []string `json:"diagnostics"`
}

type binding struct {
	Environment    string   `json:"environment"`
	PHP            string   `json:"php"`
	Service        string   `json:"service,omitempty"`
	Container      string   `json:"container,omitempty"`
	BaseID         string   `json:"base_id,omitempty"`
	BaseUser       string   `json:"base_user,omitempty"`
	ComposeFiles   []string `json:"compose_files,omitempty"`
	EnvFiles       []string `json:"env_files,omitempty"`
	Profiles       []string `json:"profiles,omitempty"`
	ComposeProject string   `json:"compose_project,omitempty"`
	ComposeHash    string   `json:"compose_hash,omitempty"`
	ContainerRoot  string   `json:"container_root"`
	HTTP           bool     `json:"http"`
	HTTPAllow      string   `json:"http_allow,omitempty"`
}

type receipt struct {
	Version      int               `json:"version"`
	Project      string            `json:"project"`
	Owner        string            `json:"owner"`
	Producer     string            `json:"producer"`
	Binding      binding           `json:"binding"`
	Phase        string            `json:"phase"`
	Files        map[string]string `json:"files"`
	Inputs       map[string]string `json:"inputs"`
	ExcludeFile  string            `json:"exclude_file,omitempty"`
	ExcludeAdded bool              `json:"exclude_added"`
	RepoPrefix   string            `json:"repo_prefix,omitempty"`
	ImageID      string            `json:"image_id,omitempty"`
	Restored     bool              `json:"restored"`
}

type workspace struct {
	project, directory, owner string
	process                   runner.Process
	state                     receipt
}

var projectNames = []string{
	".ddev/web-build/Dockerfile.phpwarden-spx", ".ddev/web-build/phpwarden-spx-build.sh",
	".ddev/web-build/phpwarden-spx.tar.gz", ".ddev/docker-compose.phpwarden-spx.yaml",
}

var privateNames = []string{"Dockerfile", "build.sh", "spx.tar.gz", "spx.ini", "compose.yaml", "http.key", ".dockerignore"}
