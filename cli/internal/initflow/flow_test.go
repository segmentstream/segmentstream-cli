package initflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/segmentstream/segmentstream-cli/cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/cli/internal/project"
	sourcepkg "github.com/segmentstream/segmentstream-cli/cli/internal/source"
	"github.com/segmentstream/segmentstream-cli/cli/internal/warehouse"
	"github.com/segmentstream/segmentstream-cli/cli/internal/warehouse/bigquery"
)

func testRegistry() warehouse.Registry {
	return warehouse.NewRegistry(bigquery.NewConnector())
}

func TestEvaluateAsksForWarehouseWithoutMutating(t *testing.T) {
	projectStore := &fakeProjectStore{}
	scaffolder := &fakeScaffolder{}

	result, err := (Service{
		WarehouseRegistry: testRegistry(),
		ProjectStore:      projectStore,
		CredentialStore:   &fakeCredentialStore{},
		Scaffolder:        scaffolder,
	}).Evaluate(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	assertInitEnvelopeV2(t, result.Envelope)
	if result.ExitCode != cliresult.ExitReady {
		t.Fatalf("exit code = %d, want %d", result.ExitCode, cliresult.ExitReady)
	}
	assertWarehouseTypeAction(t, result.Envelope.NextAction)
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
		WarehouseRegistry: testRegistry(),
		ProjectStore:      projectStore,
		CredentialStore:   &fakeCredentialStore{},
		Scaffolder:        scaffolder,
	}).Evaluate(context.Background(), Options{SelectWarehouse: "bigquery"})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	assertInitEnvelopeV2(t, result.Envelope)
	if result.ExitCode != cliresult.ExitReady {
		t.Fatalf("exit code = %d, want %d", result.ExitCode, cliresult.ExitReady)
	}
	assertWarehouseAuthAction(t, result.Envelope.NextAction)
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
		WarehouseRegistry: testRegistry(),
		ProjectStore:      projectStore,
		CredentialStore:   credentialStore,
		Scaffolder:        &fakeScaffolder{},
	}).Evaluate(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	assertInitEnvelopeV2(t, result.Envelope)
	if result.ExitCode != cliresult.ExitReady {
		t.Fatalf("exit code = %d, want %d", result.ExitCode, cliresult.ExitReady)
	}
	assertWarehouseConfigAction(t, result.Envelope.NextAction)
}

