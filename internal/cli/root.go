package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/segmentstream/segmentstream-cli/internal/auth"
	"github.com/segmentstream/segmentstream-cli/internal/project"
	"github.com/segmentstream/segmentstream-cli/internal/projectruntime"
	"github.com/segmentstream/segmentstream-cli/internal/update"
	"github.com/segmentstream/segmentstream-cli/internal/version"
	"github.com/spf13/cobra"
)

func Execute() error {
	return NewRootCommand(os.Stdout, os.Stderr).Execute()
}

func NewRootCommand(out, errOut io.Writer) *cobra.Command {
	return newRootCommand(out, errOut, cliOptions{})
}

type bigQueryAuthenticator interface {
	AuthenticateBigQuery(context.Context) (string, error)
}

type cliOptions struct {
	CommandRunner            commandRunner
	NewBigQueryAuthenticator func(io.Writer, io.Writer) bigQueryAuthenticator
}

func newRootCommand(out, errOut io.Writer, options cliOptions) *cobra.Command {
	runner := options.CommandRunner
	if runner == nil {
		runner = osCommandRunner{}
	}

	root := &cobra.Command{
		Use:           "segmentstream",
		Short:         "CLI for SegmentStream marketing analytics",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	if out != nil {
		root.SetOut(out)
	}
	if errOut != nil {
		root.SetErr(errOut)
	}

	root.AddCommand(newVersionCommand(out))
	root.AddCommand(newUpdateCommand(out, errOut))
	root.AddCommand(newInitCommand(out))
	root.AddCommand(newRunCommand(out, runner))
	root.AddCommand(newSourceCommand(out))
	root.AddCommand(newAuthCommand(out, errOut, options))

	return root
}

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

			configPath := filepath.Join(projectRoot, project.ConfigFileName)
			if _, err := os.Stat(configPath); err != nil {
				if os.IsNotExist(err) {
					if err := os.WriteFile(configPath, []byte(project.DefaultConfigYAML()), 0o644); err != nil {
						return fmt.Errorf("write %s: %w", project.ConfigFileName, err)
					}
					fmt.Fprintf(out, "Created %s\n", project.ConfigFileName)
				} else {
					return fmt.Errorf("check %s: %w", project.ConfigFileName, err)
				}
			} else {
				fmt.Fprintf(out, "Using existing %s\n", project.ConfigFileName)
			}

			if err := project.EnsureRuntimeGitignored(projectRoot); err != nil {
				return err
			}
			if created, err := project.EnsureProjectReadme(projectRoot); err != nil {
				return err
			} else if created {
				fmt.Fprintf(out, "Created %s\n", project.ProjectReadmeFileName)
			}
			if created, err := project.EnsureAgentGuide(projectRoot); err != nil {
				return err
			} else if created {
				fmt.Fprintf(out, "Created %s\n", project.AgentGuideFileName)
			}
			return prepareProject(projectRoot, out)
		},
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

func newAuthCommand(out io.Writer, errOut io.Writer, options cliOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate data source credentials",
	}

	cmd.AddCommand(newAuthBigQueryCommand(out, errOut, options))

	return cmd
}

func newAuthBigQueryCommand(out io.Writer, errOut io.Writer, options cliOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "bigquery",
		Short: "Authenticate BigQuery credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
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
