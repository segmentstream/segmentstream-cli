package cli

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/segmentstream/segmentstream-cli/internal/cliresult"
	"github.com/spf13/cobra"
)

type structuredHandler func(context.Context, []string) (cliresult.Response, error)

type outputOptions struct {
	JSON bool
}

type structuredCommandContext struct {
	Output *outputOptions
}

type structuredCommandSpec struct {
	Use     string
	Short   string
	Long    string
	Command string
	Args    cobra.PositionalArgs
}

func newStructuredCommand(out, errOut io.Writer, commandContext structuredCommandContext, spec structuredCommandSpec, handler structuredHandler) *cobra.Command {
	if commandContext.Output == nil {
		commandContext.Output = &outputOptions{}
	}
	cmd := &cobra.Command{
		Use:   spec.Use,
		Short: spec.Short,
		Long:  spec.Long,
		Args:  spec.Args,
		RunE: func(cmd *cobra.Command, args []string) error {
			var handlerErr error
			response, err := handler(cmd.Context(), args)
			if err != nil {
				handlerErr = err
				response = cliresult.Error(spec.Command, err)
			}
			response = normalizeResponse(spec.Command, response)

			if !commandContext.Output.JSON && response.Status == cliresult.StatusError {
				return cliresult.WithExitCode(response.ExitCode, responseError(response, handlerErr))
			}

			if commandContext.Output.JSON {
				if err := cliresult.WriteJSON(commandOut(cmd, out), response); err != nil {
					return err
				}
			} else {
				writer := commandOut(cmd, out)
				if response.Status == cliresult.StatusError {
					writer = commandErr(cmd, errOut)
				}
				if err := cliresult.WriteHuman(writer, response); err != nil {
					return err
				}
			}
			if response.ExitCode != cliresult.ExitReady {
				return cliresult.WithExitCode(response.ExitCode, nil)
			}
			return nil
		},
	}
	return cmd
}

func commandOut(cmd *cobra.Command, out io.Writer) io.Writer {
	if out != nil {
		return out
	}
	return cmd.OutOrStdout()
}

func commandErr(cmd *cobra.Command, errOut io.Writer) io.Writer {
	if errOut != nil {
		return errOut
	}
	return cmd.ErrOrStderr()
}

func responseError(response cliresult.Response, fallback error) error {
	if fallback != nil {
		return fallback
	}
	if len(response.Diagnostics) > 0 && response.Diagnostics[0].Message != "" {
		return errors.New(response.Diagnostics[0].Message)
	}
	return errors.New(string(response.Status))
}

func normalizeResponse(command string, response cliresult.Response) cliresult.Response {
	if response.SchemaVersion == "" {
		response.SchemaVersion = cliresult.SchemaVersion
	}
	if response.Command == "" {
		response.Command = command
	}
	if response.Status == "" {
		response.Status = cliresult.StatusOK
	}
	return response
}

func textDocument(write func(io.Writer)) cliresult.Document {
	var output strings.Builder
	write(&output)
	text := strings.TrimRight(output.String(), "\n")
	if text == "" {
		return cliresult.Document{}
	}
	return cliresult.Document{
		Blocks: []cliresult.Block{
			{Kind: cliresult.BlockCode, Text: text},
		},
	}
}
