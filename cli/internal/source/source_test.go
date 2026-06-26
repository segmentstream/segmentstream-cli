package source

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContractsLoadFromEmbeddedTemplates(t *testing.T) {
	contracts, err := Contracts()
	if err != nil {
		t.Fatalf("Contracts failed: %v", err)
	}
	if len(contracts) != 3 {
		t.Fatalf("contracts = %+v, want exactly three supported contracts", contracts)
	}
	contract := findContract(t, contracts, "events")
	if contract.Type != "events" || contract.SchemaVersion != 1 {
		t.Fatalf("contract identity = %s/%d, want events/1", contract.Type, contract.SchemaVersion)
	}
	if !contract.Default {
		t.Fatal("events contract should be the default")
	}
	if contract.Status != "supported" {
		t.Fatalf("status = %q, want supported", contract.Status)
	}
	if contract.Model.Name != "events" || contract.Model.Partition != "event_date" {
		t.Fatalf("model = %+v, want events partitioned by event_date", contract.Model)
	}
	if len(contract.Columns) != 7 {
		t.Fatalf("columns = %+v, want 7 event columns", contract.Columns)
	}
	if contract.Columns[0].Name != "event_id" || !contract.Columns[0].Required {
		t.Fatalf("first column = %+v, want required event_id", contract.Columns[0])
	}

	identityContract := findContract(t, contracts, "identity_keys")
	if identityContract.Default {
		t.Fatal("identity_keys contract should not be the default")
	}
	if identityContract.Model.Name != "identity_keys" || identityContract.Model.Partition != "date" {
		t.Fatalf("identity model = %+v, want identity_keys partitioned by date", identityContract.Model)
	}
	if identityContract.SchemaVersion != 2 {
		t.Fatalf("identity schema version = %d, want 2", identityContract.SchemaVersion)
	}
	if len(identityContract.Columns) != 5 {
		t.Fatalf("identity columns = %+v, want 5 columns", identityContract.Columns)
	}
	if identityContract.Columns[0].Name != "date" || !identityContract.Columns[0].Required {
		t.Fatalf("first identity column = %+v, want required date", identityContract.Columns[0])
	}
	if identityContract.Columns[1].Name != "observed_at" || !identityContract.Columns[1].Required {
		t.Fatalf("second identity column = %+v, want required observed_at", identityContract.Columns[1])
	}
	if len(identityContract.Migrations) != 1 ||
		identityContract.Migrations[0].From != 1 ||
		identityContract.Migrations[0].To != 2 ||
		identityContract.Migrations[0].Guide != "migrations/1_to_2.md" {
		t.Fatalf("identity migrations = %+v, want v1 to v2 guide", identityContract.Migrations)
	}

	conversionEventsContract := findContract(t, contracts, "conversion_events")
	if conversionEventsContract.Default {
		t.Fatal("conversion_events contract should not be the default")
	}
	if conversionEventsContract.Model.Name != "conversion_events" || conversionEventsContract.Model.Partition != "date" {
		t.Fatalf("conversion_events model = %+v, want conversion_events partitioned by date", conversionEventsContract.Model)
	}
	if conversionEventsContract.SchemaVersion != 1 {
		t.Fatalf("conversion_events schema version = %d, want 1", conversionEventsContract.SchemaVersion)
	}
	if len(conversionEventsContract.Columns) != 5 {
		t.Fatalf("conversion_events columns = %+v, want 5 columns", conversionEventsContract.Columns)
	}
	if conversionEventsContract.Columns[0].Name != "date" || !conversionEventsContract.Columns[0].Required {
		t.Fatalf("first conversion_events column = %+v, want required date", conversionEventsContract.Columns[0])
	}
	if conversionEventsContract.Columns[4].Name != "conversion_value" || conversionEventsContract.Columns[4].Required {
		t.Fatalf("conversion value column = %+v, want optional conversion_value", conversionEventsContract.Columns[4])
	}
}

