package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/segmentstream/segmentstream-cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/internal/credentials"
	"github.com/segmentstream/segmentstream-cli/internal/googleoauth"
	"github.com/segmentstream/segmentstream-cli/internal/warehouse"
	"github.com/segmentstream/segmentstream-cli/internal/warehouse/bigquery"
	"github.com/spf13/cobra"
)

func Execute() error {
	return NewRootCommand(os.Stdout, os.Stderr).Execute()
}

func Main(args []string, out, errOut io.Writer) int {
	if errOut == nil {
		errOut = io.Discard
	}

	output := &outputOptions{}
	output.JSON = argsRequestJSON(args)
	cmd := newRootCommand(out, errOut, cliOptions{Output: output})
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		if err.Error() != "" {
			if output.JSON {
				_ = cliresult.WriteJSON(out, cliresult.Error("segmentstream", err))
			} else {
				fmt.Fprintln(errOut, err)
			}
		}
		return cliresult.ExitCode(err)
	}
	return 0
}

func argsRequestJSON(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "--json", "--json=true":
			return true
		case "--json=false":
			return false
		}
	}
	return false
}

func NewRootCommand(out, errOut io.Writer) *cobra.Command {
	return newRootCommand(out, errOut, cliOptions{})
}

type cliOptions struct {
	CommandRunner     commandRunner
	Credentials       credentials.Store
	WarehouseRegistry warehouse.Registry
	WarehouseOAuth    warehouseOAuthLogin
	Output            *outputOptions
}

type warehouseOAuthLogin func(context.Context, io.Writer, googleoauth.LoginOptions) (credentials.GoogleOAuthCredential, error)

func newRootCommand(out, errOut io.Writer, options cliOptions) *cobra.Command {
	runner := options.CommandRunner
	if runner == nil {
		runner = osCommandRunner{}
	}
	registry := options.WarehouseRegistry
	if registry.IsZero() {
		registry = warehouse.NewRegistry(bigquery.NewConnector())
	}
	output := options.Output
	if output == nil {
		output = &outputOptions{}
	}
	commandContext := structuredCommandContext{Output: output}

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
	root.PersistentFlags().BoolVar(&output.JSON, "json", output.JSON, "Emit JSON output for agents and automation")

	root.AddCommand(newVersionCommand(out, commandContext))
	root.AddCommand(newUpdateCommand(out, errOut, commandContext))
	root.AddCommand(newInitCommand(out, commandContext, options))
	root.AddCommand(newRunCommand(out, errOut, commandContext, runner))
	root.AddCommand(newSourceCommand(out, errOut, commandContext, runner))
	root.AddCommand(newWarehouseCommand(out, errOut, commandContext, options.Credentials, registry, options.WarehouseOAuth))

	return root
}
