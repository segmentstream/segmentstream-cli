package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/segmentstream/segmentstream-cli/internal/auth"
	"github.com/spf13/cobra"
)

type bigQueryAuthenticator interface {
	AuthenticateBigQuery(context.Context) (string, error)
}

func newAuthCommand(out io.Writer, errOut io.Writer, options cliOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate data source credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newAuthAddCommand(out, errOut, options))

	return cmd
}

func newAuthAddCommand(out io.Writer, errOut io.Writer, options cliOptions) *cobra.Command {
	return &cobra.Command{
		Use:       "add <type>",
		Short:     "Add data source credentials",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bigquery"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "bigquery" {
				return fmt.Errorf("unsupported auth type %q; only bigquery is supported", args[0])
			}

			factory := options.NewBigQueryAuthenticator
			if factory == nil {
				factory = func(out, errOut io.Writer) bigQueryAuthenticator {
					return auth.NewGCloudAuthenticator(out, errOut)
				}
			}

			_, err := factory(out, errOut).AuthenticateBigQuery(cmd.Context())
			return err
		},
	}
}
