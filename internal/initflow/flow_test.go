package initflow

import (
	"context"
	"errors"
	"testing"

	"github.com/segmentstream/segmentstream-cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/internal/project"
)

func TestEvaluateAsksForWarehouseWithoutMutating(t *testing.T) {
	projectStore := &fakeProjectStore{}
	scaffolder := &fakeScaffolder{}

	result, err := (Service{
		ProjectStore: projectStore,
		Credentials:  &fakeCredentialStore{},
		Scaffolder:   scaffolder,
	}).Evaluate(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result.ExitCode != cliresult.ExitNeedsUserDecision {
		t.Fatalf("exit code = %d, want %d", result.ExitCode, cliresult.ExitNeedsUserDecision)
	}
	if result.Envelope.NextAction.Type != "ask_user" {
		t.Fatalf("next action = %+v, want ask_user", result.Envelope.NextAction)
	}
	if projectStore.selectedWarehouse != "" {
		t.Fatalf("selected warehouse = %q, want no mutation", projectStore.selectedWarehouse)
	}
	if scaffolder.called {
		t.Fatal("scaffolder was called for read-only evaluation")
	}
}

func TestEvaluateSelectsWarehouseThenNeedsAuth(t *testing.T) {
	projectStore := &fakeProjectStore{}
	scaffolder := &fakeScaffolder{}

	result, err := (Service{
		ProjectStore: projectStore,
		Credentials:  &fakeCredentialStore{},
		Scaffolder:   scaffolder,
	}).Evaluate(context.Background(), Options{SelectWarehouse: "bigquery"})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result.ExitCode != cliresult.ExitNeedsAuth {
		t.Fatalf("exit code = %d, want %d", result.ExitCode, cliresult.ExitNeedsAuth)
	}
	if projectStore.selectedWarehouse != "bigquery" {
		t.Fatalf("selected warehouse = %q, want bigquery", projectStore.selectedWarehouse)
	}
	if !scaffolder.called {
		t.Fatal("scaffolder was not called after selecting warehouse")
	}
	config := projectStore.config
	if config.Warehouse.Type != "bigquery" || config.Warehouse.Auth != "default-bigquery" {
		t.Fatalf("warehouse = %+v, want selected bigquery", config.Warehouse)
	}
	if config.Warehouse.Project != "" {
		t.Fatalf("warehouse project = %q, want no placeholder", config.Warehouse.Project)
	}
}

func TestEvaluateNeedsConfigurationAfterAuth(t *testing.T) {
	projectStore := &fakeProjectStore{
		exists: true,
		config: project.Config{
			Version: project.SupportedConfigVersion,
			Warehouse: project.Warehouse{
				Type: "bigquery",
				Auth: "default-bigquery",
			},
		},
	}
	credentialStore := &fakeCredentialStore{hasBigQueryCredential: true}

	result, err := (Service{
		ProjectStore: projectStore,
		Credentials:  credentialStore,
		Scaffolder:   &fakeScaffolder{},
	}).Evaluate(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result.ExitCode != cliresult.ExitMisconfigured {
		t.Fatalf("exit code = %d, want %d", result.ExitCode, cliresult.ExitMisconfigured)
	}
	if result.Envelope.NextAction.Command != "segmentstream warehouse configure --project <project> --dataset <dataset> --location <location>" {
		t.Fatalf("next action = %+v", result.Envelope.NextAction)
	}
	if len(result.Envelope.NextAction.Hints) != 1 {
		t.Fatalf("hints = %+v, want one browse hint", result.Envelope.NextAction.Hints)
	}
	hint := result.Envelope.NextAction.Hints[0]
	if hint.ID != "browse_warehouse_before_configure" {
		t.Fatalf("hint id = %q, want browse_warehouse_before_configure", hint.ID)
	}
	if len(hint.Commands) != 2 ||
		hint.Commands[0] != "segmentstream warehouse browse --json" ||
		hint.Commands[1] != "segmentstream warehouse browse --path <project> --json" {
		t.Fatalf("hint commands = %+v, want browse commands", hint.Commands)
	}
}

func TestEvaluateRejectsUnsupportedWarehouse(t *testing.T) {
	result, err := (Service{
		ProjectStore: &fakeProjectStore{
			exists: true,
			config: project.Config{
				Version: project.SupportedConfigVersion,
				Warehouse: project.Warehouse{
					Type: "snowflake",
				},
			},
		},
		Credentials: &fakeCredentialStore{},
		Scaffolder:  &fakeScaffolder{},
	}).Evaluate(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result.ExitCode != cliresult.ExitNeedsUserDecision {
		t.Fatalf("exit code = %d, want %d", result.ExitCode, cliresult.ExitNeedsUserDecision)
	}
	assertStage(t, result.Envelope.Stages, 1, stageWarehouseType, statusInvalid, true)
	if len(result.Envelope.Diagnostics) != 1 || result.Envelope.Diagnostics[0].ID != "unsupported_warehouse" {
		t.Fatalf("diagnostics = %+v, want unsupported_warehouse", result.Envelope.Diagnostics)
	}
	if result.Envelope.NextAction.Type != "ask_user" {
		t.Fatalf("next action = %+v, want ask_user", result.Envelope.NextAction)
	}
}

func TestEvaluateReportsMissingAuthEvenWhenDefaultCredentialExists(t *testing.T) {
	result, err := (Service{
		ProjectStore: &fakeProjectStore{
			exists: true,
			config: project.Config{
				Version: project.SupportedConfigVersion,
				Warehouse: project.Warehouse{
					Type:     "bigquery",
					Project:  "example-project",
					Dataset:  "segmentstream",
					Location: "EU",
				},
			},
		},
		Credentials: &fakeCredentialStore{hasBigQueryCredential: true},
		Scaffolder:  &fakeScaffolder{},
	}).Evaluate(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result.ExitCode != cliresult.ExitMisconfigured {
		t.Fatalf("exit code = %d, want %d", result.ExitCode, cliresult.ExitMisconfigured)
	}
	if len(result.Envelope.Diagnostics) != 1 || result.Envelope.Diagnostics[0].ID != "missing_auth" {
		t.Fatalf("diagnostics = %+v, want missing_auth", result.Envelope.Diagnostics)
	}
}

func TestEvaluateNeedsAccessTestAfterConfiguration(t *testing.T) {
	result, err := (Service{
		ProjectStore: configuredProjectStore(),
		Credentials:  &fakeCredentialStore{hasBigQueryCredential: true},
		Scaffolder:   &fakeScaffolder{},
	}).Evaluate(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result.ExitCode != cliresult.ExitMisconfigured {
		t.Fatalf("exit code = %d, want %d", result.ExitCode, cliresult.ExitMisconfigured)
	}
	if result.Envelope.NextAction.Command != "segmentstream warehouse test" {
		t.Fatalf("next action = %+v, want warehouse test", result.Envelope.NextAction)
	}
	assertStage(t, result.Envelope.Stages, 4, stageWarehouseAccess, statusUntested, true)
}

func TestEvaluateReadyAfterAccessMarker(t *testing.T) {
	result, err := (Service{
		ProjectStore: configuredProjectStore(),
		Credentials: &fakeCredentialStore{
			hasBigQueryCredential: true,
			hasAccessMarker:       true,
		},
		Scaffolder: &fakeScaffolder{},
	}).Evaluate(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result.ExitCode != cliresult.ExitReady || !result.Envelope.Ready {
		t.Fatalf("result = %+v, want ready", result)
	}
	if result.Envelope.NextAction.Type != "done" {
		t.Fatalf("next action = %+v, want done", result.Envelope.NextAction)
	}
	for i, stage := range result.Envelope.Stages {
		if stage.Current {
			t.Fatalf("stage[%d] = %+v, want no current stage when ready", i, stage)
		}
		wantStatus := statusSatisfied
		if stage.ID == string(stagePrerequisites) {
			wantStatus = statusSatisfiedWithWarnings
		}
		if stage.Status != wantStatus {
			t.Fatalf("stage[%d] = %+v, want status %q", i, stage, wantStatus)
		}
	}
}

func TestEvaluatePropagatesProjectLoadError(t *testing.T) {
	loadErr := errors.New("load failed")

	_, err := (Service{
		ProjectStore: &fakeProjectStore{loadErr: loadErr},
		Credentials:  &fakeCredentialStore{},
		Scaffolder:   &fakeScaffolder{},
	}).Evaluate(context.Background(), Options{})

	if !errors.Is(err, loadErr) {
		t.Fatalf("error = %v, want load error", err)
	}
}

func TestEvaluatePropagatesCredentialCheckErrors(t *testing.T) {
	credentialErr := errors.New("credential check failed")

	_, err := (Service{
		ProjectStore: configuredProjectStore(),
		Credentials:  &fakeCredentialStore{hasBigQueryCredentialErr: credentialErr},
		Scaffolder:   &fakeScaffolder{},
	}).Evaluate(context.Background(), Options{})
	if !errors.Is(err, credentialErr) {
		t.Fatalf("error = %v, want credential check error", err)
	}

	markerErr := errors.New("access marker check failed")
	_, err = (Service{
		ProjectStore: configuredProjectStore(),
		Credentials: &fakeCredentialStore{
			hasBigQueryCredential: true,
			hasAccessMarkerErr:    markerErr,
		},
		Scaffolder: &fakeScaffolder{},
	}).Evaluate(context.Background(), Options{})
	if !errors.Is(err, markerErr) {
		t.Fatalf("error = %v, want access marker check error", err)
	}
}

func TestEvaluateDoesNotScaffoldWhenWarehouseSelectionFails(t *testing.T) {
	selectErr := errors.New("select failed")
	scaffolder := &fakeScaffolder{}

	_, err := (Service{
		ProjectStore: &fakeProjectStore{selectErr: selectErr},
		Credentials:  &fakeCredentialStore{},
		Scaffolder:   scaffolder,
	}).Evaluate(context.Background(), Options{SelectWarehouse: "bigquery"})

	if !errors.Is(err, selectErr) {
		t.Fatalf("error = %v, want select error", err)
	}
	if scaffolder.called {
		t.Fatal("scaffolder was called after select warehouse failed")
	}
}

func TestEvaluatePropagatesScaffolderError(t *testing.T) {
	scaffoldErr := errors.New("scaffold failed")

	_, err := (Service{
		ProjectStore: &fakeProjectStore{},
		Credentials:  &fakeCredentialStore{},
		Scaffolder:   &fakeScaffolder{err: scaffoldErr},
	}).Evaluate(context.Background(), Options{SelectWarehouse: "bigquery"})

	if !errors.Is(err, scaffoldErr) {
		t.Fatalf("error = %v, want scaffold error", err)
	}
}

func TestBuildStagesProjectsBlockerOntoStagePlan(t *testing.T) {
	stageBlocker := blocker{
		StageID: stageWarehouseConfig,
		Status:  statusInvalid,
	}

	stages := buildStages(stagePlan, map[stageID]bool{
		stagePrerequisites: true,
		stageWarehouseType: true,
		stageWarehouseAuth: true,
	}, &stageBlocker)

	assertStage(t, stages, 0, stagePrerequisites, statusSatisfiedWithWarnings, false)
	assertStage(t, stages, 1, stageWarehouseType, statusSatisfied, false)
	assertStage(t, stages, 2, stageWarehouseAuth, statusSatisfied, false)
	assertStage(t, stages, 3, stageWarehouseConfig, statusInvalid, true)
	assertStage(t, stages, 4, stageWarehouseAccess, statusPending, false)
}

func TestBuildStagesRequiresCompletedDependencies(t *testing.T) {
	stages := buildStages([]stageSpec{
		{ID: stageWarehouseType, DependsOn: []stageID{stagePrerequisites}},
	}, map[stageID]bool{
		stageWarehouseType: true,
	}, nil)

	assertStage(t, stages, 0, stageWarehouseType, statusPending, false)
}

func assertStage(t *testing.T, stages []cliresult.Stage, index int, id stageID, status string, current bool) {
	t.Helper()
	if index >= len(stages) {
		t.Fatalf("stage[%d] missing from %+v", index, stages)
	}
	stage := stages[index]
	if stage.ID != string(id) || stage.Status != status || stage.Current != current {
		t.Fatalf("stage[%d] = %+v, want id %q status %q current %v", index, stage, id, status, current)
	}
}

func configuredProjectStore() *fakeProjectStore {
	return &fakeProjectStore{
		exists: true,
		config: project.Config{
			Version: project.SupportedConfigVersion,
			Warehouse: project.Warehouse{
				Type:     "bigquery",
				Auth:     "default-bigquery",
				Project:  "example-project",
				Dataset:  "segmentstream",
				Location: "EU",
			},
		},
	}
}

type fakeProjectStore struct {
	config            project.Config
	exists            bool
	selectedWarehouse string
	loadErr           error
	selectErr         error
}

func (store *fakeProjectStore) LoadPartial() (project.Config, bool, error) {
	if store.loadErr != nil {
		return project.Config{}, false, store.loadErr
	}
	return store.config, store.exists, nil
}

func (store *fakeProjectStore) SelectWarehouse(warehouseType string) (project.Config, error) {
	if store.selectErr != nil {
		return project.Config{}, store.selectErr
	}
	store.selectedWarehouse = warehouseType
	store.exists = true
	if store.config.Version == 0 {
		store.config.Version = project.SupportedConfigVersion
	}
	if store.config.Warehouse.Type == "" {
		store.config.Warehouse.Type = warehouseType
	}
	if store.config.Warehouse.Auth == "" {
		store.config.Warehouse.Auth = defaultBigQueryAuth
	}
	return store.config, nil
}

type fakeCredentialStore struct {
	hasBigQueryCredential    bool
	hasBigQueryCredentialErr error
	hasAccessMarker          bool
	hasAccessMarkerErr       error
}

func (store *fakeCredentialStore) HasBigQueryCredential(name string) (bool, error) {
	if store.hasBigQueryCredentialErr != nil {
		return false, store.hasBigQueryCredentialErr
	}
	return store.hasBigQueryCredential, nil
}

func (store *fakeCredentialStore) HasMatchingAccessMarker(name, projectID, dataset, location string) (bool, error) {
	if store.hasAccessMarkerErr != nil {
		return false, store.hasAccessMarkerErr
	}
	return store.hasAccessMarker, nil
}

type fakeScaffolder struct {
	called bool
	err    error
}

func (scaffolder *fakeScaffolder) EnsureInitFiles() error {
	scaffolder.called = true
	return scaffolder.err
}
