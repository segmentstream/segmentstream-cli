package cli

import (
	"bytes"
	"context"
	"os/exec"
)

type commandInvocation struct {
	Name string
	Args []string
	Dir  string
}

type commandRunner interface {
	LookPath(file string) (string, error)
	Run(ctx context.Context, invocation commandInvocation) (string, error)
}

type osCommandRunner struct{}

func (osCommandRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (osCommandRunner) Run(ctx context.Context, invocation commandInvocation) (string, error) {
	cmd := exec.CommandContext(ctx, invocation.Name, invocation.Args...)
	cmd.Dir = invocation.Dir

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err := cmd.Run()
	return output.String(), err
}
