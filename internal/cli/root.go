package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func Execute() error {
	return NewRootCommand(os.Stdout, os.Stderr).Execute()
}

func Main(args []string, out, errOut io.Writer) int {
	if errOut == nil {
		errOut = io.Discard
	}

	cmd := NewRootCommand(out, errOut)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	return 0
}

func NewRootCommand(out, errOut io.Writer) *cobra.Command {
	return newRootCommand(out, errOut, cliOptions{})
}

type cliOptions struct {
	CommandRunner            commandRunner
	NewBigQueryAuthenticator func(io.Writer, io.Writer) bigQueryAuthenticator
}

func newRootCommand(out, errOut io.Writer, options cliOptions) *cobra.Command {
	runner := options.CommandRunner
	if runner == nil {
		runner = osCommandRunner{}
	}

	root := &cobra.Command{
		Use:           "segmentstream",
		Short:         "CLI for SegmentStream marketing analytics",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	if out != nil {
		root.SetOut(out)
	}
	if errOut != nil {
		root.SetErr(errOut)
	}

	root.AddCommand(newVersionCommand(out))
	root.AddCommand(newUpdateCommand(out, errOut))
	root.AddCommand(newInitCommand(out))
	root.AddCommand(newRunCommand(out, runner))
	root.AddCommand(newSourceCommand(out))
	root.AddCommand(newAuthCommand(out, errOut, options))

	return root
}
