package runner

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

type Command struct {
	Name      string
	Args      []string
	Directory string
	Input     []byte
}

type Result struct {
	Stdout      []byte
	Stderr      []byte
	ExitCode    int
	Err         error
	Unavailable bool
	TimedOut    bool
}

type Process interface {
	Run(context.Context, Command) Result
}

type Exec struct{}

func (Exec) Run(ctx context.Context, spec Command) Result {
	command := exec.CommandContext(ctx, spec.Name, spec.Args...)
	command.Dir = spec.Directory
	command.Stdin = bytes.NewReader(spec.Input)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	result := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), Err: err}
	if err != nil {
		result.ExitCode = -1
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			result.ExitCode = exit.ExitCode()
		}
		var executable *exec.Error
		result.Unavailable = errors.As(err, &executable) && errors.Is(executable.Err, exec.ErrNotFound)
		result.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
	}
	return result
}
