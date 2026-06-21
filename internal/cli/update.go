package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/segmentstream/segmentstream-cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/internal/update"
	"github.com/segmentstream/segmentstream-cli/internal/version"
	"github.com/spf13/cobra"
)

type updateData update.Result

func newUpdateCommand(out, errOut io.Writer, commandContext structuredCommandContext) *cobra.Command {
	var checkOnly bool

	cmd := newStructuredCommand(out, errOut, commandContext, structuredCommandSpec{
		Use:     "update",
		Short:   "Update segmentstream",
		Args:    cobra.NoArgs,
		Command: "update",
	}, func(ctx context.Context, args []string) (cliresult.Response, error) {
		updater := update.NewUpdater(version.Current(), io.Discard, errOut)
		result, err := updater.RunWithResult(ctx, update.Options{CheckOnly: checkOnly})
		if err != nil {
			return cliresult.Response{}, err
		}
		return cliresult.OK("update", updateData(result)), nil
	})

	cmd.Flags().BoolVar(&checkOnly, "check", false, "check for updates without installing")

	return cmd
}

func (data updateData) HumanDocument() cliresult.Document {
	return textDocument(func(out io.Writer) {
		fmt.Fprintf(out, "Current version: %s\n", data.CurrentVersion)
		fmt.Fprintf(out, "Latest version:  %s\n", data.LatestVersion)
		switch data.Status {
		case "up_to_date":
			fmt.Fprintln(out, "segmentstream is already up to date.")
		case "update_available":
			fmt.Fprintln(out, "An update is available.")
		case "updated":
			fmt.Fprintf(out, "Updated segmentstream to %s\n", data.LatestVersion)
		default:
			fmt.Fprintf(out, "Update status: %s\n", data.Status)
		}
	})
}