func TestEvaluateRejectsUnsupportedWarehouse(t *testing.T) {
	result, err := (Service{
		WarehouseRegistry: testRegistry(),
		ProjectStore: &fakeProjectStore{
			exists: true,
			config: project.Config{
				Version: project.SupportedConfigVersion,
				Warehouse: project.Warehouse{
					Type: "snowflake",
				},
			},
		},
		CredentialStore: &fakeCredentialStore{},
		Scaffolder:      &fakeScaffolder{},
	}).Evaluate(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	assertInitEnvelopeV2(t, result.Envelope)
	if result.ExitCode != cliresult.ExitReady {
		t.Fatalf("exit code = %d, want %d", result.ExitCode, cliresult.ExitReady)
	}
	assertStage(t, result.Envelope.Stages, 1, stageWarehouseType, statusInvalid, true)
	if len(result.Envelope.Diagnostics) != 1 || result.Envelope.Diagnostics[0].ID != "unsupported_warehouse" {
		t.Fatalf("diagnostics = %+v, want unsupported_warehouse", result.Envelope.Diagnostics)
	}
	assertWarehouseTypeAction(t, result.Envelope.NextAction)
}

func TestEvaluateReportsMissingAuthEvenWhenDefaultCredentialExists(t *testing.T) {
	result, err := (Service{
		WarehouseRegistry: testRegistry(),
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
		CredentialStore: &fakeCredentialStore{hasBigQueryCredential: true},
		Scaffolder:      &fakeScaffolder{},
	}).Evaluate(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	assertInitEnvelopeV2(t, result.Envelope)
	if result.ExitCode != cliresult.ExitReady {
		t.Fatalf("exit code = %d, want %d", result.ExitCode, cliresult.ExitReady)
	}
	if len(result.Envelope.Diagnostics) != 1 || result.Envelope.Diagnostics[0].ID != "missing_auth" {
		t.Fatalf("diagnostics = %+v, want missing_auth", result.Envelope.Diagnostics)
	}
}

func TestEvaluateNeedsAccessTestAfterConfiguration(t *testing.T) {
	result, err := (Service{
		WarehouseRegistry: testRegistry(),
		ProjectStore:      configuredProjectStore(),
		CredentialStore:   &fakeCredentialStore{hasBigQueryCredential: true},
		Scaffolder:        &fakeScaffolder{},
	}).Evaluate(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	assertInitEnvelopeV2(t, result.Envelope)
	if result.ExitCode != cliresult.ExitReady {
		t.Fatalf("exit code = %d, want %d", result.ExitCode, cliresult.ExitReady)
	}
	if result.Envelope.NextAction.Type != actionRunCommand ||
		result.Envelope.NextAction.Stage != string(stageWarehouseAccess) ||
		result.Envelope.NextAction.Command != "segmentstream warehouse test --json" {
		t.Fatalf("next action = %+v, want warehouse access run_command", result.Envelope.NextAction)
	}
	assertNoPlaceholderRunCommand(t, result.Envelope.NextAction)
	assertStage(t, result.Envelope.Stages, 4, stageWarehouseAccess, statusUntested, true)
}

func TestEvaluateNeedsSourcesAfterAccessMarker(t *testing.T) {
	result, err := (Service{
		WarehouseRegistry: testRegistry(),
		ProjectStore:      configuredProjectStore(),
		CredentialStore: &fakeCredentialStore{
			hasBigQueryCredential: true,
			hasAccessMarker:       true,
		},
		Scaffolder: &fakeScaffolder{},
	}).Evaluate(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	assertInitEnvelopeV2(t, result.Envelope)
	if result.ExitCode != cliresult.ExitReady || result.Envelope.Ready {
		t.Fatalf("result = %+v, want not ready without sources", result)
	}
	if len(result.Envelope.Diagnostics) != 1 || result.Envelope.Diagnostics[0].ID != "missing_sources" {
		t.Fatalf("diagnostics = %+v, want missing_sources", result.Envelope.Diagnostics)
	}
	if result.Envelope.NextAction.Type != actionRunCommand ||
		result.Envelope.NextAction.Stage != string(stageSources) ||
		result.Envelope.NextAction.Command != "segmentstream source contracts" {
		t.Fatalf("next action = %+v, want source contracts command", result.Envelope.NextAction)
	}
	assertStage(t, result.Envelope.Stages, 5, stageSources, statusMissing, true)
	assertStage(t, result.Envelope.Stages, 6, stageIdentity, statusPending, false)
}

func TestEvaluateNeedsIdentitySourceAfterEventSource(t *testing.T) {
	result, err := (Service{
		WarehouseRegistry: testRegistry(),
		ProjectStore:      configuredProjectStoreWithSource(),
		CredentialStore: &fakeCredentialStore{
			hasBigQueryCredential: true,
			hasAccessMarker:       true,
		},
		Scaffolder:     &fakeScaffolder{},
		SourceVerifier: &fakeSourceVerifier{valid: true},
	}).Evaluate(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	assertInitEnvelopeV2(t, result.Envelope)
	if result.ExitCode != cliresult.ExitReady || result.Envelope.Ready {
		t.Fatalf("result = %+v, want not ready without identity_keys source", result)
	}
	if result.Envelope.NextAction.Type != actionRunCommand ||
		result.Envelope.NextAction.Stage != string(stageSources) ||
		result.Envelope.NextAction.Command != "segmentstream source contracts --type identity_keys" {
		t.Fatalf("next action = %+v, want identity_keys source contract command", result.Envelope.NextAction)
	}
	if len(result.Envelope.Diagnostics) != 1 || result.Envelope.Diagnostics[0].ID != "missing_identity_keys_source" {
		t.Fatalf("diagnostics = %+v, want missing_identity_keys_source", result.Envelope.Diagnostics)
	}
	assertStage(t, result.Envelope.Stages, 5, stageSources, statusMissing, true)
	assertStage(t, result.Envelope.Stages, 6, stageIdentity, statusPending, false)
}

func TestEvaluateNeedsEventSourceAfterIdentitySource(t *testing.T) {
	result, err := (Service{
		WarehouseRegistry: testRegistry(),
		ProjectStore:      configuredProjectStoreWithIdentitySource(),
		CredentialStore: &fakeCredentialStore{
			hasBigQueryCredential: true,
			hasAccessMarker:       true,
		},
		Scaffolder: &fakeScaffolder{},
		SourceVerifier: &fakeSourceVerifier{
			valid: true,
			contracts: map[string]string{
				"sdk_identity": "identity_keys",
			},
		},
	}).Evaluate(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	assertInitEnvelopeV2(t, result.Envelope)
	if result.ExitCode != cliresult.ExitReady || result.Envelope.Ready {
		t.Fatalf("result = %+v, want not ready without events source", result)
	}
	if result.Envelope.NextAction.Type != actionRunCommand ||
		result.Envelope.NextAction.Stage != string(stageSources) ||
		result.Envelope.NextAction.Command != "segmentstream source contracts --type events" {
		t.Fatalf("next action = %+v, want events source contract command", result.Envelope.NextAction)
	}
	if len(result.Envelope.Diagnostics) != 1 || result.Envelope.Diagnostics[0].ID != "missing_events_source" {
		t.Fatalf("diagnostics = %+v, want missing_events_source", result.Envelope.Diagnostics)
	}
}

func TestEvaluateNeedsIdentityConfigAfterRequiredSources(t *testing.T) {
	result, err := (Service{
		WarehouseRegistry: testRegistry(),
		ProjectStore:      configuredProjectStoreWithRequiredSources(),
		CredentialStore: &fakeCredentialStore{
			hasBigQueryCredential: true,
			hasAccessMarker:       true,
		},
		Scaffolder:     &fakeScaffolder{},
		SourceVerifier: fakeRequiredSourceVerifier(),
	}).Evaluate(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	assertInitEnvelopeV2(t, result.Envelope)
	if result.ExitCode != cliresult.ExitReady || result.Envelope.Ready {
		t.Fatalf("result = %+v, want not ready without identity.keys", result)
	}
	if result.Envelope.NextAction.Type != actionHumanInput ||
		result.Envelope.NextAction.Stage != string(stageIdentity) ||
		result.Envelope.NextAction.Verify != "segmentstream init --json" {
		t.Fatalf("next action = %+v, want identity human_input action", result.Envelope.NextAction)
	}
	if len(result.Envelope.Diagnostics) != 1 ||
		result.Envelope.Diagnostics[0].ID != "missing_identity_keys" ||
		result.Envelope.Diagnostics[0].Suggestion == "" {
		t.Fatalf("diagnostics = %+v, want missing_identity_keys with suggestion", result.Envelope.Diagnostics)
	}
	assertStage(t, result.Envelope.Stages, 5, stageSources, statusSatisfied, false)
	assertStage(t, result.Envelope.Stages, 6, stageIdentity, statusMissing, true)
}

func TestEvaluateNeedsConversionsSourceAfterEventAndIdentitySources(t *testing.T) {
	result, err := (Service{
		WarehouseRegistry: testRegistry(),
		ProjectStore:      configuredProjectStoreWithEventAndIdentitySources(),
		CredentialStore: &fakeCredentialStore{
			hasBigQueryCredential: true,
			hasAccessMarker:       true,
		},
		Scaffolder: &fakeScaffolder{},
		SourceVerifier: &fakeSourceVerifier{
			valid: true,
			contracts: map[string]string{
				"ga4":          "events",
				"sdk_identity": "identity_keys",
			},
		},
	}).Evaluate(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	assertInitEnvelopeV2(t, result.Envelope)
	if result.ExitCode != cliresult.ExitReady || result.Envelope.Ready {
		t.Fatalf("result = %+v, want not ready without conversion_events source", result)
	}
	if result.Envelope.NextAction.Type != actionRunCommand ||
		result.Envelope.NextAction.Stage != string(stageSources) ||
		result.Envelope.NextAction.Command != "segmentstream source contracts --type conversion_events" {
		t.Fatalf("next action = %+v, want conversion_events source contract command", result.Envelope.NextAction)
	}
	if len(result.Envelope.Diagnostics) != 1 || result.Envelope.Diagnostics[0].ID != "missing_conversion_events_source" {
		t.Fatalf("diagnostics = %+v, want missing_conversion_events_source", result.Envelope.Diagnostics)
	}
}

func TestEvaluateReadyAfterAccessMarkerSourcesAndIdentityConfig(t *testing.T) {
	result, err := (Service{
		WarehouseRegistry: testRegistry(),
		ProjectStore:      configuredProjectStoreWithRequiredSourcesAndIdentity(),
		CredentialStore: &fakeCredentialStore{
			hasBigQueryCredential: true,
			hasAccessMarker:       true,
		},
		Scaffolder:     &fakeScaffolder{},
		SourceVerifier: fakeRequiredSourceVerifier(),
	}).Evaluate(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	assertInitEnvelopeV2(t, result.Envelope)
	if result.ExitCode != cliresult.ExitReady || !result.Envelope.Ready {
		t.Fatalf("result = %+v, want ready", result)
	}
	if result.Envelope.NextAction.Type != actionRunCommand ||
		result.Envelope.NextAction.Stage != "ready" ||
		result.Envelope.NextAction.Command != "segmentstream run" {
		t.Fatalf("next action = %+v, want ready run_command", result.Envelope.NextAction)
	}
	assertNoPlaceholderRunCommand(t, result.Envelope.NextAction)
	for i, stage := range result.Envelope.Stages {
		if stage.Current {
			t.Fatalf("stage[%d] = %+v, want no current stage when ready", i, stage)
		}
		wantStatus := statusSatisfied
		if stage.Status != wantStatus {
			t.Fatalf("stage[%d] = %+v, want status %q", i, stage, wantStatus)
		}
	}
}

func TestEvaluateNeedsSourceVerificationAfterSourceIsDeclared(t *testing.T) {
	result, err := (Service{
		WarehouseRegistry: testRegistry(),
		ProjectStore:      configuredProjectStoreWithSource(),
		CredentialStore: &fakeCredentialStore{
			hasBigQueryCredential: true,
			hasAccessMarker:       true,
		},
		Scaffolder:     &fakeScaffolder{},
		SourceVerifier: &fakeSourceVerifier{valid: false, reason: "source files changed since verification"},
	}).Evaluate(context.Background(), Options{})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	assertInitEnvelopeV2(t, result.Envelope)
	if result.ExitCode != cliresult.ExitReady || result.Envelope.Ready {
		t.Fatalf("result = %+v, want not ready without source verification", result)
	}
	if len(result.Envelope.Diagnostics) != 1 ||
		result.Envelope.Diagnostics[0].ID != "source_verification_required" ||
		!strings.Contains(result.Envelope.Diagnostics[0].Message, "source files changed") {
		t.Fatalf("diagnostics = %+v, want source verification diagnostic", result.Envelope.Diagnostics)
	}
	if result.Envelope.NextAction.Type != actionRunCommand ||
		result.Envelope.NextAction.Stage != string(stageSources) ||
		result.Envelope.NextAction.Command != "segmentstream source verify ga4" ||
		result.Envelope.NextAction.Verify != "segmentstream init --json" {
		t.Fatalf("next action = %+v, want source verify command", result.Envelope.NextAction)
	}
	assertStage(t, result.Envelope.Stages, 5, stageSources, statusUntested, true)
}

func TestEvaluatePropagatesProjectLoadError(t *testing.T) {
	loadErr := errors.New("load failed")

	_, err := (Service{
		WarehouseRegistry: testRegistry(),
		ProjectStore:      &fakeProjectStore{loadErr: loadErr},
		CredentialStore:   &fakeCredentialStore{},
		Scaffolder:        &fakeScaffolder{},
	}).Evaluate(context.Background(), Options{})

	if !errors.Is(err, loadErr) {
		t.Fatalf("error = %v, want load error", err)
	}
}

func TestEvaluatePropagatesCredentialCheckErrors(t *testing.T) {
	credentialErr := errors.New("credential check failed")

	_, err := (Service{
		WarehouseRegistry: testRegistry(),
		ProjectStore:      configuredProjectStore(),
		CredentialStore:   &fakeCredentialStore{hasBigQueryCredentialErr: credentialErr},
		Scaffolder:        &fakeScaffolder{},
	}).Evaluate(context.Background(), Options{})
	if !errors.Is(err, credentialErr) {
		t.Fatalf("error = %v, want credential check error", err)
	}

	markerErr := errors.New("access marker check failed")
	_, err = (Service{
		WarehouseRegistry: testRegistry(),
		ProjectStore:      configuredProjectStore(),
		CredentialStore: &fakeCredentialStore{
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
		WarehouseRegistry: testRegistry(),
		ProjectStore:      &fakeProjectStore{selectErr: selectErr},
		CredentialStore:   &fakeCredentialStore{},
		Scaffolder:        scaffolder,
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
		WarehouseRegistry: testRegistry(),
		ProjectStore:      &fakeProjectStore{},
		CredentialStore:   &fakeCredentialStore{},
		Scaffolder:        &fakeScaffolder{err: scaffoldErr},
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

	assertStage(t, stages, 0, stagePrerequisites, statusSatisfied, false)
	assertStage(t, stages, 1, stageWarehouseType, statusSatisfied, false)
	assertStage(t, stages, 2, stageWarehouseAuth, statusSatisfied, false)
	assertStage(t, stages, 3, stageWarehouseConfig, statusInvalid, true)
	assertStage(t, stages, 4, stageWarehouseAccess, statusPending, false)
	assertStage(t, stages, 5, stageSources, statusPending, false)
	assertStage(t, stages, 6, stageIdentity, statusPending, false)
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

func assertInitEnvelopeV2(t *testing.T, envelope cliresult.Envelope) {
	t.Helper()
	if envelope.SchemaVersion != cliresult.SchemaVersion {
		t.Fatalf("schema version = %q, want %q", envelope.SchemaVersion, cliresult.SchemaVersion)
	}
	if strings.Join(envelope.Capabilities.AuthMethods, ",") != "oauth,service_account_key" {
		t.Fatalf("auth methods = %+v, want oauth and service_account_key", envelope.Capabilities.AuthMethods)
	}
	if envelope.NextAction.Type != actionHumanInput && envelope.NextAction.Type != actionRunCommand {
		t.Fatalf("next action type = %q, want human_input or run_command", envelope.NextAction.Type)
	}
	assertNoPlaceholderRunCommand(t, envelope.NextAction)
}

func assertWarehouseTypeAction(t *testing.T, action cliresult.NextAction) {
	t.Helper()
	if action.Type != actionHumanInput || action.Stage != string(stageWarehouseType) || action.Verify != "segmentstream init --json" {
		t.Fatalf("next action = %+v, want warehouse_type human_input", action)
	}
	if len(action.Accepts) != 1 {
		t.Fatalf("accepts = %+v, want one option", action.Accepts)
	}
	accept := action.Accepts[0]
	if accept.Method != "bigquery" ||
		accept.Label == "" ||
		accept.Command != "segmentstream init --warehouse bigquery" ||
		accept.Value != "bigquery" ||
		len(accept.Inputs) != 0 {
		t.Fatalf("accept = %+v, want bigquery warehouse selection", accept)
	}
}

func assertWarehouseAuthAction(t *testing.T, action cliresult.NextAction) {
	t.Helper()
	if action.Type != actionHumanInput || action.Stage != string(stageWarehouseAuth) || action.Verify != "segmentstream init --json" {
		t.Fatalf("next action = %+v, want warehouse_auth human_input", action)
	}
	if len(action.Accepts) != 2 {
		t.Fatalf("accepts = %+v, want oauth and service-account auth methods", action.Accepts)
	}
	oauth := action.Accepts[0]
	if oauth.Method != "oauth" || oauth.Command != "segmentstream warehouse auth login" || len(oauth.Inputs) != 1 {
		t.Fatalf("accept = %+v, want OAuth login auth", oauth)
	}
	oauthInput := oauth.Inputs[0]
	if oauthInput.Name != "port" ||
		oauthInput.Type != "integer" ||
		oauthInput.Flag != "--port" ||
		oauthInput.Label == "" ||
		oauthInput.Required {
		t.Fatalf("input = %+v, want optional OAuth callback port", oauthInput)
	}

	serviceAccount := action.Accepts[1]
	if serviceAccount.Method != "service_account_key" || serviceAccount.Command != "segmentstream warehouse auth" || len(serviceAccount.Inputs) != 1 {
		t.Fatalf("accept = %+v, want service-account key auth", serviceAccount)
	}
	input := serviceAccount.Inputs[0]
	if input.Name != "path" ||
		input.Type != "filepath" ||
		input.Flag != "--service-account-key" ||
		input.Label == "" ||
		!input.Required {
		t.Fatalf("input = %+v, want required service-account key path", input)
	}
}

func assertWarehouseConfigAction(t *testing.T, action cliresult.NextAction) {
	t.Helper()
	if action.Type != actionHumanInput || action.Stage != string(stageWarehouseConfig) || action.Verify != "segmentstream init --json" {
		t.Fatalf("next action = %+v, want warehouse_config human_input", action)
	}
	if len(action.Accepts) != 1 {
		t.Fatalf("accepts = %+v, want one config option", action.Accepts)
	}
	accept := action.Accepts[0]
	if accept.Method != "warehouse_config" || accept.Command != "segmentstream warehouse configure" || len(accept.Inputs) != 4 {
		t.Fatalf("accept = %+v, want warehouse configure inputs", accept)
	}
	want := []struct {
		name      string
		inputType string
		flag      string
		required  bool
	}{
		{name: "project", inputType: "string", flag: "--project", required: true},
		{name: "dataset", inputType: "string", flag: "--dataset", required: true},
		{name: "location", inputType: "string", flag: "--location", required: true},
		{name: "create_dataset", inputType: "boolean", flag: "--create-dataset", required: false},
	}
	for i, wantInput := range want {
		input := accept.Inputs[i]
		if input.Name != wantInput.name ||
			input.Type != wantInput.inputType ||
			input.Flag != wantInput.flag ||
			input.Label == "" ||
			input.Required != wantInput.required {
			t.Fatalf("input[%d] = %+v, want %+v", i, input, wantInput)
		}
	}
}

func assertNoPlaceholderRunCommand(t *testing.T, action cliresult.NextAction) {
	t.Helper()
	if action.Type == actionRunCommand && strings.Contains(action.Command, "<") {
		t.Fatalf("run_command contains placeholder: %+v", action)
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

func configuredProjectStoreWithSource() *fakeProjectStore {
	store := configuredProjectStore()
	store.config.Sources = []project.Source{
		{Name: "ga4", Path: "./sources/ga4"},
	}
	return store
}

func configuredProjectStoreWithIdentitySource() *fakeProjectStore {
	store := configuredProjectStore()
	store.config.Sources = []project.Source{
		{Name: "sdk_identity", Path: "./sources/sdk_identity"},
	}
	return store
}

func configuredProjectStoreWithRequiredSources() *fakeProjectStore {
	store := configuredProjectStore()
	store.config.Sources = []project.Source{
		{Name: "ga4", Path: "./sources/ga4"},
		{Name: "crm_conversion_events", Path: "./sources/crm_conversion_events"},
		{Name: "sdk_identity", Path: "./sources/sdk_identity"},
	}
	return store
}

func configuredProjectStoreWithEventAndIdentitySources() *fakeProjectStore {
	store := configuredProjectStore()
	store.config.Sources = []project.Source{
		{Name: "ga4", Path: "./sources/ga4"},
		{Name: "sdk_identity", Path: "./sources/sdk_identity"},
	}
	return store
}

func configuredProjectStoreWithRequiredSourcesAndIdentity() *fakeProjectStore {
	store := configuredProjectStoreWithRequiredSources()
	store.config.Identity = &project.Identity{
		Keys: []project.IdentityKey{
			{
				Name:                    "user_id",
				Tier:                    "deterministic",
				WindowDays:              180,
				MaxDistinctAnonymousIDs: 1000,
				Scope:                   "project",
			},
		},
	}
	return store
}

func fakeRequiredSourceVerifier() *fakeSourceVerifier {
	return &fakeSourceVerifier{
		valid: true,
		contracts: map[string]string{
			"ga4":                   "events",
			"crm_conversion_events": "conversion_events",
			"sdk_identity":          "identity_keys",
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

func (store *fakeProjectStore) SelectWarehouse(warehouseType, defaultAuthName string) (project.Config, error) {
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
		store.config.Warehouse.Auth = defaultAuthName
	}
	return store.config, nil
}

type fakeCredentialStore struct {
	hasBigQueryCredential    bool
	hasBigQueryCredentialErr error
	hasAccessMarker          bool
	hasAccessMarkerErr       error
}

func (store *fakeCredentialStore) HasCredential(warehouseType, name string) (bool, error) {
	_ = warehouseType
	_ = name
	if store.hasBigQueryCredentialErr != nil {
		return false, store.hasBigQueryCredentialErr
	}
	return store.hasBigQueryCredential, nil
}

func (store *fakeCredentialStore) HasMatchingAccessMarker(warehouseType, name string, config project.Warehouse) (bool, error) {
	_ = warehouseType
	_ = name
	_ = config
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

type fakeSourceVerifier struct {
	valid     bool
	reason    string
	err       error
	contracts map[string]string
}

func (verifier *fakeSourceVerifier) CheckSource(projectRoot string, source project.Source) (SourceVerificationStatus, error) {
	if verifier.err != nil {
		return SourceVerificationStatus{}, verifier.err
	}
	contractType := verifier.contracts[source.Name]
	if contractType == "" {
		contractType = "events"
	}
	return SourceVerificationStatus{
		Valid:  verifier.valid,
		Reason: verifier.reason,
		Contract: sourcepkg.ContractIdentity{
			Type:          contractType,
			SchemaVersion: 1,
		},
	}, nil
}
