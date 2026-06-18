package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/segmentstream/segmentstream-cli/internal/projectsource"
	"github.com/spf13/cobra"
)

func newSourceCommand(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "source",
		Short: "Manage SegmentStream sources",
	}

	cmd.AddCommand(newSourceInitCommand(out))

	return cmd
}

func newSourceInitCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "init <name>",
		Short: "Create a local source package template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("find current directory: %w", err)
			}

			source, err := projectsource.Init(projectRoot, args[0])
			if err != nil {
				return err
			}

			relativePath := filepath.ToSlash(filepath.Join(projectsource.SourcesDirName, source.Name))
			fmt.Fprintf(out, "Created source template %q at %s\n", source.Name, relativePath)
			fmt.Fprintln(out)
			fmt.Fprintln(out, "This is a scaffold, not a completed source implementation.")
			fmt.Fprintln(out, "Agent task: inspect the raw source schema, then implement the dbt models for this source:")
			fmt.Fprintf(out, "- %s\n", filepath.ToSlash(filepath.Join(relativePath, "models", "staging", "stg_events_"+source.Name+".sql")))
			fmt.Fprintf(out, "- %s\n", filepath.ToSlash(filepath.Join(relativePath, "models", "exports", "events_"+source.Name+".sql")))
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Add this source to segmentstream.yml:")
			fmt.Fprintln(out, "sources:")
			fmt.Fprintf(out, "  - name: %s\n", source.Name)
			fmt.Fprintf(out, "    path: ./%s\n", relativePath)
			return nil
		},
	}
}
