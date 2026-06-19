package cli

import (
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
	JSON              bool
}

type warehouseBrowseOptions struct {
	Path string
	JSON bool
}

type warehouseConfigureOptions struct {
	Project  string
	Dataset  string
	Location string
	JSON     bool
}

type warehouseTestOptions struct {
	JSON bool
}

func newWarehouseCommand(out, errOut io.Writer, credentialStore credentials.Store, registry warehouse.Registry, oauthLogin warehouseOAuthLogin) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "warehouse",
		Short: "Manage the configured data warehouse",
		Long: "Manage warehouse authentication, discovery, configuration, and access checks.\n\n" +
			"Warehouse subcommands read warehouse.type from segmentstream.yml. Credentials\n" +
			"are stored outside the project under ~/.segmentstream and segmentstream.yml\n" +
			"contains only the credential name.",
	}
	cmd.AddCommand(newWarehouseAuthCommand(out, errOut, credentialStore, oauthLogin))
	cmd.AddCommand(newWarehouseBrowseCommand(out, credentialStore, registry))
	cmd.AddCommand(newWarehouseConfigureCommand(out, credentialStore, registry))
	cmd.AddCommand(newWarehouseTestCommand(out, credentialStore, registry))
	return cmd
}

func newWarehouseAuthCommand(out, errOut io.Writer, credentialStore credentials.Store, oauthLogin warehouseOAuthLogin) *cobra.Command {
	options := warehouseAuthOptions{Name: "default-bigquery"}
	cmd := &cobra.Command{
		Use:   "auth [--service-account-key <path>]",
		Short: "Store or create warehouse authentication",
		Long: "Store warehouse authentication for the warehouse selected in segmentstream.yml.\n\n" +
			"Use --service-account-key to copy a BigQuery service-account key to\n" +
			"~/.segmentstream/bigquery/<name>.json. Use auth login to authenticate with\n" +
			"Google OAuth in a browser and store an authorized-user credential there.\n" +
			"No credential material is written to segmentstream.yml.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(options.ServiceAccountKey) == "" {
				return fmt.Errorf("--service-account-key is required, or run segmentstream warehouse auth login")
			}
			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("find current directory: %w", err)
			}
			store, config, err := loadWarehouseAuthConfig(projectRoot)
			if err != nil {
				return err
			}
			path, err := credentialStore.SaveServiceAccountKey(options.Name, options.ServiceAccountKey)
			if err != nil {
				return err
			}
			config.Warehouse.Auth = options.Name
			if err := store.SavePartial(config); err != nil {
				return err
			}

			result := struct {
				SchemaVersion string `json:"schema_version"`
				Warehouse     string `json:"warehouse"`
				Credential    string `json:"credential"`
				Path          string `json:"path"`
			}{
				SchemaVersion: cliresult.SchemaVersion,
				Warehouse:     "bigquery",
				Credential:    options.Name,
				Path:          path,
			}
			if options.JSON {
				return cliresult.WriteJSON(out, result)
			}
			fmt.Fprintf(out, "Stored BigQuery credential %q at %s\n", options.Name, path)
			fmt.Fprintf(out, "Updated %s warehouse.auth to %q\n", project.ConfigFileName, options.Name)
			return nil
		},
	}
	cmd.Flags().StringVar(&options.ServiceAccountKey, "service-account-key", "", "Path to a BigQuery service-account JSON key")
	cmd.Flags().StringVar(&options.Name, "name", "default-bigquery", "Credential name stored in segmentstream.yml as warehouse.auth")
	cmd.Flags().BoolVar(&options.JSON, "json", false, "Emit JSON output for agents and automation")
	cmd.AddCommand(newWarehouseAuthLoginCommand(out, errOut, credentialStore, oauthLogin))
	return cmd
}

