package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/segmentstream/segmentstream-cli/internal/cliresult"
	sourcepkg "github.com/segmentstream/segmentstream-cli/internal/source"
	"github.com/spf13/cobra"
)

type sourceContractsOptions struct {
	Type string
}

type sourceScaffoldOptions struct {
	Type string
}

type sourceVerifyOptions struct {
	StartDate string
}

type sourceContractAction struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

type sourceContractSummary struct {
	Contract    sourcepkg.ContractIdentity `json:"contract"`
	Description string                     `json:"description"`
	Default     bool                       `json:"default,omitempty"`
	Status      string                     `json:"status"`
	Model       string                     `json:"model"`
	Actions     []sourceContractAction     `json:"actions"`
}

type sourceContractsListResult struct {
	SchemaVersion string                  `json:"schema_version"`
	Contracts     []sourceContractSummary `json:"contracts"`
}

type sourceContractDetailResult struct {
	SchemaVersion string                     `json:"schema_version"`
	Contract      sourcepkg.ContractIdentity `json:"contract"`
	Description   string                     `json:"description"`
	Default       bool                       `json:"default,omitempty"`
	Status        string                     `json:"status"`
	Model         sourcepkg.ContractModel    `json:"model"`
	Columns       []sourcepkg.ContractColumn `json:"columns"`
	Actions       []sourceContractAction     `json:"actions"`
}

