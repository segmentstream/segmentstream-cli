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
			fmt.Fprintf(out, "Created source %q at %s\n", source.Name, relativePath)
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Add this source to segmentstream.yml:")
			fmt.Fprintln(out, "sources:")
			fmt.Fprintf(out, "  - name: %s\n", source.Name)
			fmt.Fprintf(out, "    path: ./%s\n", relativePath)
			return nil
		},
	}
}
