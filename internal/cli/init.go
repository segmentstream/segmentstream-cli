package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/segmentstream/segmentstream-cli/internal/project"
	"github.com/segmentstream/segmentstream-cli/internal/projectruntime"
	"github.com/spf13/cobra"
)

func newInitCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a SegmentStream project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("find current directory: %w", err)
			}

			result, err := project.Scaffold(projectRoot)
			if err != nil {
				return err
			}
			writeScaffoldResult(out, result)

			return prepareProject(projectRoot, out)
		},
	}
}

func writeScaffoldResult(out io.Writer, result project.ScaffoldResult) {
	if result.ConfigCreated {
		fmt.Fprintf(out, "Created %s\n", project.ConfigFileName)
	} else if result.ConfigExisted {
		fmt.Fprintf(out, "Using existing %s\n", project.ConfigFileName)
	}
	if result.ReadmeCreated {
		fmt.Fprintf(out, "Created %s\n", project.ProjectReadmeFileName)
	}
	if result.AgentGuideCreated {
		fmt.Fprintf(out, "Created %s\n", project.AgentGuideFileName)
	}
}

func prepareProject(projectRoot string, out io.Writer) error {
	config, err := project.LoadConfig(projectRoot)
	if err != nil {
		return err
	}
	if err := projectruntime.Prepare(projectRoot, config); err != nil {
		return err
	}
	fmt.Fprintln(out, "Prepared SegmentStream project")
	return nil
}
