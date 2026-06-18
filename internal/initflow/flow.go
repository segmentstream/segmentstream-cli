package initflow

import (
	"context"
	"fmt"

	"github.com/segmentstream/segmentstream-cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/internal/credentials"
	"github.com/segmentstream/segmentstream-cli/internal/project"
)

const (
	defaultBigQueryAuth              = "default-bigquery"
	configureWarehouseCommand        = "segmentstream warehouse configure --project <project> --dataset <dataset> --location <location>"
	browseWarehouseProjectsCommand   = "segmentstream warehouse browse --json"
	browseWarehouseDatasetsCommand   = "segmentstream warehouse browse --path <project> --json"
	browseWarehouseBeforeConfigureID = "browse_warehouse_before_configure"
)

type Options struct {
	SelectWarehouse string
}

type Result struct {
	Envelope cliresult.Envelope
	ExitCode int
}

type Service struct {
	ProjectRoot string
	Credentials credentials.Store
}

func (service Service) Evaluate(ctx context.Context, options Options) (Result, error) {
	_ = ctx

	store := project.Store{Root: service.ProjectRoot}
	if options.SelectWarehouse != "" {
		if _, err := store.SelectWarehouse(options.SelectWarehouse); err != nil {
			return Result{}, err
		}
		if err := project.EnsureRuntimeGitignored(service.ProjectRoot); err != nil {
			return Result{}, err
		}
		if _, err := project.EnsureProjectReadme(service.ProjectRoot); err != nil {
			return Result{}, err
		}
		if _, err := project.EnsureAgentGuide(service.ProjectRoot); err != nil {
			return Result{}, err
		}
	}

	config, exists, err := store.LoadPartial()
	if err != nil {
		return Result{}, err
	}

	warnings := []cliresult.Warning{
		{
			ID:          "docker",
			RequiredFor: "run",
			Fix:         "Install Docker Desktop or Docker Engine with Docker Compose V2 before running segmentstream run.",
		},
	}
	warehouseName := config.Warehouse.Type
	var warehouse *string
	if warehouseName != "" {
		warehouse = &warehouseName
	}

	envelope := cliresult.Envelope{
		SchemaVersion: cliresult.SchemaVersion,
		Warehouse:     warehouse,
		Warnings:      warnings,
	}

	if !exists || config.Warehouse.Type == "" {
		envelope.Stages = []cliresult.Stage{
			{ID: "prerequisites", Status: "satisfied_with_warnings"},
			{ID: "warehouse_type", Status: "missing", Current: true},
			{ID: "warehouse_auth", Status: "pending"},
			{ID: "warehouse_config", Status: "pending"},
			{ID: "warehouse_access", Status: "pending"},
		}
		envelope.NextAction = cliresult.NextAction{
			Type:          "ask_user",
			HumanRequired: true,
			Reason:        "Select the warehouse SegmentStream should use.",
			Options: []cliresult.NextActionOption{
				{Value: "bigquery", Status: "available"},
				{Value: "snowflake", Status: "coming_soon"},
				{Value: "databricks", Status: "coming_soon"},
			},
		}
		return Result{Envelope: envelope, ExitCode: cliresult.ExitNeedsUserDecision}, nil
	}

	if config.Warehouse.Type != "bigquery" {
		envelope.Diagnostics = []cliresult.Diagnostic{{
			ID:      "unsupported_warehouse",
			Field:   "warehouse.type",
			Message: fmt.Sprintf("Unsupported warehouse.type %q.", config.Warehouse.Type),
		}}
		envelope.Stages = []cliresult.Stage{
			{ID: "prerequisites", Status: "satisfied_with_warnings"},
			{ID: "warehouse_type", Status: "invalid", Current: true},
			{ID: "warehouse_auth", Status: "pending"},
			{ID: "warehouse_config", Status: "pending"},
			{ID: "warehouse_access", Status: "pending"},
		}
		envelope.NextAction = cliresult.NextAction{
			Type:          "ask_user",
			HumanRequired: true,
			Reason:        "Only BigQuery is available in this release.",
		}
		return Result{Envelope: envelope, ExitCode: cliresult.ExitNeedsUserDecision}, nil
	}

	authName := config.Warehouse.Auth
	if authName == "" {
		authName = defaultBigQueryAuth
	}
	hasCredential, err := service.Credentials.HasBigQueryCredential(authName)
	if err != nil {
		return Result{}, err
	}
	if !hasCredential {
		envelope.Stages = []cliresult.Stage{
			{ID: "prerequisites", Status: "satisfied_with_warnings"},
			{ID: "warehouse_type", Status: "satisfied"},
			{ID: "warehouse_auth", Status: "missing", Current: true},
			{ID: "warehouse_config", Status: "pending"},
			{ID: "warehouse_access", Status: "pending"},
		}
		envelope.NextAction = cliresult.NextAction{
			Type:          "run_command",
			Command:       "segmentstream warehouse auth --service-account-key <path>",
			HumanRequired: true,
			Reason:        "No BigQuery credential was found for warehouse.auth.",
		}
		return Result{Envelope: envelope, ExitCode: cliresult.ExitNeedsAuth}, nil
	}

	diagnostics := configDiagnostics(config)
	if len(diagnostics) > 0 {
		envelope.Diagnostics = diagnostics
		envelope.Stages = []cliresult.Stage{
			{ID: "prerequisites", Status: "satisfied_with_warnings"},
			{ID: "warehouse_type", Status: "satisfied"},
			{ID: "warehouse_auth", Status: "satisfied"},
			{ID: "warehouse_config", Status: "invalid", Current: true},
			{ID: "warehouse_access", Status: "pending"},
		}
		envelope.NextAction = cliresult.NextAction{
			Type:          "run_command",
			Command:       configureWarehouseCommand,
			HumanRequired: true,
			Reason:        "Warehouse project, dataset, or location is not configured.",
			Hints: []cliresult.NextActionHint{
				{
					ID:      browseWarehouseBeforeConfigureID,
					Message: "Use warehouse browse to discover accessible projects, datasets, and locations before configuring.",
					Commands: []string{
						browseWarehouseProjectsCommand,
						browseWarehouseDatasetsCommand,
					},
				},
			},
		}
		return Result{Envelope: envelope, ExitCode: cliresult.ExitMisconfigured}, nil
	}

	verified, err := service.Credentials.HasMatchingAccessMarker(authName, config.Warehouse.Project, config.Warehouse.Dataset, config.Warehouse.Location)
	if err != nil {
		return Result{}, err
	}
	if !verified {
		envelope.Stages = []cliresult.Stage{
			{ID: "prerequisites", Status: "satisfied_with_warnings"},
			{ID: "warehouse_type", Status: "satisfied"},
			{ID: "warehouse_auth", Status: "satisfied"},
			{ID: "warehouse_config", Status: "satisfied"},
			{ID: "warehouse_access", Status: "untested", Current: true},
		}
		envelope.NextAction = cliresult.NextAction{
			Type:    "run_command",
			Command: "segmentstream warehouse test",
			Reason:  "Warehouse access has not been verified for this project, dataset, and location.",
		}
		return Result{Envelope: envelope, ExitCode: cliresult.ExitMisconfigured}, nil
	}

	envelope.Ready = true
	envelope.Stages = []cliresult.Stage{
		{ID: "prerequisites", Status: "satisfied_with_warnings"},
		{ID: "warehouse_type", Status: "satisfied"},
		{ID: "warehouse_auth", Status: "satisfied"},
		{ID: "warehouse_config", Status: "satisfied"},
		{ID: "warehouse_access", Status: "satisfied"},
	}
	envelope.NextAction = cliresult.NextAction{
		Type:      "done",
		Suggested: "segmentstream run",
	}
	return Result{Envelope: envelope, ExitCode: cliresult.ExitReady}, nil
}