type sourceScaffoldAction struct {
	Type    string `json:"type"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

type sourceScaffoldResult struct {
	SchemaVersion string                     `json:"schema_version"`
	Source        sourceScaffoldResultSource `json:"source"`
	Directory     string                     `json:"directory"`
	CreatedFiles  []string                   `json:"created_files"`
	Contract      sourcepkg.ContractIdentity `json:"contract"`
	Actions       []sourceScaffoldAction     `json:"actions"`
}

type sourceScaffoldResultSource struct {
	Name        string `json:"name"`
	PackageName string `json:"package_name"`
}

type sourceVerifyResult struct {
	SchemaVersion    string `json:"schema_version"`
	Source           string `json:"source"`
	SourcePath       string `json:"source_path"`
	Status           string `json:"source_verify"`
	StartDate        string `json:"start_date"`
	EndExclusiveDate string `json:"end_exclusive_date"`
	EndInclusiveDate string `json:"-"`
	MarkerPath       string `json:"marker_path"`
	Fingerprint      string `json:"fingerprint"`
}

func newSourceCommand(out, errOut io.Writer, commandContext structuredCommandContext, runner commandRunner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "source",
		Short: "Manage SegmentStream sources",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown source command %q", args[0])
			}
			return cmd.Help()
		},
	}

	cmd.AddCommand(newSourceContractsCommand(out, commandContext))
	cmd.AddCommand(newSourceScaffoldCommand(out, commandContext))
	cmd.AddCommand(newSourceVerifyCommand(out, errOut, commandContext, runner))

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
			contract, err := sourcepkg.ContractByType(options.Type)
			if err != nil {
				return cliresult.Response{}, err
			}
			return cliresult.OK("source.contracts", sourceContractDetail(contract)), nil
		}

		contracts, err := sourcepkg.Contracts()
		if err != nil {
			return cliresult.Response{}, err
		}
		return cliresult.OK("source.contracts", sourceContractsList(contracts)), nil
	})
	cmd.Flags().StringVar(&options.Type, "type", "", "Show the full schema for a source contract type")
	return cmd
}

func newSourceScaffoldCommand(out io.Writer, commandContext structuredCommandContext) *cobra.Command {
	options := sourceScaffoldOptions{}
	cmd := newStructuredCommand(out, nil, commandContext, structuredCommandSpec{
		Use:   "scaffold <name> --type <contract>",
		Short: "Scaffold a source template from a contract",
		Long: "Scaffold a local source template from a contract.\n\n" +
			"The generated source is not implemented yet. Read IMPLEMENTATION_GUIDE.md\n" +
			"inside the generated source directory, then declare raw inputs and write\n" +
			"the source model SQL.",
		Args:    cobra.ExactArgs(1),
		Command: "source.scaffold",
	}, func(ctx context.Context, args []string) (cliresult.Response, error) {
		_ = ctx
		if options.Type == "" {
			return cliresult.Response{}, fmt.Errorf("--type is required; run segmentstream source contracts to list supported contracts")
		}
		projectRoot, err := os.Getwd()
		if err != nil {
			return cliresult.Response{}, fmt.Errorf("find current directory: %w", err)
		}

		source, err := sourcepkg.Create(projectRoot, args[0], options.Type)
		if err != nil {
			return cliresult.Response{}, err
		}
		return cliresult.OK("source.scaffold", sourceScaffoldJSON(source)), nil
	})
	cmd.Flags().StringVar(&options.Type, "type", "", "Source contract type")
	return cmd
}

func newSourceVerifyCommand(out, errOut io.Writer, commandContext structuredCommandContext, runner commandRunner) *cobra.Command {
	options := sourceVerifyOptions{}
	cmd := newStructuredCommand(out, errOut, commandContext, structuredCommandSpec{
		Use:   "verify <name>",
		Short: "Verify a source implementation with dbt tests in Docker",
		Long: "Verify a declared source implementation with the dbt tests in its source package.\n\n" +
			"The command prepares the local SegmentStream Docker runtime, then runs dbt\n" +
			"inside Docker against the source package. No source verification SQL or\n" +
			"scripts are generated by the CLI.",
		Args:    cobra.ExactArgs(1),
		Command: "source.verify",
	}, func(ctx context.Context, args []string) (cliresult.Response, error) {
		projectRoot, err := os.Getwd()
		if err != nil {
			return cliresult.Response{}, fmt.Errorf("find current directory: %w", err)
		}
		progressOut := out
		if commandContext.Output != nil && commandContext.Output.JSON && errOut != nil {
			progressOut = errOut
		}
		result, err := sourcepkg.Verify(ctx, sourcepkg.VerifyRequest{
			ProjectRoot:        projectRoot,
			SourceName:         args[0],
			StartDate:          options.StartDate,
			Runner:             sourceCommandRunner{runner: runner},
			Progress:           newRunProgress(progressOut, 4),
			WarehousePreflight: preflightWarehouseAuth,
			Now:                currentTime,
		})
		if err != nil {
			return cliresult.Response{}, err
		}
		return cliresult.OK("source.verify", sourceVerifyJSON(result)), nil
	})
	cmd.Flags().StringVar(&options.StartDate, "start-date", "", "Verify from this UTC date in YYYY-MM-DD format; defaults to the last 7 days")
	return cmd
}

type sourceCommandRunner struct {
	runner commandRunner
}

func (r sourceCommandRunner) LookPath(file string) (string, error) {
	return r.runner.LookPath(file)
}

func (r sourceCommandRunner) Run(ctx context.Context, invocation sourcepkg.CommandInvocation) (string, error) {
	return r.runner.Run(ctx, commandInvocation{
		Name: invocation.Name,
		Args: invocation.Args,
		Dir:  invocation.Dir,
	})
}

func sourceContractsList(contracts []sourcepkg.Contract) sourceContractsListResult {
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

func sourceContractDetail(contract sourcepkg.Contract) sourceContractDetailResult {
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

func sourceContractActions(contract sourcepkg.Contract) []sourceContractAction {
	return []sourceContractAction{
		{
			Type:    "inspect_schema",
			Command: fmt.Sprintf("segmentstream source contracts --type %s --json", contract.Type),
		},
		{
			Type:    "scaffold_source",
			Command: fmt.Sprintf("segmentstream source scaffold <name> --type %s --json", contract.Type),
		},
	}
}

func sourceScaffoldJSON(source sourcepkg.Source) sourceScaffoldResult {
	relativePath := sourceRelativePath(source)
	return sourceScaffoldResult{
		SchemaVersion: cliresult.SchemaVersion,
		Source: sourceScaffoldResultSource{
			Name:        source.Name,
			PackageName: source.PackageName,
		},
		Directory:    relativePath,
		CreatedFiles: append([]string(nil), source.CreatedFiles...),
		Contract:     source.Contract,
		Actions: []sourceScaffoldAction{
			{
				Type:    "read_implementation_guide",
				Path:    sourceImplementationGuidePath(source),
				Message: "Read this guide to implement the source package.",
			},
		},
	}
}

func sourceVerifyJSON(result sourcepkg.VerifyResult) sourceVerifyResult {
	return sourceVerifyResult{
		SchemaVersion:    cliresult.SchemaVersion,
		Source:           result.Source,
		SourcePath:       result.SourcePath,
		Status:           result.Status,
		StartDate:        result.StartDate,
		EndExclusiveDate: result.EndExclusiveDate,
		EndInclusiveDate: result.EndInclusiveDate,
		MarkerPath:       result.MarkerPath,
		Fingerprint:      result.Fingerprint,
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
				case "scaffold_source":
					fmt.Fprintf(out, "  Scaffold: %s\n", strings.TrimSuffix(action.Command, " --json"))
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

func (result sourceScaffoldResult) HumanDocument() cliresult.Document {
	return textDocument(func(out io.Writer) {
		fmt.Fprintf(out, "Scaffolded source template %q at %s\n", result.Source.Name, result.Directory)
		fmt.Fprintf(out, "Contract: %s (schema_version: %d)\n", result.Contract.Type, result.Contract.SchemaVersion)
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Next action:")
		for _, action := range result.Actions {
			if action.Type == "read_implementation_guide" && action.Path != "" {
				fmt.Fprintf(out, "- Read %s to implement this source.\n", action.Path)
			}
		}
	})
}

func (result sourceVerifyResult) HumanDocument() cliresult.Document {
	return textDocument(func(out io.Writer) {
		fmt.Fprintf(out, "Source %q verification passed.\n", result.Source)
		fmt.Fprintf(out, "Window: %s through %s\n", result.StartDate, result.EndInclusiveDate)
		fmt.Fprintf(out, "Marker: %s\n", result.MarkerPath)
	})
}

func sourceRelativePath(source sourcepkg.Source) string {
	return filepath.ToSlash(filepath.Join(sourcepkg.SourcesDirName, source.Name))
}

func sourceImplementationGuidePath(source sourcepkg.Source) string {
	return filepath.ToSlash(filepath.Join(sourceRelativePath(source), "IMPLEMENTATION_GUIDE.md"))
}
