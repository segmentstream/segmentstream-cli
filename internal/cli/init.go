package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/segmentstream/segmentstream-cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/internal/initflow"
	"github.com/segmentstream/segmentstream-cli/internal/project"
	"github.com/spf13/cobra"
)

type initCommandOptions struct {
	Warehouse string
	JSON      bool
}

func newInitCommand(out io.Writer, cliOptions cliOptions) *cobra.Command {
	options := initCommandOptions{}
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Inspect or initialize SegmentStream project state",
		Long: "Inspect SegmentStream project setup and report the next action.\n\n" +
			"By default, output is human-readable. With --json, init emits a stable\n" +
			"state-machine envelope for agents and automation. Running init --json is\n" +
			"read-only. Running init --warehouse bigquery creates or updates\n" +
			"segmentstream.yml with the selected warehouse type and credential name.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("find current directory: %w", err)
			}

			result, err := (initflow.Service{
				ProjectRoot: projectRoot,
				Credentials: cliOptions.Credentials,
			}).Evaluate(cmd.Context(), initflow.Options{SelectWarehouse: options.Warehouse})
			if err != nil {
				return err
			}
			if options.JSON {
				if err := cliresult.WriteJSON(out, result.Envelope); err != nil {
					return err
				}
			} else {
				writeInitResult(out, options.Warehouse, result)
			}
			if result.ExitCode != cliresult.ExitReady {
				return cliresult.WithExitCode(result.ExitCode, nil)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&options.Warehouse, "warehouse", "", "Select and persist the warehouse type; currently only bigquery is available")
	cmd.Flags().BoolVar(&options.JSON, "json", false, "Emit a stable JSON state envelope for agents and automation")
	return cmd
}

func writeInitResult(out io.Writer, selectedWarehouse string, result initflow.Result) {
	if selectedWarehouse != "" {
		fmt.Fprintf(out, "Selected warehouse %q in %s\n", selectedWarehouse, project.ConfigFileName)
	}
	if result.Envelope.Ready {
		fmt.Fprintln(out, "SegmentStream project is ready.")
		if result.Envelope.NextAction.Suggested != "" {
			fmt.Fprintf(out, "Next: %s\n", result.Envelope.NextAction.Suggested)
		}
		return
	}
	fmt.Fprintln(out, "SegmentStream project is not ready yet.")
	if len(result.Envelope.Diagnostics) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Diagnostics:")
		for _, diagnostic := range result.Envelope.Diagnostics {
			if diagnostic.Field != "" {
				fmt.Fprintf(out, "- %s: %s\n", diagnostic.Field, diagnostic.Message)
				continue
			}
			fmt.Fprintf(out, "- %s\n", diagnostic.Message)
		}
	}
	if result.Envelope.NextAction.Type != "" {
		fmt.Fprintln(out)
		fmt.Fprintf(out, "Next action: %s\n", result.Envelope.NextAction.Type)
		if result.Envelope.NextAction.Command != "" {
			fmt.Fprintf(out, "Run: %s\n", result.Envelope.NextAction.Command)
		}
		if result.Envelope.NextAction.Reason != "" {
			fmt.Fprintf(out, "Reason: %s\n", result.Envelope.NextAction.Reason)
		}
		for _, hint := range result.Envelope.NextAction.Hints {
			fmt.Fprintf(out, "Hint: %s\n", hint.Message)
			for _, command := range hint.Commands {
				fmt.Fprintf(out, "  %s\n", command)
			}
		}
	}
}
