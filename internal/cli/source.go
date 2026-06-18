package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/segmentstream/segmentstream-cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/internal/projectsource"
	"github.com/spf13/cobra"
)

type sourceContractsOptions struct {
	Type string
	JSON bool
}

type sourceCreateOptions struct {
	Type string
	JSON bool
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

func newSourceCommand(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "source",
		Short: "Manage SegmentStream sources",
	}

	cmd.AddCommand(newSourceContractsCommand(out))
	cmd.AddCommand(newSourceCreateCommand(out))
	cmd.AddCommand(newSourceInitCommand(out))

	return cmd
}

func newSourceContractsCommand(out io.Writer) *cobra.Command {
	options := sourceContractsOptions{}
	cmd := &cobra.Command{
		Use:   "contracts",
		Short: "List supported custom source contracts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if options.Type != "" {
				contract, err := projectsource.ContractByType(options.Type)
				if err != nil {
					return err
				}
				if options.JSON {
					return cliresult.WriteJSON(out, sourceContractDetail(contract))
				}
				writeSourceContractDetail(out, contract)
				return nil
			}

			contracts, err := projectsource.Contracts()
			if err != nil {
				return err
			}
			if options.JSON {
				return cliresult.WriteJSON(out, sourceContractsList(contracts))
			}
			writeSourceContractsList(out, contracts)
			return nil
		},
	}
	cmd.Flags().StringVar(&options.Type, "type", "", "Show the full schema for a source contract type")
	cmd.Flags().BoolVar(&options.JSON, "json", false, "Emit JSON output for agents and automation")
	return cmd
}

func newSourceCreateCommand(out io.Writer) *cobra.Command {
	options := sourceCreateOptions{}
	cmd := &cobra.Command{
		Use:   "create <name> --type <contract>",
		Short: "Create a local source package from a contract",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if options.Type == "" {
				return fmt.Errorf("--type is required; run segmentstream source contracts to list supported contracts")
			}
			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("find current directory: %w", err)
			}

			source, err := projectsource.Create(projectRoot, args[0], options.Type)
			if err != nil {
				return err
			}
			if options.JSON {
				return cliresult.WriteJSON(out, sourceCreateJSON(source))
			}
			writeSourceCreateResult(out, source)
			return nil
		},
	}
	cmd.Flags().StringVar(&options.Type, "type", "", "Source contract type")
	cmd.Flags().BoolVar(&options.JSON, "json", false, "Emit JSON output for agents and automation")
	return cmd
}

func newSourceInitCommand(out io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "init <name>",
		Short: "Create a local source package from the default contract",
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

			writeSourceCreateResult(out, source)
			return nil
		},
	}
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

func writeSourceContractsList(out io.Writer, contracts []projectsource.Contract) {
	fmt.Fprintln(out, "Supported source contracts:")
	for _, contract := range contracts {
		marker := ""
		if contract.Default {
			marker = ", default"
		}
		fmt.Fprintf(out, "- %s (schema_version: %d, %s%s)\n", contract.Type, contract.SchemaVersion, contract.Status, marker)
		fmt.Fprintf(out, "  %s\n", contract.Description)
		fmt.Fprintf(out, "  Inspect: segmentstream source contracts --type %s\n", contract.Type)
		fmt.Fprintf(out, "  Create: segmentstream source create <name> --type %s\n", contract.Type)
	}
}

func writeSourceContractDetail(out io.Writer, contract projectsource.Contract) {
	fmt.Fprintf(out, "Source contract: %s\n", contract.Type)
	fmt.Fprintf(out, "Schema version: %d\n", contract.SchemaVersion)
	fmt.Fprintf(out, "Status: %s\n", contract.Status)
	if contract.Default {
		fmt.Fprintln(out, "Default: yes")
	} else {
		fmt.Fprintln(out, "Default: no")
	}
	fmt.Fprintf(out, "Model: %s\n", contract.Model.Name)
	fmt.Fprintf(out, "Partition: %s\n", contract.Model.Partition)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Columns:")
	for _, column := range contract.Columns {
		required := "optional"
		if column.Required {
			required = "required"
		}
		fmt.Fprintf(out, "- %s %s %s: %s\n", column.Name, column.Type, required, column.Description)
	}
}

func writeSourceCreateResult(out io.Writer, source projectsource.Source) {
	relativePath := sourceRelativePath(source)
	fmt.Fprintf(out, "Created source %q at %s\n", source.Name, relativePath)
	fmt.Fprintf(out, "Contract: %s (schema_version: %d)\n", source.Contract.Type, source.Contract.SchemaVersion)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Implement:")
	fmt.Fprintf(out, "- %s\n", sourceImplementationPath(source))
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Add this source to segmentstream.yml:")
	fmt.Fprint(out, sourceConfigSnippet(source))
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
