package cli

import (
	"context"
	"io"

	"github.com/segmentstream/segmentstream-cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/internal/version"
	"github.com/spf13/cobra"
)

type versionResult struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

func newVersionCommand(out io.Writer, commandContext structuredCommandContext) *cobra.Command {
	return newStructuredCommand(out, nil, commandContext, structuredCommandSpec{
		Use:     "version",
		Short:   "Print version information",
		Command: "version",
		Args:    cobra.NoArgs,
	}, runVersionCommand)
}

func runVersionCommand(ctx context.Context, args []string) (cliresult.Response, error) {
	_ = ctx
	_ = args
	info := version.Current()
	return cliresult.OK("version", versionResult{
		Version: info.Version,
		Commit:  info.Commit,
		Date:    info.Date,
		OS:      info.OS,
		Arch:    info.Arch,
	}), nil
}

func (result versionResult) HumanDocument() cliresult.Document {
	return cliresult.Document{
		Summary: "segmentstream " + result.Version,
		Blocks: []cliresult.Block{
			{
				Kind: cliresult.BlockFields,
				Fields: []cliresult.Field{
					{Name: "commit", Value: result.Commit},
					{Name: "date", Value: result.Date},
					{Name: "os/arch", Value: result.OS + "/" + result.Arch},
				},
			},
		},
	}
}
