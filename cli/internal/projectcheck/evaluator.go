package projectcheck

import (
	"context"
	"fmt"

	"github.com/segmentstream/segmentstream-cli/cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/cli/internal/credentials"
	"github.com/segmentstream/segmentstream-cli/cli/internal/project"
	"github.com/segmentstream/segmentstream-cli/cli/internal/warehouse"
)

type Result struct {
	Envelope cliresult.Envelope
	ExitCode int
}

type Evaluator struct {
	ProjectRoot       string
	ProjectStore      ProjectStore
	Credentials       credentials.Store
	CredentialStore   CredentialStore
	WarehouseRegistry warehouse.Registry
	SourceVerifier    SourceVerifier
}

func (evaluator Evaluator) Evaluate(ctx context.Context) (Result, error) {
	_ = ctx

	store := evaluator.projectStore()
	credentialStore := evaluator.credentialStore()
	sourceVerifier := evaluator.sourceVerifier()
	registry := evaluator.WarehouseRegistry

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

	sourceCoverage := sourceContractCoverage{}
	for _, source := range config.Sources {
		status, err := sourceVerifier.CheckSource(evaluator.ProjectRoot, source)
		if err != nil {
			return Result{}, err
		}
		sourceCoverage.record(status.Contract.Type)
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

	if !sourceCoverage.satisfied() {
		return resultFor(envelope, eval.withBlocker(sourceCoverageBlocker(sourceCoverage))), nil
	}
	eval.complete(stageSources)

	if config.Identity == nil || len(config.Identity.Keys) == 0 {
		return resultFor(envelope, eval.withBlocker(blocker{
			StageID:    stageIdentity,
			Status:     statusMissing,
			NextAction: configureIdentityAction(),
			Diagnostics: []cliresult.Diagnostic{
				{
					ID:         "missing_identity_keys",
					Field:      "identity.keys",
					Message:    "segmentstream.yml does not configure identity.keys.",
					Suggestion: "Add at least one key emitted by an identity_keys source under identity.keys, then run segmentstream init --json.",
				},
			},
		})), nil
	}
	eval.complete(stageIdentity)

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
