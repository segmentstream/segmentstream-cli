package initflow

import (
	"context"
	"fmt"

	"github.com/segmentstream/segmentstream-cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/internal/project"
)

const (
	defaultBigQueryAuth              = "default-bigquery"
	authWarehouseCommand             = "segmentstream warehouse auth --service-account-key <path>"
	configureWarehouseCommand        = "segmentstream warehouse configure --project <project> --dataset <dataset> --location <location>"
	testWarehouseCommand             = "segmentstream warehouse test"
	browseWarehouseProjectsCommand   = "segmentstream warehouse browse --json"
	browseWarehouseDatasetsCommand   = "segmentstream warehouse browse --path <project> --json"
	browseWarehouseBeforeConfigureID = "browse_warehouse_before_configure"

	statusSatisfiedWithWarnings = "satisfied_with_warnings"
	statusSatisfied             = "satisfied"
	statusMissing               = "missing"
	statusPending               = "pending"
	statusInvalid               = "invalid"
	statusUntested              = "untested"

	actionAskUser    = "ask_user"
	actionRunCommand = "run_command"
	actionDone       = "done"
)

type stageID string

const (
	stagePrerequisites   stageID = "prerequisites"
	stageWarehouseType   stageID = "warehouse_type"
	stageWarehouseAuth   stageID = "warehouse_auth"
	stageWarehouseConfig stageID = "warehouse_config"
	stageWarehouseAccess stageID = "warehouse_access"
)

type Options struct {
	SelectWarehouse string
}

type Result struct {
	Envelope cliresult.Envelope
	ExitCode int
}

type Service struct {
	ProjectRoot  string
	ProjectStore ProjectStore
	Credentials  CredentialStore
	Scaffolder   ProjectScaffolder
}

type stageSpec struct {
	ID        stageID
	DependsOn []stageID
}

var stagePlan = []stageSpec{
	{ID: stagePrerequisites},
	{ID: stageWarehouseType, DependsOn: []stageID{stagePrerequisites}},
	{ID: stageWarehouseAuth, DependsOn: []stageID{stageWarehouseType}},
	{ID: stageWarehouseConfig, DependsOn: []stageID{stageWarehouseAuth}},
	{ID: stageWarehouseAccess, DependsOn: []stageID{stageWarehouseConfig}},
}

type blocker struct {
	StageID     stageID
	Status      string
	ExitCode    int
	NextAction  cliresult.NextAction
	Diagnostics []cliresult.Diagnostic
}

type evaluation struct {
	completed map[stageID]bool
	blocker   *blocker
	ready     bool
}

func (service Service) Evaluate(ctx context.Context, options Options) (Result, error) {
	_ = ctx

	store := service.projectStore()
	credentialStore := service.credentialStore()
	if options.SelectWarehouse != "" {
		if _, err := store.SelectWarehouse(options.SelectWarehouse); err != nil {
			return Result{}, err
		}
		if err := service.projectScaffolder().EnsureInitFiles(); err != nil {
			return Result{}, err
		}
	}

	config, exists, err := store.LoadPartial()
	if err != nil {
		return Result{}, err
	}

	envelope := baseEnvelope(config)
	eval := newEvaluation()
	eval.complete(stagePrerequisites)

	if !exists || config.Warehouse.Type == "" {
		return resultFor(envelope, eval.withBlocker(blocker{
			StageID:    stageWarehouseType,
			Status:     statusMissing,
			ExitCode:   cliresult.ExitNeedsUserDecision,
			NextAction: selectWarehouseAction(),
		})), nil
	}

	if config.Warehouse.Type != "bigquery" {
		return resultFor(envelope, eval.withBlocker(blocker{
			StageID:     stageWarehouseType,
			Status:      statusInvalid,
			ExitCode:    cliresult.ExitNeedsUserDecision,
			NextAction:  unsupportedWarehouseAction(),
			Diagnostics: unsupportedWarehouseDiagnostics(config.Warehouse.Type),
		})), nil
	}
	eval.complete(stageWarehouseType)

	authName := warehouseAuthName(config)
	hasCredential, err := credentialStore.HasBigQueryCredential(authName)
	if err != nil {
		return Result{}, err
	}
	if !hasCredential {
		return resultFor(envelope, eval.withBlocker(blocker{
			StageID:    stageWarehouseAuth,
			Status:     statusMissing,
			ExitCode:   cliresult.ExitNeedsAuth,
			NextAction: authenticateWarehouseAction(),
		})), nil
	}
	eval.complete(stageWarehouseAuth)

	diagnostics := configDiagnostics(config)
	if len(diagnostics) > 0 {
		return resultFor(envelope, eval.withBlocker(blocker{
			StageID:     stageWarehouseConfig,
			Status:      statusInvalid,
			ExitCode:    cliresult.ExitMisconfigured,
			NextAction:  configureWarehouseAction(),
			Diagnostics: diagnostics,
		})), nil
	}
	eval.complete(stageWarehouseConfig)

	verified, err := credentialStore.HasMatchingAccessMarker(authName, config.Warehouse.Project, config.Warehouse.Dataset, config.Warehouse.Location)
	if err != nil {
		return Result{}, err
	}
	if !verified {
		return resultFor(envelope, eval.withBlocker(blocker{
			StageID:    stageWarehouseAccess,
			Status:     statusUntested,
			ExitCode:   cliresult.ExitMisconfigured,
			NextAction: testWarehouseAction(),
		})), nil
	}
	eval.complete(stageWarehouseAccess)

	eval.ready = true
	return resultFor(envelope, eval), nil
}

