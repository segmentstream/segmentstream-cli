package cli

import (
	"io"

	"github.com/segmentstream/segmentstream-cli/internal/update"
	"github.com/segmentstream/segmentstream-cli/internal/version"
	"github.com/spf13/cobra"
)

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
