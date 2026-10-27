package performance

import (
	"os"
	"time"
)

const Producer = "62b228e4f7ca5e87faf6cc1e31bac7ad3c959a07"

type Options struct {
	Directory     string
	Key           string
	Producer      string
	Workspace     string
	ContainerRoot string
	Top           int
	limits        *limits
	directoryRoot *os.Root
	workspaceRoot *os.Root
}

type limits struct {
	metadata   int64
	compressed int64
	decoded    int64
	batch      int64
	reports    int
	events     int
	functions  int
	edges      int
	depth      int
	line       int
	timeout    time.Duration
}

func defaults() limits {
	return limits{metadata: 64 << 10, compressed: 16 << 20, decoded: 64 << 20,
		batch: 64 << 20, reports: 25, events: 1000000, functions: 8192,
		edges: 32768, depth: 512, line: 64 << 10, timeout: 15 * time.Second}
}

type Diagnostic struct {
	Code    string `json:"code"`
	Report  string `json:"report,omitempty"`
	Message string `json:"message"`
}

type Caller struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Calls int64  `json:"calls"`
}

type Function struct {
	ID             int      `json:"id"`
	Name           string   `json:"name"`
	Calls          int64    `json:"calls"`
	SelfMS         float64  `json:"self_ms"`
	InclusiveMS    float64  `json:"inclusive_ms"`
	CPUSelfMS      *float64 `json:"cpu_self_ms,omitempty"`
	Share          float64  `json:"self_percent"`
	File           string   `json:"file,omitempty"`
	Line           int      `json:"definition_line,omitempty"`
	Mapped         bool     `json:"source_mapped"`
	Callers        []Caller `json:"callers"`
	OmittedCallers int      `json:"omitted_callers"`
}

type Profile struct {
	Key              string     `json:"key"`
	RecordedAt       string     `json:"recorded_at"`
	Kind             string     `json:"kind"`
	Description      string     `json:"description"`
	Compression      string     `json:"compression"`
	WallMS           float64    `json:"wall_ms"`
	CPUMS            *float64   `json:"cpu_ms,omitempty"`
	CPUOmittedReason string     `json:"cpu_omitted_reason,omitempty"`
	PeakMemory       int64      `json:"peak_memory_bytes"`
	Calls            int64      `json:"calls"`
	TotalFunctions   int        `json:"total_functions"`
	OmittedFunctions int        `json:"omitted_functions"`
	Functions        []Function `json:"top_functions"`
}

type Report struct {
	Version        int          `json:"version"`
	Command        string       `json:"command"`
	Producer       string       `json:"producer"`
	ProducerOrigin string       `json:"producer_origin"`
	Scope          string       `json:"scope"`
	Complete       bool         `json:"complete"`
	ExitCode       int          `json:"exit_code"`
	Profiles       []Profile    `json:"profiles"`
	Diagnostics    []Diagnostic `json:"diagnostics"`
	Notes          []string     `json:"notes"`
}
