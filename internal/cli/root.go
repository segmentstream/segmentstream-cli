package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/segmentstream/segmentstream-cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/internal/credentials"
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

	cmd := NewRootCommand(out, errOut)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		if err.Error() != "" {
			fmt.Fprintln(errOut, err)
		}
		return cliresult.ExitCode(err)
	}
	return 0
}

func NewRootCommand(out, errOut io.Writer) *cobra.Command {
	return newRootCommand(out, errOut, cliOptions{})
}

type cliOptions struct {
	CommandRunner     commandRunner
	Credentials       credentials.Store
	WarehouseRegistry warehouse.Registry
	WarehouseOAuth    warehouseOAuthLogin
}

type warehouseOAuthLogin func(context.Context, io.Writer) (credentials.GoogleOAuthCredential, error)

func newRootCommand(out, errOut io.Writer, options cliOptions) *cobra.Command {
	runner := options.CommandRunner
	if runner == nil {
		runner = osCommandRunner{}
	}
	registry := options.WarehouseRegistry
	if registry.IsZero() {
		registry = warehouse.NewRegistry(bigquery.NewConnector())
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
	root.AddCommand(newInitCommand(out, options))
	root.AddCommand(newRunCommand(out, runner))
	root.AddCommand(newSourceCommand(out))
	root.AddCommand(newWarehouseCommand(out, errOut, options.Credentials, registry, options.WarehouseOAuth))

	return root
}
