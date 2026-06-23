package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/segmentstream/segmentstream-cli/cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/cli/internal/initflow"
	"github.com/segmentstream/segmentstream-cli/cli/internal/project"
	"github.com/spf13/cobra"
)

type initCommandOptions struct {
	Warehouse string
}

type initResponseData struct {
	SelectedWarehouse string             `json:"selected_warehouse,omitempty"`
	Envelope          cliresult.Envelope `json:"envelope"`
}

func newInitCommand(out io.Writer, commandContext structuredCommandContext, cliOptions cliOptions) *cobra.Command {
	options := initCommandOptions{}
	cmd := newStructuredCommand(out, nil, commandContext, structuredCommandSpec{
		Use:   "init",
		Short: "Inspect or initialize SegmentStream project state",
		Long: "Inspect SegmentStream project setup and report the next action.\n\n" +
			"By default, output is human-readable. With --json, init emits a stable\n" +
			"state-machine envelope for agents and automation. Running init --json is\n" +
			"read-only. Running init --warehouse bigquery creates or updates\n" +
			"segmentstream.yml with the selected warehouse type and credential name.",
		Args:    cobra.NoArgs,
		Command: "init",
	}, func(cmdContext context.Context, args []string) (cliresult.Response, error) {
		projectRoot, err := os.Getwd()
		if err != nil {
			return cliresult.Response{}, fmt.Errorf("find current directory: %w", err)
		}

		result, err := (initflow.Service{
			ProjectRoot: projectRoot,
			Credentials: cliOptions.Credentials,
		}).Evaluate(cmdContext, initflow.Options{SelectWarehouse: options.Warehouse})
		if err != nil {
			return cliresult.Response{}, err
		}
		return cliresult.OK("init", initResponseData{
			SelectedWarehouse: options.Warehouse,
			Envelope:          result.Envelope,
		}), nil
	})
	cmd.Flags().StringVar(&options.Warehouse, "warehouse", "", "Select and persist the warehouse type; currently only bigquery is available")
	return cmd
}

func (data initResponseData) HumanDocument() cliresult.Document {
	return textDocument(func(out io.Writer) {
		writeInitResult(out, data.SelectedWarehouse, initflow.Result{Envelope: data.Envelope})
	})
}

func writeInitResult(out io.Writer, selectedWarehouse string, result initflow.Result) {
	if selectedWarehouse != "" {
		fmt.Fprintf(out, "Selected warehouse %q in %s\n", selectedWarehouse, project.ConfigFileName)
	}
	if result.Envelope.Ready {
		fmt.Fprintln(out, "SegmentStream project is ready.")
		if result.Envelope.NextAction.Command != "" {
			fmt.Fprintf(out, "Next: %s\n", result.Envelope.NextAction.Command)
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
		for _, accept := range result.Envelope.NextAction.Accepts {
			fmt.Fprintf(out, "Option: %s\n", accept.Label)
			if accept.Command != "" {
				fmt.Fprintf(out, "  Command: %s\n", accept.Command)
			}
			for _, input := range accept.Inputs {
				fmt.Fprintf(out, "  Input: %s (%s", input.Label, input.Type)
				if input.Flag != "" {
					fmt.Fprintf(out, ", %s", input.Flag)
				}
				if input.Required {
					fmt.Fprint(out, ", required")
				}
				fmt.Fprintln(out, ")")
			}
		}
		if result.Envelope.NextAction.Verify != "" {
			fmt.Fprintf(out, "Verify: %s\n", result.Envelope.NextAction.Verify)
		}
	}
}