func newWarehouseAuthLoginCommand(out, errOut io.Writer, credentialStore credentials.Store, oauthLogin warehouseOAuthLogin) *cobra.Command {
	options := warehouseAuthOptions{Name: "default-bigquery"}
	if oauthLogin == nil {
		oauthLogin = googleoauth.Login
	}
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate BigQuery with Google OAuth",
		Long: "Authenticate BigQuery with Google OAuth in a browser and store a local\n" +
			"authorized-user credential outside the project. The stored credential can be\n" +
			"used by SegmentStream, dbt, and Google client libraries as Application Default\n" +
			"Credentials.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("find current directory: %w", err)
			}
			store, config, err := loadWarehouseAuthConfig(projectRoot)
			if err != nil {
				return err
			}
			loginOut := out
			if options.JSON && errOut != nil {
				loginOut = errOut
			}
			credential, err := oauthLogin(cmd.Context(), loginOut)
			if err != nil {
				return err
			}
			path, err := credentialStore.SaveGoogleOAuthCredential(options.Name, credential)
			if err != nil {
				return err
			}
			config.Warehouse.Auth = options.Name
			if err := store.SavePartial(config); err != nil {
				return err
			}

			result := struct {
				SchemaVersion string `json:"schema_version"`
				Warehouse     string `json:"warehouse"`
				Credential    string `json:"credential"`
				Method        string `json:"method"`
				Path          string `json:"path"`
			}{
				SchemaVersion: cliresult.SchemaVersion,
				Warehouse:     "bigquery",
				Credential:    options.Name,
				Method:        "oauth",
				Path:          path,
			}
			if options.JSON {
				return cliresult.WriteJSON(out, result)
			}
			fmt.Fprintf(out, "Stored BigQuery OAuth credential %q at %s\n", options.Name, path)
			fmt.Fprintf(out, "Updated %s warehouse.auth to %q\n", project.ConfigFileName, options.Name)
			return nil
		},
	}
	cmd.Flags().StringVar(&options.Name, "name", "default-bigquery", "Credential name stored in segmentstream.yml as warehouse.auth")
	cmd.Flags().BoolVar(&options.JSON, "json", false, "Emit JSON output for agents and automation")
	return cmd
}