func TestContractByTypeRejectsUnknownType(t *testing.T) {
	_, err := ContractByType("costs")
	if err == nil {
		t.Fatal("expected unknown contract type error")
	}
	if !strings.Contains(err.Error(), `unknown source contract type "costs"`) ||
		!strings.Contains(err.Error(), "supported types: conversion_events, events, identity_keys") {
		t.Fatalf("error = %v, want clear unknown type message", err)
	}
}

func TestCreateScaffoldsSourcePackageFromContract(t *testing.T) {
	root := t.TempDir()
	writeProjectConfig(t, root)

	source, err := Create(root, "ga4", "events")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if source.Name != "ga4" {
		t.Fatalf("Name = %q, want ga4", source.Name)
	}
	if source.PackageName != "segmentstream_source_ga4" {
		t.Fatalf("PackageName = %q, want segmentstream_source_ga4", source.PackageName)
	}
	if source.Contract.Type != "events" || source.Contract.SchemaVersion != 1 {
		t.Fatalf("Contract = %+v, want events schema version 1", source.Contract)
	}
	if source.ModelName != "events" {
		t.Fatalf("ModelName = %q, want events", source.ModelName)
	}

	expectedFiles := []string{
		".gitignore",
		"contract.yml",
		"dbt_project.yml",
		filepath.Join("models", "events.sql"),
		filepath.Join("models", "schema.yml"),
		"README.md",
		"source.yml",
		filepath.Join("tests", "verify_events_contract.sql"),
		filepath.Join("tests", "verify_events_non_empty.sql"),
	}
	for _, relative := range expectedFiles {
		assertGenerated(t, filepath.Join(source.Path, relative))
		if !containsCreatedFile(source.CreatedFiles, filepath.ToSlash(filepath.Join("sources", "ga4", relative))) {
			t.Fatalf("CreatedFiles = %v, want %s", source.CreatedFiles, relative)
		}
	}
	for _, relative := range []string{
		"IMPLEMENTATION_GUIDE.md",
		"migrations",
		"macros",
		"seeds",
		"snapshots",
		filepath.Join("models", "marts"),
		filepath.Join("models", "staging"),
		filepath.Join("models", "exports"),
	} {
		assertMissing(t, filepath.Join(source.Path, relative))
	}

	readme, err := os.ReadFile(filepath.Join(source.Path, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# ga4 Events Source",
		"generated SegmentStream source scaffold",
		"contract.yml",
		"models/schema.yml",
		"Output Schema",
	} {
		if !strings.Contains(string(readme), want) {
			t.Fatalf("README.md does not contain %q:\n%s", want, string(readme))
		}
	}

	schema, err := os.ReadFile(filepath.Join(source.Path, "models", "schema.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"name: ga4_raw",
		"database: REPLACE_WITH_RAW_BIGQUERY_PROJECT",
		"identifier: REPLACE_WITH_RAW_EVENTS_TABLE",
		"contract:",
		"type: events",
	} {
		if !strings.Contains(string(schema), want) {
			t.Fatalf("schema.yml does not contain %q:\n%s", want, string(schema))
		}
	}

	model, err := os.ReadFile(filepath.Join(source.Path, "models", "events.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(model), "__SOURCE_NAME__") {
		t.Fatalf("model still contains source placeholder:\n%s", string(model))
	}
	for _, want := range []string{
		"segmentstream_start_date",
		"segmentstream_end_date",
		"Implement sources/ga4/models/events.sql",
		"event_id",
		"where false",
	} {
		if !strings.Contains(string(model), want) {
			t.Fatalf("model does not contain %q:\n%s", want, string(model))
		}
	}

	dbtProject, err := os.ReadFile(filepath.Join(source.Path, "dbt_project.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"profile: segmentstream",
		"test-paths:",
		"- tests",
	} {
		if !strings.Contains(string(dbtProject), want) {
			t.Fatalf("dbt_project.yml does not contain %q:\n%s", want, string(dbtProject))
		}
	}

	contractTest, err := os.ReadFile(filepath.Join(source.Path, "tests", "verify_events_contract.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"segmentstream_source_verify",
		"cast(event_id as string)",
		"event_id is null",
		"event_date >= date('{{ segmentstream_end_date }}')",
	} {
		if !strings.Contains(string(contractTest), want) {
			t.Fatalf("contract test does not contain %q:\n%s", want, string(contractTest))
		}
	}

	nonEmptyTest, err := os.ReadFile(filepath.Join(source.Path, "tests", "verify_events_non_empty.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(nonEmptyTest), "Source returned no rows") {
		t.Fatalf("non-empty test is missing failure message:\n%s", string(nonEmptyTest))
	}
}

func TestCreateScaffoldsIdentityKeysSourcePackageFromContract(t *testing.T) {
	root := t.TempDir()
	writeProjectConfig(t, root)

	source, err := Create(root, "crm", "identity_keys")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if source.Contract.Type != "identity_keys" || source.Contract.SchemaVersion != 2 {
		t.Fatalf("Contract = %+v, want identity_keys schema version 2", source.Contract)
	}
	if source.ModelName != "identity_keys" {
		t.Fatalf("ModelName = %q, want identity_keys", source.ModelName)
	}

	expectedFiles := []string{
		".gitignore",
		"contract.yml",
		"dbt_project.yml",
		filepath.Join("models", "identity_keys.sql"),
		filepath.Join("models", "schema.yml"),
		"README.md",
		"source.yml",
		filepath.Join("tests", "verify_identity_keys_contract.sql"),
		filepath.Join("tests", "verify_identity_keys_non_empty.sql"),
	}
	for _, relative := range expectedFiles {
		assertGenerated(t, filepath.Join(source.Path, relative))
		if !containsCreatedFile(source.CreatedFiles, filepath.ToSlash(filepath.Join("sources", "crm", relative))) {
			t.Fatalf("CreatedFiles = %v, want %s", source.CreatedFiles, relative)
		}
	}
	assertMissing(t, filepath.Join(source.Path, "migrations"))

	readme, err := os.ReadFile(filepath.Join(source.Path, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# crm Identity Keys Source",
		"identity_keys",
		"Output Schema",
	} {
		if !strings.Contains(string(readme), want) {
			t.Fatalf("README.md does not contain %q:\n%s", want, string(readme))
		}
	}

	schema, err := os.ReadFile(filepath.Join(source.Path, "models", "schema.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"name: crm_raw",
		"identifier: REPLACE_WITH_RAW_IDENTITY_KEYS_TABLE",
		"type: identity_keys",
		"schema_version: 2",
		"name: identity_keys",
		"observed_at",
	} {
		if !strings.Contains(string(schema), want) {
			t.Fatalf("schema.yml does not contain %q:\n%s", want, string(schema))
		}
	}

	model, err := os.ReadFile(filepath.Join(source.Path, "models", "identity_keys.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"segmentstream_start_date",
		"segmentstream_end_date",
		"Implement sources/crm/models/identity_keys.sql",
		"observed_at",
		"anonymous_id",
		"key_name",
		"where false",
	} {
		if !strings.Contains(string(model), want) {
			t.Fatalf("model does not contain %q:\n%s", want, string(model))
		}
	}

	contractTest, err := os.ReadFile(filepath.Join(source.Path, "tests", "verify_identity_keys_contract.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"segmentstream_source_verify",
		"cast(date as date)",
		"cast(observed_at as timestamp)",
		"observed_at is null",
		"anonymous_id is null",
		"date >= date('{{ segmentstream_end_date }}')",
		"date != date(observed_at)",
	} {
		if !strings.Contains(string(contractTest), want) {
			t.Fatalf("contract test does not contain %q:\n%s", want, string(contractTest))
		}
	}
}

func TestCreateScaffoldsConversionEventsSourcePackageFromContract(t *testing.T) {
	root := t.TempDir()
	writeProjectConfig(t, root)

	source, err := Create(root, "crm_conversion_events", "conversion_events")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if source.Contract.Type != "conversion_events" || source.Contract.SchemaVersion != 1 {
		t.Fatalf("Contract = %+v, want conversion_events schema version 1", source.Contract)
	}
	if source.ModelName != "conversion_events" {
		t.Fatalf("ModelName = %q, want conversion_events", source.ModelName)
	}

	expectedFiles := []string{
		".gitignore",
		"contract.yml",
		"dbt_project.yml",
		filepath.Join("models", "conversion_events.sql"),
		filepath.Join("models", "schema.yml"),
		"README.md",
		"source.yml",
		filepath.Join("tests", "verify_conversion_events_contract.sql"),
		filepath.Join("tests", "verify_conversion_events_non_empty.sql"),
	}
	for _, relative := range expectedFiles {
		assertGenerated(t, filepath.Join(source.Path, relative))
		if !containsCreatedFile(source.CreatedFiles, filepath.ToSlash(filepath.Join("sources", "crm_conversion_events", relative))) {
			t.Fatalf("CreatedFiles = %v, want %s", source.CreatedFiles, relative)
		}
	}

	readme, err := os.ReadFile(filepath.Join(source.Path, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# crm_conversion_events Conversion Events Source",
		"conversion_events",
		"may be null",
		"Output Schema",
	} {
		if !strings.Contains(string(readme), want) {
			t.Fatalf("README.md does not contain %q:\n%s", want, string(readme))
		}
	}

	schema, err := os.ReadFile(filepath.Join(source.Path, "models", "schema.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"name: crm_conversion_events_raw",
		"identifier: REPLACE_WITH_RAW_CONVERSION_EVENTS_TABLE",
		"type: conversion_events",
		"schema_version: 1",
		"name: conversion_events",
		"conversion_time",
		"conversion_value",
	} {
		if !strings.Contains(string(schema), want) {
			t.Fatalf("schema.yml does not contain %q:\n%s", want, string(schema))
		}
	}

	model, err := os.ReadFile(filepath.Join(source.Path, "models", "conversion_events.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"segmentstream_start_date",
		"segmentstream_end_date",
		"Implement sources/crm_conversion_events/models/conversion_events.sql",
		"conversion_name",
		"conversion_id",
		"cast(null as float64) as conversion_value",
		"where false",
	} {
		if !strings.Contains(string(model), want) {
			t.Fatalf("model does not contain %q:\n%s", want, string(model))
		}
	}

	contractTest, err := os.ReadFile(filepath.Join(source.Path, "tests", "verify_conversion_events_contract.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"segmentstream_source_verify",
		"cast(date as date)",
		"cast(conversion_time as timestamp)",
		"cast(conversion_value as float64)",
		"conversion_time is null",
		"conversion_name is null",
		"conversion_id is null",
		"date >= date('{{ segmentstream_end_date }}')",
		"date != date(conversion_time)",
	} {
		if !strings.Contains(string(contractTest), want) {
			t.Fatalf("contract test does not contain %q:\n%s", want, string(contractTest))
		}
	}
	if strings.Contains(string(contractTest), "conversion_value is null") {
		t.Fatalf("contract test should allow null conversion_value:\n%s", string(contractTest))
	}

	nonEmptyTest, err := os.ReadFile(filepath.Join(source.Path, "tests", "verify_conversion_events_non_empty.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(nonEmptyTest), "Source returned no conversion rows") {
		t.Fatalf("non-empty test is missing failure message:\n%s", string(nonEmptyTest))
	}
}

func TestValidateSupportedContractIdentityReturnsMigrationGuide(t *testing.T) {
	err := ValidateSupportedContractIdentityForSource(
		ContractIdentity{Type: "identity_keys", SchemaVersion: 1},
		ContractValidationContext{
			SourceName: "sdk_identity",
			SourcePath: filepath.Join("sources", "sdk_identity"),
		},
	)
	if err == nil {
		t.Fatal("expected migration-required error")
	}

	migrationErr, ok := AsContractMigrationRequired(err)
	if !ok {
		t.Fatalf("error = %T %v, want ContractMigrationRequiredError", err, err)
	}
	if !errors.As(err, &migrationErr) {
		t.Fatalf("errors.As did not match migration error")
	}
	if migrationErr.ContractType != "identity_keys" ||
		migrationErr.FromSchemaVersion != 1 ||
		migrationErr.ToSchemaVersion != 2 ||
		migrationErr.SourceName != "sdk_identity" ||
		migrationErr.NextCommand != "segmentstream source verify sdk_identity" {
		t.Fatalf("migration error = %+v, want populated identity migration", migrationErr)
	}
	for _, want := range []string{
		"Migration guide",
		"schema_version: 2",
		"sources/sdk_identity/models/identity_keys.sql",
		"observed_at",
		"analytics-core now performs that",
		"segmentstream source verify sdk_identity",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "__SOURCE_NAME__") || strings.Contains(err.Error(), "__SOURCE_PATH__") {
		t.Fatalf("error still contains template placeholders:\n%v", err)
	}
}

func TestCreateRequiresSegmentStreamProject(t *testing.T) {
	_, err := Create(t.TempDir(), "ga4", "events")
	if err == nil {
		t.Fatal("expected missing project config error")
	}
	if !strings.Contains(err.Error(), "segmentstream.yml was not found") {
		t.Fatalf("error = %v, want missing config message", err)
	}
}

func TestCreateRejectsInvalidSourceName(t *testing.T) {
	root := t.TempDir()
	writeProjectConfig(t, root)

	for _, name := range []string{"", "GA4", "ga-4", "4ga", "../ga4"} {
		_, err := Create(root, name, "events")
		if err == nil {
			t.Fatalf("expected invalid source name error for %q", name)
		}
	}
}

func TestCreateDoesNotOverwriteExistingSource(t *testing.T) {
	root := t.TempDir()
	writeProjectConfig(t, root)

	existing := filepath.Join(root, SourcesDirName, "ga4")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := Create(root, "ga4", "events")
	if err == nil {
		t.Fatal("expected existing source error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error = %v, want already exists", err)
	}
}

func TestCreateUsesRequestedContract(t *testing.T) {
	root := t.TempDir()
	writeProjectConfig(t, root)

	source, err := Create(root, "ga4", "events")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if source.Contract.Type != "events" || source.ModelName != "events" {
		t.Fatalf("source = %+v, want events contract and model", source)
	}

	identitySource, err := Create(root, "crm", "identity_keys")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if identitySource.Contract.Type != "identity_keys" || identitySource.ModelName != "identity_keys" {
		t.Fatalf("source = %+v, want identity_keys contract and model", identitySource)
	}

	conversionEventsSource, err := Create(root, "crm_conversion_events", "conversion_events")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if conversionEventsSource.Contract.Type != "conversion_events" || conversionEventsSource.ModelName != "conversion_events" {
		t.Fatalf("source = %+v, want conversion_events contract and model", conversionEventsSource)
	}
}

func TestValidateSourceDirRejectsUnexpectedPath(t *testing.T) {
	root := t.TempDir()
	err := validateSourceDir(root, "ga4", filepath.Join(root, "outside"))
	if err == nil {
		t.Fatal("expected path safety error")
	}
	if !strings.Contains(err.Error(), "refusing to write source directory") {
		t.Fatalf("error = %v, want path safety refusal", err)
	}
}

func writeProjectConfig(t *testing.T, root string) {
	t.Helper()
	config := `version: 1
warehouse:
  type: bigquery
  auth: default-bigquery
  project: example-project
  dataset: segmentstream
`
	if err := os.WriteFile(filepath.Join(root, "segmentstream.yml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
}

func findContract(t *testing.T, contracts []Contract, contractType string) Contract {
	t.Helper()
	for _, contract := range contracts {
		if contract.Type == contractType {
			return contract
		}
	}
	t.Fatalf("contracts = %+v, want contract type %q", contracts, contractType)
	return Contract{}
}

func assertGenerated(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected generated path %s: %v", path, err)
	}
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected path %s to be missing, stat error = %v", path, err)
	}
}

func containsCreatedFile(files []string, want string) bool {
	for _, file := range files {
		if file == want {
			return true
		}
	}
	return false
}
