package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/segmentstream/segmentstream-cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/internal/credentials"
	"github.com/segmentstream/segmentstream-cli/internal/googleoauth"
	"github.com/segmentstream/segmentstream-cli/internal/project"
	"github.com/segmentstream/segmentstream-cli/internal/warehouse"
	"github.com/spf13/cobra"
)

type warehouseAuthOptions struct {
	ServiceAccountKey string
	Name              string
	Port              int
}

type warehouseBrowseOptions struct {
	Path string
}

type warehouseConfigureOptions struct {
	Project       string
	Dataset       string
	Location      string
	CreateDataset bool
}

type warehouseAuthResult struct {
	SchemaVersion string `json:"schema_version"`
	Warehouse     string `json:"warehouse"`
	Credential    string `json:"credential"`
	Method        string `json:"method,omitempty"`
	Path          string `json:"path"`
}

type warehouseBrowseData warehouse.BrowseResult
type warehouseConfigureData warehouse.ConfigureResult
type warehouseTestData warehouse.TestResult

func newWarehouseCommand(out, errOut io.Writer, commandContext structuredCommandContext, credentialStore credentials.Store, registry warehouse.Registry, oauthLogin warehouseOAuthLogin) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "warehouse",
		Short: "Manage the configured data warehouse",
		Long: "Manage warehouse authentication, discovery, configuration, and access checks.\n\n" +
			"Warehouse subcommands read warehouse.type from segmentstream.yml. Credentials\n" +
			"are stored outside the project under ~/.segmentstream and segmentstream.yml\n" +
			"contains only the credential name.",
	}
	cmd.AddCommand(newWarehouseAuthCommand(out, errOut, commandContext, credentialStore, oauthLogin))
	cmd.AddCommand(newWarehouseBrowseCommand(out, commandContext, credentialStore, registry))
	cmd.AddCommand(newWarehouseConfigureCommand(out, commandContext, credentialStore, registry))
	cmd.AddCommand(newWarehouseTestCommand(out, commandContext, credentialStore, registry))
	return cmd
}

func newWarehouseAuthCommand(out, errOut io.Writer, commandContext structuredCommandContext, credentialStore credentials.Store, oauthLogin warehouseOAuthLogin) *cobra.Command {
	options := warehouseAuthOptions{Name: "default-bigquery"}
	cmd := newStructuredCommand(out, errOut, commandContext, structuredCommandSpec{
		Use:   "auth [--service-account-key <path>]",
		Short: "Store or create warehouse authentication",
		Long: "Store warehouse authentication for the warehouse selected in segmentstream.yml.\n\n" +
			"Use --service-account-key to copy a BigQuery service-account key to\n" +
			"~/.segmentstream/bigquery/<name>.json. Use auth login to print a Google\n" +
			"OAuth URL and store an authorized-user credential there after the loopback\n" +
			"redirect completes.\n" +
			"No credential material is written to segmentstream.yml.",
		Args:    cobra.NoArgs,
		Command: "warehouse.auth",
	}, func(ctx context.Context, args []string) (cliresult.Response, error) {
		_ = ctx
		if strings.TrimSpace(options.ServiceAccountKey) == "" {
			return cliresult.Response{}, fmt.Errorf("--service-account-key is required, or run segmentstream warehouse auth login")
		}
		projectRoot, err := os.Getwd()
		if err != nil {
			return cliresult.Response{}, fmt.Errorf("find current directory: %w", err)
		}
		store, config, err := loadWarehouseAuthConfig(projectRoot)
		if err != nil {
			return cliresult.Response{}, err
		}
		path, err := credentialStore.SaveServiceAccountKey(options.Name, options.ServiceAccountKey)
		if err != nil {
			return cliresult.Response{}, err
		}
		config.Warehouse.Auth = options.Name
		if err := store.SavePartial(config); err != nil {
			return cliresult.Response{}, err
		}

		result := warehouseAuthResult{
			SchemaVersion: cliresult.SchemaVersion,
			Warehouse:     "bigquery",
			Credential:    options.Name,
			Path:          path,
		}
		return cliresult.OK("warehouse.auth", result), nil
	})
	cmd.Flags().StringVar(&options.ServiceAccountKey, "service-account-key", "", "Path to a BigQuery service-account JSON key")
	cmd.Flags().StringVar(&options.Name, "name", "default-bigquery", "Credential name stored in segmentstream.yml as warehouse.auth")
	cmd.AddCommand(newWarehouseAuthLoginCommand(out, errOut, commandContext, credentialStore, oauthLogin))
	return cmd
}

