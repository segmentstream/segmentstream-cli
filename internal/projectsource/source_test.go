package projectsource

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestContractsLoadFromEmbeddedTemplates(t *testing.T) {
	contracts, err := Contracts()
	if err != nil {
		t.Fatalf("Contracts failed: %v", err)
	}
	if len(contracts) != 1 {
		t.Fatalf("contracts = %+v, want exactly one supported contract", contracts)
	}
	contract := contracts[0]
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
}

func TestContractByTypeRejectsUnknownType(t *testing.T) {
	_, err := ContractByType("costs")
	if err == nil {
		t.Fatal("expected unknown contract type error")
	}
	if !strings.Contains(err.Error(), `unknown source contract type "costs"`) ||
		!strings.Contains(err.Error(), "supported types: events") {
		t.Fatalf("error = %v, want clear unknown type message", err)
	}
}

func TestInitCreatesSourcePackageFromDefaultContract(t *testing.T) {
	root := t.TempDir()
	writeProjectConfig(t, root)

	source, err := Init(root, "ga4")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
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
		"contract.yml",
		"dbt_project.yml",
		filepath.Join("models", "events.sql"),
		filepath.Join("models", "schema.yml"),
		"source.yml",
	}
	for _, relative := range expectedFiles {
		assertGenerated(t, filepath.Join(source.Path, relative))
	}
	if got := collectGeneratedFiles(t, source.Path); !reflect.DeepEqual(got, expectedFiles) {
		t.Fatalf("generated files = %v, want %v", got, expectedFiles)
	}
	expectedCreatedFiles := []string{
		"sources/ga4/contract.yml",
		"sources/ga4/dbt_project.yml",
		"sources/ga4/models/events.sql",
		"sources/ga4/models/schema.yml",
		"sources/ga4/source.yml",
	}
	if !reflect.DeepEqual(source.CreatedFiles, expectedCreatedFiles) {
		t.Fatalf("CreatedFiles = %v, want %v", source.CreatedFiles, expectedCreatedFiles)
	}
	for _, relative := range []string{
		"README.md",
		".gitignore",
		"macros",
		"seeds",
		"snapshots",
		"tests",
		filepath.Join("models", "staging"),
		filepath.Join("models", "exports"),
	} {
		assertMissing(t, filepath.Join(source.Path, relative))
	}

	contract, err := os.ReadFile(filepath.Join(source.Path, "contract.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"type: events",
		"schema_version: 1",
		"default: true",
		"status: supported",
		"name: event_id",
		"type: STRING",
		"name: event_date",
	} {
		if !strings.Contains(string(contract), want) {
			t.Fatalf("contract.yml does not contain %q:\n%s", want, string(contract))
		}
	}

	project, err := os.ReadFile(filepath.Join(source.Path, "dbt_project.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"name: segmentstream_source_ga4",
		"+materialized: ephemeral",
		"clean-targets:",
	} {
		if !strings.Contains(string(project), want) {
			t.Fatalf("dbt_project.yml does not contain %q:\n%s", want, string(project))
		}
	}
	if strings.Contains(string(project), "analysis-paths") {
		t.Fatalf("dbt_project.yml should not contain analysis paths:\n%s", string(project))
	}
	for _, notWant := range []string{
		"staging:",
		"exports:",
		"+materialized: incremental",
		"+incremental_strategy: insert_overwrite",
		"+partition_by:",
		"test-paths:",
		"seed-paths:",
		"macro-paths:",
		"snapshot-paths:",
	} {
		if strings.Contains(string(project), notWant) {
			t.Fatalf("dbt_project.yml should not contain %q:\n%s", notWant, string(project))
		}
	}

	sourceYAML, err := os.ReadFile(filepath.Join(source.Path, "source.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"name: ga4_raw",
		"identifier: \"{{ var('ga4_raw_events_table', 'ga4_events') }}\"",
	} {
		if !strings.Contains(string(sourceYAML), want) {
			t.Fatalf("source.yml does not contain %q:\n%s", want, string(sourceYAML))
		}
	}

	schema, err := os.ReadFile(filepath.Join(source.Path, "models", "schema.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"name: events",
		"sources:",
		"name: ga4_raw",
		"identifier: \"{{ var('ga4_raw_events_table', 'ga4_events') }}\"",
		"segmentstream:",
		"source_name: ga4",
		"contract:",
		"type: events",
		"schema_version: 1",
		"name: event_id",
		"name: page_url",
		"name: page_referrer",
		"name: event_date",
	} {
		if !strings.Contains(string(schema), want) {
			t.Fatalf("schema.yml does not contain %q:\n%s", want, string(schema))
		}
	}
	for _, notWant := range []string{
		"events_v1",
		"name: events_ga4",
		"name: source_event_id",
		"name: user_id",
		"data_tests:",
		"not_null",
	} {
		if strings.Contains(string(schema), notWant) {
			t.Fatalf("schema.yml should not contain %q:\n%s", notWant, string(schema))
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
		"{{ config(materialized='ephemeral', alias='ga4_events') }}",
		"event_id",
		"page_url",
		"page_referrer",
		"from {{ source('ga4_raw', 'events') }}",
		"where date(cast(event_timestamp as timestamp)) >= date('{{ segmentstream_start_date }}')",
		"and date(cast(event_timestamp as timestamp)) < date('{{ segmentstream_end_date }}')",
	} {
		if !strings.Contains(string(model), want) {
			t.Fatalf("model does not contain %q:\n%s", want, string(model))
		}
	}
	for _, notWant := range []string{
		"stg_events_ga4",
		"ref(",
		"is_incremental()",
		"_dbt_max_partition",
		"source_event_id",
		"user_id",
	} {
		if strings.Contains(string(model), notWant) {
			t.Fatalf("model should not contain %q:\n%s", notWant, string(model))
		}
	}
}

func TestInitRequiresSegmentStreamProject(t *testing.T) {
	_, err := Init(t.TempDir(), "ga4")
	if err == nil {
		t.Fatal("expected missing project config error")
	}
	if !strings.Contains(err.Error(), "segmentstream.yml was not found") {
		t.Fatalf("error = %v, want missing config message", err)
	}
}

func TestInitRejectsInvalidSourceName(t *testing.T) {
	root := t.TempDir()
	writeProjectConfig(t, root)

	for _, name := range []string{"", "GA4", "ga-4", "4ga", "../ga4"} {
		_, err := Init(root, name)
		if err == nil {
			t.Fatalf("expected invalid source name error for %q", name)
		}
	}
}

func TestInitDoesNotOverwriteExistingSource(t *testing.T) {
	root := t.TempDir()
	writeProjectConfig(t, root)

	existing := filepath.Join(root, SourcesDirName, "ga4")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := Init(root, "ga4")
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

func collectGeneratedFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, relative)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return files
}
