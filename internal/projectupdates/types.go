package projectupdates

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rchehd/phpwarden/internal/config"
)

type Options struct {
	Security       bool
	Direct         bool
	NoDev          bool
	RuntimeVersion string
}

type Advisory struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Link     string `json:"link"`
	Affected string `json:"affected_versions"`
	Severity string `json:"severity,omitempty"`
	Ignored  bool   `json:"ignored_by_project"`
}

type Package struct {
	Name          string     `json:"name"`
	Current       string     `json:"locked_version"`
	Latest        string     `json:"latest_version,omitempty"`
	UpdateType    string     `json:"update_type,omitempty"`
	Direct        bool       `json:"direct"`
	Dev           bool       `json:"dev"`
	Abandoned     bool       `json:"abandoned"`
	Advisories    []Advisory `json:"advisories"`
	Compatibility string     `json:"compatibility"`
}

type Lifecycle struct {
	Branch        string `json:"branch"`
	ActiveUntil   string `json:"active_until"`
	SecurityUntil string `json:"security_until"`
}

type PHP struct {
	Branch            string      `json:"branch"`
	Current           *string     `json:"runtime_version"`
	Latest            string      `json:"latest_version,omitempty"`
	UpdateAvailable   *bool       `json:"update_available"`
	SecurityRelease   bool        `json:"latest_is_security_release"`
	Support           string      `json:"support"`
	Lifecycle         *Lifecycle  `json:"lifecycle,omitempty"`
	SupportedBranches []Lifecycle `json:"supported_branches"`
	Note              string      `json:"note"`
}

type Source struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	FetchedAt string `json:"fetched_at,omitempty"`
	Note      string `json:"note,omitempty"`
}

type Project struct {
	Ecosystem       string   `json:"ecosystem"`
	ID              string   `json:"id"`
	Current         string   `json:"version"`
	Origin          string   `json:"version_source"`
	Paths           []string `json:"paths"`
	Direct          bool     `json:"direct"`
	Dev             bool     `json:"dev"`
	Status          string   `json:"status"`
	Reasons         []string `json:"reasons"`
	SecurityCovered *bool    `json:"security_covered"`
	ReleaseURL      string   `json:"release_url,omitempty"`
}

type ProfileReport struct {
	Complete bool
	Projects []Project
	Sources  []Source
	Notes    []string
}

type Metadata func(context.Context, string, string, func([]byte) error, func(context.Context) ([]byte, error)) ([]byte, Source, error)
type ProfileFetcher func(context.Context, Metadata) ProfileReport

type Report struct {
	Version  int       `json:"version"`
	Command  string    `json:"command"`
	Complete bool      `json:"complete"`
	ExitCode int       `json:"exit_code"`
	PHP      PHP       `json:"php"`
	Packages []Package `json:"packages"`
	Projects []Project `json:"projects"`
	Sources  []Source  `json:"sources"`
	Notes    []string  `json:"notes"`
}

type ComposerData struct {
	Outdated    json.RawMessage `json:"outdated"`
	Audit       json.RawMessage `json:"audit"`
	Diagnostics bool            `json:"diagnostics"`
}

type ComposerFetcher func(context.Context, []byte, []byte) ([]byte, error)
type HTTPFetcher func(context.Context, string) ([]byte, error)

type Request struct {
	Location     config.Location
	Settings     config.Updates
	Branch       string
	ComposerRoot string
	Toolset      string
	Options      Options
	Composer     ComposerFetcher
	Profile      ProfileFetcher
	HTTP         HTTPFetcher
	Now          time.Time
}