func newWarehouseAuthLoginCommand(out, errOut io.Writer, commandContext structuredCommandContext, credentialStore credentials.Store, oauthLogin warehouseOAuthLogin) *cobra.Command {
	options := warehouseAuthOptions{Name: "default-bigquery"}
	if oauthLogin == nil {
		oauthLogin = googleoauth.LoginWithOptions
	}
	cmd := newStructuredCommand(out, errOut, commandContext, structuredCommandSpec{
		Use:   "login",
		Short: "Authenticate BigQuery with Google OAuth",
		Long: "Authenticate BigQuery with Google OAuth by printing a URL for the user to\n" +
			"open in a browser on the same computer. The command waits for Google's\n" +
			"loopback redirect and stores a local authorized-user credential outside the\n" +
			"project. The stored credential can be used by SegmentStream, dbt, and Google\n" +
			"client libraries as Application Default Credentials. For headless servers or\n" +
			"CI, use segmentstream warehouse auth --service-account-key=<path> instead.",
		Args:    cobra.NoArgs,
		Command: "warehouse.auth.login",
	}, func(ctx context.Context, args []string) (cliresult.Response, error) {
		if options.Port < 0 || options.Port > 65535 {
			return cliresult.Response{}, fmt.Errorf("invalid --port %d; use 0-65535", options.Port)
		}
		projectRoot, err := os.Getwd()
		if err != nil {
			return cliresult.Response{}, fmt.Errorf("find current directory: %w", err)
		}
		store, config, err := loadWarehouseAuthConfig(projectRoot)
		if err != nil {
			return cliresult.Response{}, err
		}
		loginOut := out
		if commandContext.Output != nil && commandContext.Output.JSON && errOut != nil {
			loginOut = errOut
		}
		credential, err := oauthLogin(ctx, loginOut, googleoauth.LoginOptions{
			Port: options.Port,
		})
		if err != nil {
			return cliresult.Response{}, err
		}
		path, err := credentialStore.SaveGoogleOAuthCredential(options.Name, credential)
		if err != nil {
			return cliresult.Response{}, err
		}
		config.Warehouse.Auth = options.Name
		if err := store.SavePartial(config); err != nil {
			return cliresult.Response{}, err
		}

		result := warehouseAuthResult{
			SchemaVersion: cliresult.SchemaVersion,
			Warehouse:     "bigquery",
			Credential:    options.Name,
			Method:        "oauth",
			Path:          path,
		}
		return cliresult.OK("warehouse.auth.login", result), nil
	})
	cmd.Flags().StringVar(&options.Name, "name", "default-bigquery", "Credential name stored in segmentstream.yml as warehouse.auth")
	cmd.Flags().IntVar(&options.Port, "port", 0, "Loopback callback port for Google OAuth; 0 chooses a random available port")
	return cmd
}

func newWarehouseBrowseCommand(out io.Writer, commandContext structuredCommandContext, credentialStore credentials.Store, registry warehouse.Registry) *cobra.Command {
	options := warehouseBrowseOptions{}
	cmd := newStructuredCommand(out, nil, commandContext, structuredCommandSpec{
		Use:   "browse",
		Short: "Browse warehouse projects, datasets, tables, and schemas",
		Long: "Browse the configured warehouse using the credential named by warehouse.auth.\n\n" +
			"Without --path, BigQuery browse lists accessible projects. With --path <project>,\n" +
			"it lists datasets in that project with their locations. With --path <project>/<dataset>,\n" +
			"it lists tables. With --path <project>/<dataset>/<table>, it returns the table schema.",
		Args:    cobra.NoArgs,
		Command: "warehouse.browse",
	}, func(ctx context.Context, args []string) (cliresult.Response, error) {
		connector, credentialPath, err := loadWarehouseBrowseState(credentialStore, registry)
		if err != nil {
			return cliresult.Response{}, err
		}
		result, err := connector.Browse(ctx, credentialPath, options.Path)
		if err != nil {
			return cliresult.Response{}, err
		}
		return cliresult.OK("warehouse.browse", warehouseBrowseData(result)), nil
	})
	cmd.Flags().StringVar(&options.Path, "path", "", "Browse below this path; for BigQuery, use <project>, <project>/<dataset>, or <project>/<dataset>/<table>")
	return cmd
}