func baseEnvelope(config project.Config) cliresult.Envelope {
	warehouseName := config.Warehouse.Type
	var warehouse *string
	if warehouseName != "" {
		warehouse = &warehouseName
	}

	return cliresult.Envelope{
		SchemaVersion: cliresult.SchemaVersion,
		Warehouse:     warehouse,
		Warnings: []cliresult.Warning{
			{
				ID:          "docker",
				RequiredFor: "run",
				Fix:         "Install Docker Desktop or Docker Engine with Docker Compose V2 before running segmentstream run.",
			},
		},
	}
}

func newEvaluation() evaluation {
	return evaluation{completed: map[stageID]bool{}}
}

func (eval *evaluation) complete(id stageID) {
	eval.completed[id] = true
}

func (eval evaluation) withBlocker(blocker blocker) evaluation {
	eval.blocker = &blocker
	return eval
}

func resultFor(envelope cliresult.Envelope, eval evaluation) Result {
	envelope.Ready = eval.ready
	envelope.Stages = buildStages(stagePlan, eval.completed, eval.blocker)
	if eval.blocker == nil {
		envelope.NextAction = doneAction()
		return Result{Envelope: envelope, ExitCode: cliresult.ExitReady}
	}

	envelope.NextAction = eval.blocker.NextAction
	envelope.Diagnostics = eval.blocker.Diagnostics
	return Result{Envelope: envelope, ExitCode: eval.blocker.ExitCode}
}

func buildStages(plan []stageSpec, completed map[stageID]bool, blocker *blocker) []cliresult.Stage {
	stages := make([]cliresult.Stage, 0, len(plan))
	for _, spec := range plan {
		status := statusPending
		if completed[spec.ID] && dependenciesCompleted(spec, completed) {
			status = completedStageStatus(spec.ID)
		}
		current := false
		if blocker != nil && blocker.StageID == spec.ID {
			status = blocker.Status
			current = true
		}
		stages = append(stages, cliresult.Stage{
			ID:      string(spec.ID),
			Status:  status,
			Current: current,
		})
	}
	return stages
}

func dependenciesCompleted(spec stageSpec, completed map[stageID]bool) bool {
	for _, dependency := range spec.DependsOn {
		if !completed[dependency] {
			return false
		}
	}
	return true
}

func completedStageStatus(id stageID) string {
	if id == stagePrerequisites {
		return statusSatisfiedWithWarnings
	}
	return statusSatisfied
}

func selectWarehouseAction() cliresult.NextAction {
	return cliresult.NextAction{
		Type:          actionAskUser,
		HumanRequired: true,
		Reason:        "Select the warehouse SegmentStream should use.",
		Options: []cliresult.NextActionOption{
			{Value: "bigquery", Status: "available"},
			{Value: "snowflake", Status: "coming_soon"},
			{Value: "databricks", Status: "coming_soon"},
		},
	}
}

func unsupportedWarehouseAction() cliresult.NextAction {
	return cliresult.NextAction{
		Type:          actionAskUser,
		HumanRequired: true,
		Reason:        "Only BigQuery is available in this release.",
	}
}

func authenticateWarehouseAction() cliresult.NextAction {
	return cliresult.NextAction{
		Type:          actionRunCommand,
		Command:       authWarehouseCommand,
		HumanRequired: true,
		Reason:        "No BigQuery credential was found for warehouse.auth.",
	}
}

func configureWarehouseAction() cliresult.NextAction {
	return cliresult.NextAction{
		Type:          actionRunCommand,
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
}

func testWarehouseAction() cliresult.NextAction {
	return cliresult.NextAction{
		Type:    actionRunCommand,
		Command: testWarehouseCommand,
		Reason:  "Warehouse access has not been verified for this project, dataset, and location.",
	}
}

func doneAction() cliresult.NextAction {
	return cliresult.NextAction{
		Type:      actionDone,
		Suggested: "segmentstream run",
	}
}

func unsupportedWarehouseDiagnostics(warehouseType string) []cliresult.Diagnostic {
	return []cliresult.Diagnostic{{
		ID:      "unsupported_warehouse",
		Field:   "warehouse.type",
		Message: fmt.Sprintf("Unsupported warehouse.type %q.", warehouseType),
	}}
}

func warehouseAuthName(config project.Config) string {
	if config.Warehouse.Auth != "" {
		return config.Warehouse.Auth
	}
	return defaultBigQueryAuth
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
