package initflow

import (
	"context"
	"fmt"

	"github.com/segmentstream/segmentstream-cli/cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/cli/internal/credentials"
	"github.com/segmentstream/segmentstream-cli/cli/internal/project"
	"github.com/segmentstream/segmentstream-cli/cli/internal/warehouse"
)

const (
	testWarehouseCommand   = "segmentstream warehouse test --json"
	sourceContractsCommand = "segmentstream source contracts"
	initVerifyCommand      = "segmentstream init --json"
	runCommand             = "segmentstream run"

	statusSatisfied = "satisfied"
	statusMissing   = "missing"
	statusPending   = "pending"
	statusInvalid   = "invalid"
	statusUntested  = "untested"

	actionHumanInput = "human_input"
	actionRunCommand = "run_command"
)

type stageID string

const (
	stagePrerequisites   stageID = "prerequisites"
	stageWarehouseType   stageID = "warehouse_type"
	stageWarehouseAuth   stageID = "warehouse_auth"
	stageWarehouseConfig stageID = "warehouse_config"
	stageWarehouseAccess stageID = "warehouse_access"
	stageSources         stageID = "sources"
)

type Options struct {
	SelectWarehouse string
}

type Result struct {
	Envelope cliresult.Envelope
	ExitCode int
}

type Service struct {
	ProjectRoot       string
	ProjectStore      ProjectStore
	Credentials       credentials.Store
	CredentialStore   CredentialStore
	WarehouseRegistry warehouse.Registry
	Scaffolder        ProjectScaffolder
	SourceVerifier    SourceVerifier
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
	{ID: stageSources, DependsOn: []stageID{stageWarehouseAccess}},
}

