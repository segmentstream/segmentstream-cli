package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/segmentstream/segmentstream-cli/internal/update"
	"github.com/segmentstream/segmentstream-cli/internal/version"
	"github.com/spf13/cobra"
)

func Execute() error {
	return NewRootCommand(os.Stdout, os.Stderr).Execute()
}

func NewRootCommand(out, errOut io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:           "segmentstream",
		Short:         "CLI for SegmentStream marketing analytics",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newVersionCommand(out))
	root.AddCommand(newUpdateCommand(out, errOut))

	return root
}

func newVersionCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			info := version.Current()
			fmt.Fprintf(out, "segmentstream %s\n", info.Version)
			fmt.Fprintf(out, "commit: %s\n", info.Commit)
			fmt.Fprintf(out, "date: %s\n", info.Date)
			fmt.Fprintf(out, "os/arch: %s/%s\n", info.OS, info.Arch)
			return nil
		},
	}
}

func newUpdateCommand(out, errOut io.Writer) *cobra.Command {
	var checkOnly bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update segmentstream",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			updater := update.NewUpdater(version.Current(), out, errOut)
			return updater.Run(cmd.Context(), update.Options{CheckOnly: checkOnly})
		},
	}

	cmd.Flags().BoolVar(&checkOnly, "check", false, "check for updates without installing")

	return cmd
}