func configDiagnostics(config project.Config) []cliresult.Diagnostic {
	var diagnostics []cliresult.Diagnostic
	if config.Warehouse.Auth == "" {
		diagnostics = append(diagnostics, cliresult.Diagnostic{
			ID:         "missing_auth",
			Field:      "warehouse.auth",
			Message:    "warehouse.auth is required.",
			Suggestion: defaultBigQueryAuth,
		})
	}
	if config.Warehouse.Project == "" || config.Warehouse.Project == "your-gcp-project" {
		diagnostics = append(diagnostics, cliresult.Diagnostic{
			ID:      "missing_project",
			Field:   "warehouse.project",
			Message: "warehouse.project must be set to a real Google Cloud project ID.",
		})
	}
	if config.Warehouse.Dataset == "" {
		diagnostics = append(diagnostics, cliresult.Diagnostic{
			ID:      "missing_dataset",
			Field:   "warehouse.dataset",
			Message: "warehouse.dataset must be set to a BigQuery dataset ID.",
		})
	} else if err := project.ValidateBigQueryDatasetID(config.Warehouse.Dataset); err != nil {
		diagnostics = append(diagnostics, cliresult.Diagnostic{
			ID:         "invalid_dataset",
			Field:      "warehouse.dataset",
			Message:    err.Error(),
			Suggestion: suggestedDataset(config.Warehouse.Dataset),
		})
	}
	if config.Warehouse.Location == "" {
		diagnostics = append(diagnostics, cliresult.Diagnostic{
			ID:         "missing_location",
			Field:      "warehouse.location",
			Message:    "warehouse.location must be set to the BigQuery dataset location.",
			Suggestion: project.DefaultLocation,
		})
	}
	return diagnostics
}

func suggestedDataset(dataset string) string {
	if dataset == "" {
		return ""
	}
	out := []rune(dataset)
	for i, char := range out {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '_' {
			continue
		}
		out[i] = '_'
	}
	return string(out)
}