type blocker struct {
	StageID     stageID
	Status      string
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
	sourceVerifier := service.sourceVerifier()
	registry := service.WarehouseRegistry
	if options.SelectWarehouse != "" {
		provider, err := registry.Provider(options.SelectWarehouse)
		if err != nil {
			return Result{}, err
		}
		if _, err := store.SelectWarehouse(options.SelectWarehouse, provider.DefaultAuthName()); err != nil {
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

	envelope := baseEnvelope(config, registry)
	eval := newEvaluation()
	eval.complete(stagePrerequisites)

	if !exists || config.Warehouse.Type == "" {
		return resultFor(envelope, eval.withBlocker(blocker{
			StageID:    stageWarehouseType,
			Status:     statusMissing,
			NextAction: selectWarehouseAction(registry),
		})), nil
	}

	provider, err := registry.Provider(config.Warehouse.Type)
	if err != nil {
		return resultFor(envelope, eval.withBlocker(blocker{
			StageID:     stageWarehouseType,
			Status:      statusInvalid,
			NextAction:  unsupportedWarehouseAction(registry),
			Diagnostics: unsupportedWarehouseDiagnostics(config.Warehouse.Type),
		})), nil
	}
	eval.complete(stageWarehouseType)

	authName := warehouseAuthName(config, provider)
	hasCredential, err := credentialStore.HasCredential(config.Warehouse.Type, authName)
	if err != nil {
		return Result{}, err
	}
	if !hasCredential {
		return resultFor(envelope, eval.withBlocker(blocker{
			StageID:    stageWarehouseAuth,
			Status:     statusMissing,
			NextAction: authenticateWarehouseAction(provider),
		})), nil
	}
	eval.complete(stageWarehouseAuth)

	diagnostics := provider.ConfigDiagnostics(config.Warehouse)
	if len(diagnostics) > 0 {
		return resultFor(envelope, eval.withBlocker(blocker{
			StageID:     stageWarehouseConfig,
			Status:      statusInvalid,
			NextAction:  configureWarehouseAction(provider),
			Diagnostics: diagnostics,
		})), nil
	}
	eval.complete(stageWarehouseConfig)

	verified, err := credentialStore.HasMatchingAccessMarker(config.Warehouse.Type, authName, config.Warehouse)
	if err != nil {
		return Result{}, err
	}
	if !verified {
		return resultFor(envelope, eval.withBlocker(blocker{
			StageID:    stageWarehouseAccess,
			Status:     statusUntested,
			NextAction: testWarehouseAction(),
		})), nil
	}
	eval.complete(stageWarehouseAccess)

	if len(config.Sources) == 0 {
		return resultFor(envelope, eval.withBlocker(blocker{
			StageID:    stageSources,
			Status:     statusMissing,
			NextAction: selectSourceAction(),
			Diagnostics: []cliresult.Diagnostic{
				{
					ID:      "missing_sources",
					Field:   "sources",
					Message: "segmentstream.yml does not declare any sources.",
				},
			},
		})), nil
	}

	for _, source := range config.Sources {
		status, err := sourceVerifier.CheckSource(service.ProjectRoot, source)
		if err != nil {
			return Result{}, err
		}
		if !status.Valid {
			sourceName := source.Name
			reason := status.Reason
			if reason == "" {
				reason = "source has not passed verification"
			}
			return resultFor(envelope, eval.withBlocker(blocker{
				StageID:    stageSources,
				Status:     statusUntested,
				NextAction: verifySourceAction(sourceName),
				Diagnostics: []cliresult.Diagnostic{
					{
						ID:      "source_verification_required",
						Field:   sourceDiagnosticField(sourceName),
						Message: fmt.Sprintf("Source %q must pass verification: %s.", sourceName, reason),
					},
				},
			})), nil
		}
	}

	eval.complete(stageSources)

	eval.ready = true
	return resultFor(envelope, eval), nil
}

func baseEnvelope(config project.Config, registry warehouse.Registry) cliresult.Envelope {
	warehouseName := config.Warehouse.Type
	var warehouse *string
	if warehouseName != "" {
		warehouse = &warehouseName
	}

	authMethods := []string{}
	if warehouseName != "" {
		if provider, err := registry.Provider(warehouseName); err == nil {
			authMethods = provider.AuthMethods()
		}
	}
	if len(authMethods) == 0 {
		for _, provider := range registry.Providers() {
			authMethods = provider.AuthMethods()
			break
		}
	}

	return cliresult.Envelope{
		SchemaVersion: cliresult.SchemaVersion,
		Warehouse:     warehouse,
		Capabilities: cliresult.Capabilities{
			AuthMethods: authMethods,
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
	return Result{Envelope: envelope, ExitCode: cliresult.ExitReady}
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
	return statusSatisfied
}

func selectWarehouseAction(registry warehouse.Registry) cliresult.NextAction {
	var accepts []cliresult.NextActionAccept
	for _, provider := range registry.Providers() {
		accepts = append(accepts, provider.SelectWarehouseAccept())
	}
	return cliresult.NextAction{
		Type:    actionHumanInput,
		Stage:   string(stageWarehouseType),
		Reason:  "Select the warehouse SegmentStream should use.",
		Accepts: accepts,
		Verify:  initVerifyCommand,
	}
}

func unsupportedWarehouseAction(registry warehouse.Registry) cliresult.NextAction {
	return cliresult.NextAction{
		Type:    actionHumanInput,
		Stage:   string(stageWarehouseType),
		Reason:  "The configured warehouse is not available in this release.",
		Accepts: selectWarehouseAction(registry).Accepts,
		Verify:  initVerifyCommand,
	}
}

func authenticateWarehouseAction(provider warehouse.Provider) cliresult.NextAction {
	return cliresult.NextAction{
		Type:    actionHumanInput,
		Stage:   string(stageWarehouseAuth),
		Reason:  fmt.Sprintf("No %s credential is configured for the warehouse.", provider.DisplayName()),
		Accepts: provider.AuthenticateAccepts(),
		Verify:  initVerifyCommand,
	}
}

func configureWarehouseAction(provider warehouse.Provider) cliresult.NextAction {
	return cliresult.NextAction{
		Type:    actionHumanInput,
		Stage:   string(stageWarehouseConfig),
		Reason:  "Warehouse project, dataset, or location is not configured.",
		Accepts: []cliresult.NextActionAccept{provider.ConfigureAccept()},
		Verify:  initVerifyCommand,
	}
}

func testWarehouseAction() cliresult.NextAction {
	return cliresult.NextAction{
		Type:    actionRunCommand,
		Stage:   string(stageWarehouseAccess),
		Command: testWarehouseCommand,
		Reason:  "Warehouse access has not been verified for this project, dataset, and location.",
	}
}

func selectSourceAction() cliresult.NextAction {
	return cliresult.NextAction{
		Type:    actionRunCommand,
		Stage:   string(stageSources),
		Command: sourceContractsCommand,
		Reason:  "No sources are configured. Inspect supported source contracts, then ask the user which source to implement.",
	}
}

func verifySourceAction(sourceName string) cliresult.NextAction {
	command := sourceContractsCommand
	reason := "A declared source has not passed verification."
	if sourceName != "" {
		command = fmt.Sprintf("segmentstream source verify %s", sourceName)
		reason = fmt.Sprintf("Source %q has not passed verification.", sourceName)
	}
	return cliresult.NextAction{
		Type:    actionRunCommand,
		Stage:   string(stageSources),
		Command: command,
		Reason:  reason,
		Verify:  initVerifyCommand,
	}
}

func doneAction() cliresult.NextAction {
	return cliresult.NextAction{
		Type:    actionRunCommand,
		Stage:   "ready",
		Command: runCommand,
		Reason:  "SegmentStream project is ready.",
	}
}

func sourceDiagnosticField(sourceName string) string {
	if sourceName == "" {
		return "sources"
	}
	return "sources." + sourceName
}

func unsupportedWarehouseDiagnostics(warehouseType string) []cliresult.Diagnostic {
	return []cliresult.Diagnostic{{
		ID:      "unsupported_warehouse",
		Field:   "warehouse.type",
		Message: fmt.Sprintf("Unsupported warehouse.type %q.", warehouseType),
	}}
}

func warehouseAuthName(config project.Config, provider warehouse.Provider) string {
	if config.Warehouse.Auth != "" {
		return config.Warehouse.Auth
	}
	return provider.DefaultAuthName()
}
