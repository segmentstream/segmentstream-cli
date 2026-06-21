package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/segmentstream/segmentstream-cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/internal/projectsource"
	"github.com/spf13/cobra"
)

type sourceContractsOptions struct {
	Type string
}

type sourceCreateOptions struct {
	Type string
}

type sourceContractAction struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

type sourceContractSummary struct {
	Contract    projectsource.ContractIdentity `json:"contract"`
	Description string                         `json:"description"`
	Default     bool                           `json:"default,omitempty"`
	Status      string                         `json:"status"`
	Model       string                         `json:"model"`
	Actions     []sourceContractAction         `json:"actions"`
}

type sourceContractsListResult struct {
	SchemaVersion string                  `json:"schema_version"`
	Contracts     []sourceContractSummary `json:"contracts"`
}

type sourceContractDetailResult struct {
	SchemaVersion string                         `json:"schema_version"`
	Contract      projectsource.ContractIdentity `json:"contract"`
	Description   string                         `json:"description"`
	Default       bool                           `json:"default,omitempty"`
	Status        string                         `json:"status"`
	Model         projectsource.ContractModel    `json:"model"`
	Columns       []projectsource.ContractColumn `json:"columns"`
	Actions       []sourceContractAction         `json:"actions"`
}

type sourceCreateAction struct {
	Type    string `json:"type"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message,omitempty"`
	Snippet string `json:"snippet,omitempty"`
}

type sourceCreateResult struct {
	SchemaVersion string                         `json:"schema_version"`
	Source        sourceCreateResultSource       `json:"source"`
	Directory     string                         `json:"directory"`
	CreatedFiles  []string                       `json:"created_files"`
	Contract      projectsource.ContractIdentity `json:"contract"`
	Actions       []sourceCreateAction           `json:"actions"`
}

type sourceCreateResultSource struct {
	Name        string `json:"name"`
	PackageName string `json:"package_name"`
}

func newSourceCommand(out io.Writer, commandContext structuredCommandContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "source",
		Short: "Manage SegmentStream sources",
	}

	cmd.AddCommand(newSourceContractsCommand(out, commandContext))
	cmd.AddCommand(newSourceCreateCommand(out, commandContext))
	cmd.AddCommand(newSourceInitCommand(out, commandContext))

	return cmd
}

func newSourceContractsCommand(out io.Writer, commandContext structuredCommandContext) *cobra.Command {
	options := sourceContractsOptions{}
	cmd := newStructuredCommand(out, nil, commandContext, structuredCommandSpec{
		Use:     "contracts",
		Short:   "List supported custom source contracts",
		Args:    cobra.NoArgs,
		Command: "source.contracts",
	}, func(ctx context.Context, args []string) (cliresult.Response, error) {
		_ = ctx
		if options.Type != "" {
			contract, err := projectsource.ContractByType(options.Type)
			if err != nil {
				return cliresult.Response{}, err
			}
			return cliresult.OK("source.contracts", sourceContractDetail(contract)), nil
		}

		contracts, err := projectsource.Contracts()
		if err != nil {
			return cliresult.Response{}, err
		}
		return cliresult.OK("source.contracts", sourceContractsList(contracts)), nil
	})
	cmd.Flags().StringVar(&options.Type, "type", "", "Show the full schema for a source contract type")
	return cmd
}

func newSourceCreateCommand(out io.Writer, commandContext structuredCommandContext) *cobra.Command {
	options := sourceCreateOptions{}
	cmd := newStructuredCommand(out, nil, commandContext, structuredCommandSpec{
		Use:     "create <name> --type <contract>",
		Short:   "Create a local source package from a contract",
		Args:    cobra.ExactArgs(1),
		Command: "source.create",
	}, func(ctx context.Context, args []string) (cliresult.Response, error) {
		_ = ctx
		if options.Type == "" {
			return cliresult.Response{}, fmt.Errorf("--type is required; run segmentstream source contracts to list supported contracts")
		}
		projectRoot, err := os.Getwd()
		if err != nil {
			return cliresult.Response{}, fmt.Errorf("find current directory: %w", err)
		}

		source, err := projectsource.Create(projectRoot, args[0], options.Type)
		if err != nil {
			return cliresult.Response{}, err
		}
		return cliresult.OK("source.create", sourceCreateJSON(source)), nil
	})
	cmd.Flags().StringVar(&options.Type, "type", "", "Source contract type")
	return cmd
}

func newSourceInitCommand(out io.Writer, commandContext structuredCommandContext) *cobra.Command {
	return newStructuredCommand(out, nil, commandContext, structuredCommandSpec{
		Use:     "init <name>",
		Short:   "Create a local source package from the default contract",
		Args:    cobra.ExactArgs(1),
		Command: "source.init",
	}, func(ctx context.Context, args []string) (cliresult.Response, error) {
		_ = ctx
		projectRoot, err := os.Getwd()
		if err != nil {
			return cliresult.Response{}, fmt.Errorf("find current directory: %w", err)
		}

		source, err := projectsource.Init(projectRoot, args[0])
		if err != nil {
			return cliresult.Response{}, err
		}

		return cliresult.OK("source.init", sourceCreateJSON(source)), nil
	})
}