func newWarehouseConfigureCommand(out io.Writer, commandContext structuredCommandContext, credentialStore credentials.Store, registry warehouse.Registry) *cobra.Command {
	options := warehouseConfigureOptions{}
	cmd := newStructuredCommand(out, nil, commandContext, structuredCommandSpec{
		Use:   "configure --project <project> --dataset <dataset> --location <location>",
		Short: "Configure warehouse project, dataset, and location",
		Long: "Validate and write warehouse project, dataset, and location to segmentstream.yml.\n\n" +
			"For BigQuery, dataset IDs may contain only letters, numbers, and underscores.\n" +
			"If the dataset already exists, its location must match --location.",
		Args:    cobra.NoArgs,
		Command: "warehouse.configure",
	}, func(ctx context.Context, args []string) (cliresult.Response, error) {
		projectRoot, err := os.Getwd()
		if err != nil {
			return cliresult.Response{}, fmt.Errorf("find current directory: %w", err)
		}
		store := project.Store{Root: projectRoot}
		config, exists, err := store.LoadPartial()
		if err != nil {
			return cliresult.Response{}, err
		}
		if !exists || config.Warehouse.Type == "" {
			return cliresult.Response{}, fmt.Errorf("%s does not select a warehouse; run segmentstream init --warehouse bigquery first", project.ConfigFileName)
		}
		connector, err := registry.Connector(config.Warehouse.Type)
		if err != nil {
			return cliresult.Response{}, err
		}
		if config.Warehouse.Auth == "" {
			config.Warehouse.Auth = "default-bigquery"
		}
		credentialPath, err := credentialStore.BigQueryCredentialPath(config.Warehouse.Auth)
		if err != nil {
			return cliresult.Response{}, err
		}
		config.Warehouse.Project = options.Project
		config.Warehouse.Dataset = options.Dataset
		config.Warehouse.Location = options.Location
		result, err := connector.ValidateConfiguration(ctx, credentialPath, config.Warehouse, warehouse.ConfigureOptions{
			CreateDataset: options.CreateDataset,
		})
		if err != nil {
			return cliresult.Response{}, err
		}
		if len(result.Diagnostics) == 0 {
			if err := store.Save(config); err != nil {
				return cliresult.Response{}, err
			}
		}
		data := warehouseConfigureData(result)
		if len(result.Diagnostics) > 0 {
			return cliresult.Invalid("warehouse.configure", data, result.Diagnostics), nil
		}
		return cliresult.OK("warehouse.configure", data), nil
	})
	cmd.Flags().StringVar(&options.Project, "project", "", "Google Cloud project ID")
	cmd.Flags().StringVar(&options.Dataset, "dataset", "", "BigQuery dataset ID")
	cmd.Flags().StringVar(&options.Location, "location", "", "BigQuery dataset location, for example US or EU")
	cmd.Flags().BoolVar(&options.CreateDataset, "create-dataset", false, "Create the BigQuery dataset if it is missing")
	return cmd
}

