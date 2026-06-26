package projectcheck

import "github.com/segmentstream/segmentstream-cli/cli/internal/cliresult"

type sourceContractCoverage struct {
	hasEvents           bool
	hasConversionEvents bool
	hasIdentityKeys     bool
}

func (coverage *sourceContractCoverage) record(contractType string) {
	switch contractType {
	case "events":
		coverage.hasEvents = true
	case "conversion_events":
		coverage.hasConversionEvents = true
	case "identity_keys":
		coverage.hasIdentityKeys = true
	}
}

func (coverage sourceContractCoverage) satisfied() bool {
	return coverage.hasEvents && coverage.hasConversionEvents && coverage.hasIdentityKeys
}

func sourceCoverageBlocker(coverage sourceContractCoverage) blocker {
	switch {
	case !coverage.hasEvents && !coverage.hasConversionEvents && !coverage.hasIdentityKeys:
		return requiredSourceBlocker(
			"missing_required_source_contracts",
			sourceContractsCommand,
			"segmentstream.yml must declare at least one events source, one conversion_events source, and one identity_keys source.",
			"Configure one events source, one conversion_events source, and one identity_keys source under sources.",
		)
	case !coverage.hasEvents:
		return requiredSourceBlocker(
			"missing_events_source",
			eventSourceContractCommand,
			"segmentstream.yml must declare at least one events source.",
			"Run segmentstream source scaffold <name> --type events, implement it, add it under sources, then verify it.",
		)
	case !coverage.hasIdentityKeys:
		return requiredSourceBlocker(
			"missing_identity_keys_source",
			identityKeySourceContractCommand,
			"segmentstream.yml must declare at least one identity_keys source.",
			"Run segmentstream source scaffold <name> --type identity_keys, implement it, add it under sources, then verify it.",
		)
	default:
		return requiredSourceBlocker(
			"missing_conversion_events_source",
			conversionEventSourceContractCommand,
			"segmentstream.yml must declare at least one conversion_events source.",
			"Run segmentstream source scaffold <name> --type conversion_events, implement it, add it under sources, then verify it.",
		)
	}
}

func requiredSourceBlocker(id, command, message, suggestion string) blocker {
	return blocker{
		StageID:    stageSources,
		Status:     statusMissing,
		NextAction: selectRequiredSourceContractAction(command, message),
		Diagnostics: []cliresult.Diagnostic{
			{
				ID:         id,
				Field:      "sources",
				Message:    message,
				Suggestion: suggestion,
			},
		},
	}
}

func sourceDiagnosticField(sourceName string) string {
	if sourceName == "" {
		return "sources"
	}
	return "sources." + sourceName
}