func sourceContractsList(contracts []projectsource.Contract) sourceContractsListResult {
	summaries := make([]sourceContractSummary, 0, len(contracts))
	for _, contract := range contracts {
		summaries = append(summaries, sourceContractSummary{
			Contract:    contract.Identity(),
			Description: contract.Description,
			Default:     contract.Default,
			Status:      contract.Status,
			Model:       contract.Model.Name,
			Actions:     sourceContractActions(contract),
		})
	}
	return sourceContractsListResult{
		SchemaVersion: cliresult.SchemaVersion,
		Contracts:     summaries,
	}
}

func sourceContractDetail(contract projectsource.Contract) sourceContractDetailResult {
	return sourceContractDetailResult{
		SchemaVersion: cliresult.SchemaVersion,
		Contract:      contract.Identity(),
		Description:   contract.Description,
		Default:       contract.Default,
		Status:        contract.Status,
		Model:         contract.Model,
		Columns:       contract.Columns,
		Actions:       sourceContractActions(contract),
	}
}

func sourceContractActions(contract projectsource.Contract) []sourceContractAction {
	return []sourceContractAction{
		{
			Type:    "inspect_schema",
			Command: fmt.Sprintf("segmentstream source contracts --type %s --json", contract.Type),
		},
		{
			Type:    "create_source",
			Command: fmt.Sprintf("segmentstream source create <name> --type %s --json", contract.Type),
		},
	}
}

func sourceCreateJSON(source projectsource.Source) sourceCreateResult {
	relativePath := sourceRelativePath(source)
	return sourceCreateResult{
		SchemaVersion: cliresult.SchemaVersion,
		Source: sourceCreateResultSource{
			Name:        source.Name,
			PackageName: source.PackageName,
		},
		Directory:    relativePath,
		CreatedFiles: append([]string(nil), source.CreatedFiles...),
		Contract:     source.Contract,
		Actions: []sourceCreateAction{
			{
				Type: "implement",
				Path: sourceImplementationPath(source),
			},
			{
				Type:    "tell_user",
				Message: "Add this source to segmentstream.yml.",
				Snippet: sourceConfigSnippet(source),
			},
		},
	}
}

func (result sourceContractsListResult) HumanDocument() cliresult.Document {
	return textDocument(func(out io.Writer) {
		fmt.Fprintln(out, "Supported source contracts:")
		for _, contract := range result.Contracts {
			marker := ""
			if contract.Default {
				marker = ", default"
			}
			fmt.Fprintf(out, "- %s (schema_version: %d, %s%s)\n", contract.Contract.Type, contract.Contract.SchemaVersion, contract.Status, marker)
			fmt.Fprintf(out, "  %s\n", contract.Description)
			for _, action := range contract.Actions {
				switch action.Type {
				case "inspect_schema":
					fmt.Fprintf(out, "  Inspect: %s\n", strings.TrimSuffix(action.Command, " --json"))
				case "create_source":
					fmt.Fprintf(out, "  Create: %s\n", strings.TrimSuffix(action.Command, " --json"))
				}
			}
		}
	})
}

func (result sourceContractDetailResult) HumanDocument() cliresult.Document {
	return textDocument(func(out io.Writer) {
		fmt.Fprintf(out, "Source contract: %s\n", result.Contract.Type)
		fmt.Fprintf(out, "Schema version: %d\n", result.Contract.SchemaVersion)
		fmt.Fprintf(out, "Status: %s\n", result.Status)
		if result.Default {
			fmt.Fprintln(out, "Default: yes")
		} else {
			fmt.Fprintln(out, "Default: no")
		}
		fmt.Fprintf(out, "Model: %s\n", result.Model.Name)
		fmt.Fprintf(out, "Partition: %s\n", result.Model.Partition)
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Columns:")
		for _, column := range result.Columns {
			required := "optional"
			if column.Required {
				required = "required"
			}
			fmt.Fprintf(out, "- %s %s %s: %s\n", column.Name, column.Type, required, column.Description)
		}
	})
}

func (result sourceCreateResult) HumanDocument() cliresult.Document {
	return textDocument(func(out io.Writer) {
		fmt.Fprintf(out, "Created source %q at %s\n", result.Source.Name, result.Directory)
		fmt.Fprintf(out, "Contract: %s (schema_version: %d)\n", result.Contract.Type, result.Contract.SchemaVersion)
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Implement:")
		for _, action := range result.Actions {
			if action.Type == "implement" && action.Path != "" {
				fmt.Fprintf(out, "- %s\n", action.Path)
			}
		}
		fmt.Fprintln(out)
		for _, action := range result.Actions {
			if action.Type == "tell_user" && action.Snippet != "" {
				fmt.Fprintln(out, "Add this source to segmentstream.yml:")
				fmt.Fprint(out, action.Snippet)
			}
		}
	})
}

func sourceRelativePath(source projectsource.Source) string {
	return filepath.ToSlash(filepath.Join(projectsource.SourcesDirName, source.Name))
}

func sourceImplementationPath(source projectsource.Source) string {
	return filepath.ToSlash(filepath.Join(sourceRelativePath(source), "models", source.ModelName+".sql"))
}

func sourceConfigSnippet(source projectsource.Source) string {
	relativePath := sourceRelativePath(source)
	return fmt.Sprintf("sources:\n  - name: %s\n    path: ./%s\n", source.Name, relativePath)
}