func newWarehouseTestCommand(out io.Writer, commandContext structuredCommandContext, credentialStore credentials.Store, registry warehouse.Registry) *cobra.Command {
	cmd := newStructuredCommand(out, nil, commandContext, structuredCommandSpec{
		Use:   "test",
		Short: "Test warehouse credential and IAM access",
		Long: "Test the configured warehouse credential and report granular access checks.\n\n" +
			"For BigQuery, this checks connect, read, create_table, and query_in_location.\n" +
			"The create_table check creates a temporary __segmentstream_probe_* table and\n" +
			"deletes it before returning.",
		Args:    cobra.NoArgs,
		Command: "warehouse.test",
	}, func(ctx context.Context, args []string) (cliresult.Response, error) {
		projectRoot, err := os.Getwd()
		if err != nil {
			return cliresult.Response{}, fmt.Errorf("find current directory: %w", err)
		}
		config, err := project.LoadConfig(projectRoot)
		if err != nil {
			return cliresult.Response{}, err
		}
		connector, err := registry.Connector(config.Warehouse.Type)
		if err != nil {
			return cliresult.Response{}, err
		}
		credentialPath, err := credentialStore.BigQueryCredentialPath(config.Warehouse.Auth)
		if err != nil {
			return cliresult.Response{}, err
		}
		result, err := connector.Test(ctx, credentialPath, config.Warehouse)
		if err != nil {
			return cliresult.Response{}, err
		}
		if warehouse.AllChecksOK(result.Checks) {
			if err := credentialStore.SaveAccessMarker(config.Warehouse.Auth, config.Warehouse.Project, config.Warehouse.Dataset, config.Warehouse.Location); err != nil {
				return cliresult.Response{}, err
			}
		}
		data := warehouseTestData(result)
		if !warehouse.AllChecksOK(result.Checks) {
			response := cliresult.OK("warehouse.test", data)
			response.Status = cliresult.StatusInvalid
			response.ExitCode = cliresult.ExitMisconfigured
			return response, nil
		}
		return cliresult.OK("warehouse.test", data), nil
	})
	return cmd
}

func loadWarehouseBrowseState(credentialStore credentials.Store, registry warehouse.Registry) (warehouse.Connector, string, error) {
	projectRoot, err := os.Getwd()
	if err != nil {
		return nil, "", fmt.Errorf("find current directory: %w", err)
	}
	config, exists, err := (project.Store{Root: projectRoot}).LoadPartial()
	if err != nil {
		return nil, "", err
	}
	if !exists || config.Warehouse.Type == "" {
		return nil, "", fmt.Errorf("%s does not select a warehouse; run segmentstream init --warehouse bigquery first", project.ConfigFileName)
	}
	connector, err := registry.Connector(config.Warehouse.Type)
	if err != nil {
		return nil, "", err
	}
	authName := config.Warehouse.Auth
	if authName == "" {
		authName = "default-bigquery"
	}
	credentialPath, err := credentialStore.BigQueryCredentialPath(authName)
	if err != nil {
		return nil, "", err
	}
	return connector, credentialPath, nil
}

func loadWarehouseAuthConfig(projectRoot string) (project.Store, project.Config, error) {
	store := project.Store{Root: projectRoot}
	config, exists, err := store.LoadPartial()
	if err != nil {
		return store, project.Config{}, err
	}
	if !exists || config.Warehouse.Type == "" {
		return store, project.Config{}, fmt.Errorf("%s does not select a warehouse; run segmentstream init --warehouse bigquery first", project.ConfigFileName)
	}
	if config.Warehouse.Type != "bigquery" {
		return store, project.Config{}, fmt.Errorf("unsupported warehouse.type %q", config.Warehouse.Type)
	}
	return store, config, nil
}

func loadWarehouseCommandState(credentialStore credentials.Store, registry warehouse.Registry) (project.Config, warehouse.Connector, string, error) {
	projectRoot, err := os.Getwd()
	if err != nil {
		return project.Config{}, nil, "", fmt.Errorf("find current directory: %w", err)
	}
	config, err := project.LoadConfig(projectRoot)
	if err != nil {
		return project.Config{}, nil, "", err
	}
	connector, err := registry.Connector(config.Warehouse.Type)
	if err != nil {
		return project.Config{}, nil, "", err
	}
	credentialPath, err := credentialStore.BigQueryCredentialPath(config.Warehouse.Auth)
	if err != nil {
		return project.Config{}, nil, "", err
	}
	return config, connector, credentialPath, nil
}

func (result warehouseAuthResult) HumanDocument() cliresult.Document {
	return textDocument(func(out io.Writer) {
		if result.Method == "oauth" {
			fmt.Fprintf(out, "Stored BigQuery OAuth credential %q at %s\n", result.Credential, result.Path)
		} else {
			fmt.Fprintf(out, "Stored BigQuery credential %q at %s\n", result.Credential, result.Path)
		}
		fmt.Fprintf(out, "Updated %s warehouse.auth to %q\n", project.ConfigFileName, result.Credential)
	})
}

