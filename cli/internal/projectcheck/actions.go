package projectcheck

import (
	"fmt"

	"github.com/segmentstream/segmentstream-cli/cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/cli/internal/project"
	"github.com/segmentstream/segmentstream-cli/cli/internal/warehouse"
)

const (
	testWarehouseCommand             = "segmentstream warehouse test --json"
	sourceContractsCommand           = "segmentstream source contracts"
	eventSourceContractCommand       = "segmentstream source contracts --type events"
	identityKeySourceContractCommand = "segmentstream source contracts --type identity_keys"
	initVerifyCommand                = "segmentstream init --json"
	runCommand                       = "segmentstream run"

	actionHumanInput = "human_input"
	actionRunCommand = "run_command"
)

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
		Reason:  "No sources are configured. Inspect supported source contracts, then configure at least one events source and one identity_keys source.",
	}
}

func selectRequiredSourceContractAction(command, reason string) cliresult.NextAction {
	return cliresult.NextAction{
		Type:    actionRunCommand,
		Stage:   string(stageSources),
		Command: command,
		Reason:  reason,
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

func configureIdentityAction() cliresult.NextAction {
	return cliresult.NextAction{
		Type:   actionHumanInput,
		Stage:  string(stageIdentity),
		Reason: "Configure identity.keys in segmentstream.yml using keys emitted by your identity_keys source.",
		Accepts: []cliresult.NextActionAccept{
			{
				Method: "edit_segmentstream_yml",
				Label:  "Add identity.keys to segmentstream.yml",
			},
		},
		Verify: initVerifyCommand,
	}
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
