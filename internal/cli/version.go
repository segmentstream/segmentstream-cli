package cli

import (
	"fmt"
	"io"

	"github.com/segmentstream/segmentstream-cli/internal/version"
	"github.com/spf13/cobra"
)

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