func (data warehouseBrowseData) HumanDocument() cliresult.Document {
	return textDocument(func(out io.Writer) {
		writeBrowseResult(out, warehouse.BrowseResult(data))
	})
}

func (data warehouseConfigureData) HumanDocument() cliresult.Document {
	return textDocument(func(out io.Writer) {
		writeConfigureResult(out, warehouse.ConfigureResult(data))
	})
}

func (data warehouseTestData) HumanDocument() cliresult.Document {
	return textDocument(func(out io.Writer) {
		writeTestResult(out, warehouse.TestResult(data))
	})
}

func writeBrowseResult(out io.Writer, result warehouse.BrowseResult) {
	switch result.Level {
	case "project":
		fmt.Fprintln(out, "Projects:")
	case "dataset":
		fmt.Fprintf(out, "Datasets in %s:\n", result.Path)
	case "table":
		fmt.Fprintf(out, "Tables in %s:\n", result.Path)
	case "schema":
		fmt.Fprintf(out, "Schema for %s:\n", result.Path)
		for _, field := range result.Schema {
			writeBrowseField(out, field, "")
		}
		return
	default:
		fmt.Fprintf(out, "%s in %s:\n", result.Level, result.Path)
	}
	for _, child := range result.Children {
		writeBrowseChild(out, child)
	}
}

func writeBrowseChild(out io.Writer, child warehouse.BrowseChild) {
	if child.Location != "" {
		fmt.Fprintf(out, "- %s (%s)\n", child.ID, child.Location)
		return
	}
	if child.Type != "" && child.FriendlyName != "" {
		fmt.Fprintf(out, "- %s (%s, %s)\n", child.ID, child.FriendlyName, child.Type)
		return
	}
	if child.Type != "" {
		fmt.Fprintf(out, "- %s (%s)\n", child.ID, child.Type)
		return
	}
	if child.FriendlyName != "" {
		fmt.Fprintf(out, "- %s (%s)\n", child.ID, child.FriendlyName)
		return
	}
	fmt.Fprintf(out, "- %s\n", child.ID)
}

func writeBrowseField(out io.Writer, field warehouse.BrowseField, indent string) {
	mode := ""
	if field.Mode != "" {
		mode = " " + field.Mode
	}
	description := ""
	if field.Description != "" {
		description = " - " + field.Description
	}
	fmt.Fprintf(out, "%s- %s %s%s%s\n", indent, field.Name, field.Type, mode, description)
	for _, nested := range field.Fields {
		writeBrowseField(out, nested, indent+"  ")
	}
}

func writeConfigureResult(out io.Writer, result warehouse.ConfigureResult) {
	if len(result.Diagnostics) == 0 {
		fmt.Fprintln(out, "Warehouse configuration is valid.")
		for _, validation := range result.Validations {
			if validation.Status == "created" && validation.Message != "" {
				fmt.Fprintln(out, validation.Message)
			}
		}
		return
	}
	fmt.Fprintln(out, "Warehouse configuration is invalid.")
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Field != "" {
			fmt.Fprintf(out, "- %s: %s\n", diagnostic.Field, diagnostic.Message)
		} else {
			fmt.Fprintf(out, "- %s\n", diagnostic.Message)
		}
		if diagnostic.Suggestion != "" {
			if diagnostic.ID == "missing_dataset" {
				fmt.Fprintf(out, "  Next action: %s\n", diagnostic.Suggestion)
				continue
			}
			fmt.Fprintf(out, "  Suggestion: %s\n", diagnostic.Suggestion)
		}
	}
}

func writeTestResult(out io.Writer, result warehouse.TestResult) {
	if result.Status == "satisfied" {
		fmt.Fprintln(out, "Warehouse access checks passed.")
	} else {
		fmt.Fprintln(out, "Warehouse access checks failed.")
	}
	for _, check := range result.Checks {
		status := "ok"
		if !check.OK {
			status = "failed"
		}
		if check.Message != "" {
			fmt.Fprintf(out, "- %s: %s (%s)\n", check.ID, status, check.Message)
			continue
		}
		fmt.Fprintf(out, "- %s: %s\n", check.ID, status)
	}
}