func newWarehouseBrowseCommand(out io.Writer, credentialStore credentials.Store, registry warehouse.Registry) *cobra.Command {
	options := warehouseBrowseOptions{}
	cmd := &cobra.Command{
		Use:   "browse",
		Short: "Browse warehouse projects and datasets",
		Long: "Browse the configured warehouse using the credential named by warehouse.auth.\n\n" +
			"Without --path, BigQuery browse lists accessible projects. With --path <project>,\n" +
			"it lists datasets in that project with their locations.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			connector, credentialPath, err := loadWarehouseBrowseState(credentialStore, registry)
			if err != nil {
				return err
			}
			result, err := connector.Browse(cmd.Context(), credentialPath, options.Path)
			if err != nil {
				return err
			}
			if options.JSON {
				return cliresult.WriteJSON(out, result)
			}
			if result.Level == "project" {
				fmt.Fprintln(out, "Projects:")
			} else {
				fmt.Fprintf(out, "Datasets in %s:\n", result.Path)
			}
			for _, child := range result.Children {
				if child.Location != "" {
					fmt.Fprintf(out, "- %s (%s)\n", child.ID, child.Location)
					continue
				}
				if child.FriendlyName != "" {
					fmt.Fprintf(out, "- %s (%s)\n", child.ID, child.FriendlyName)
					continue
				}
				fmt.Fprintf(out, "- %s\n", child.ID)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&options.Path, "path", "", "Browse below this path; for BigQuery, pass a project ID to list datasets")
	cmd.Flags().BoolVar(&options.JSON, "json", false, "Emit JSON output for agents and automation")
	return cmd
}

func newWarehouseConfigureCommand(out io.Writer, credentialStore credentials.Store, registry warehouse.Registry) *cobra.Command {
	options := warehouseConfigureOptions{}
	cmd := &cobra.Command{
		Use:   "configure --project <project> --dataset <dataset> --location <location>",
		Short: "Configure warehouse project, dataset, and location",
		Long: "Validate and write warehouse project, dataset, and location to segmentstream.yml.\n\n" +
			"For BigQuery, dataset IDs may contain only letters, numbers, and underscores.\n" +
			"If the dataset already exists, its location must match --location.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("find current directory: %w", err)
			}
			store := project.Store{Root: projectRoot}
			config, exists, err := store.LoadPartial()
			if err != nil {
				return err
			}
			if !exists || config.Warehouse.Type == "" {
				return fmt.Errorf("%s does not select a warehouse; run segmentstream init --warehouse bigquery first", project.ConfigFileName)
			}
			connector, err := registry.Connector(config.Warehouse.Type)
			if err != nil {
				return err
			}
			if config.Warehouse.Auth == "" {
				config.Warehouse.Auth = "default-bigquery"
			}
			credentialPath, err := credentialStore.BigQueryCredentialPath(config.Warehouse.Auth)
			if err != nil {
				return err
			}
			config.Warehouse.Project = options.Project
			config.Warehouse.Dataset = options.Dataset
			config.Warehouse.Location = options.Location
			result, err := connector.ValidateConfiguration(cmd.Context(), credentialPath, config.Warehouse)
			if err != nil {
				return err
			}
			if len(result.Diagnostics) == 0 {
				if err := store.Save(config); err != nil {
					return err
				}
			}
			if options.JSON {
				if err := cliresult.WriteJSON(out, result); err != nil {
					return err
				}
			} else {
				writeConfigureResult(out, result)
			}
			if len(result.Diagnostics) > 0 {
				return cliresult.WithExitCode(cliresult.ExitMisconfigured, nil)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&options.Project, "project", "", "Google Cloud project ID")
	cmd.Flags().StringVar(&options.Dataset, "dataset", "", "BigQuery dataset ID")
	cmd.Flags().StringVar(&options.Location, "location", "", "BigQuery dataset location, for example US or EU")
	cmd.Flags().BoolVar(&options.JSON, "json", false, "Emit JSON output for agents and automation")
	return cmd
}

func newWarehouseTestCommand(out io.Writer, credentialStore credentials.Store, registry warehouse.Registry) *cobra.Command {
	options := warehouseTestOptions{}
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test warehouse credential and IAM access",
		Long: "Test the configured warehouse credential and report granular access checks.\n\n" +
			"For BigQuery, this checks connect, read, create_table, and query_in_location.\n" +
			"The create_table check creates a temporary __segmentstream_probe_* table and\n" +
			"deletes it before returning.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("find current directory: %w", err)
			}
			config, err := project.LoadConfig(projectRoot)
			if err != nil {
				return err
			}
			connector, err := registry.Connector(config.Warehouse.Type)
			if err != nil {
				return err
			}
			credentialPath, err := credentialStore.BigQueryCredentialPath(config.Warehouse.Auth)
			if err != nil {
				return err
			}
			result, err := connector.Test(cmd.Context(), credentialPath, config.Warehouse)
			if err != nil {
				return err
			}
			if warehouse.AllChecksOK(result.Checks) {
				if err := credentialStore.SaveAccessMarker(config.Warehouse.Auth, config.Warehouse.Project, config.Warehouse.Dataset, config.Warehouse.Location); err != nil {
					return err
				}
			}
			if options.JSON {
				if err := cliresult.WriteJSON(out, result); err != nil {
					return err
				}
			} else {
				writeTestResult(out, result)
			}
			if !warehouse.AllChecksOK(result.Checks) {
				return cliresult.WithExitCode(cliresult.ExitMisconfigured, nil)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&options.JSON, "json", false, "Emit JSON output for agents and automation")
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

func writeConfigureResult(out io.Writer, result warehouse.ConfigureResult) {
	if len(result.Diagnostics) == 0 {
		fmt.Fprintln(out, "Warehouse configuration is valid.")
		return
	}
	fmt.Fprintln(out, "Warehouse configuration is invalid.")
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Field != "" {
			fmt.Fprintf(out, "- %s: %s\n", diagnostic.Field, diagnostic.Message)
			continue
		}
		fmt.Fprintf(out, "- %s\n", diagnostic.Message)
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
