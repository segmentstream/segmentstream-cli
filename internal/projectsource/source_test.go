package projectsource

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCreatesSourceTemplate(t *testing.T) {
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

	for _, relative := range []string{
		"README.md",
		".gitignore",
		"dbt_project.yml",
		filepath.Join("models", "exports", "events_ga4.sql"),
		filepath.Join("models", "exports", "schema.yml"),
		filepath.Join("models", "staging", "README.md"),
		filepath.Join("models", "staging", "sources.yml"),
		filepath.Join("models", "staging", "stg_events_ga4.sql"),
		filepath.Join("macros", "README.md"),
		filepath.Join("seeds", "README.md"),
		filepath.Join("snapshots", "README.md"),
		filepath.Join("tests", "README.md"),
	} {
		assertGenerated(t, filepath.Join(source.Path, relative))
	}
	assertMissing(t, filepath.Join(source.Path, "analyses"))

	project, err := os.ReadFile(filepath.Join(source.Path, "dbt_project.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"name: segmentstream_source_ga4",
		"snapshot-paths:",
		"+materialized: ephemeral",
		"+materialized: incremental",
		"+incremental_strategy: insert_overwrite",
		"field: event_date",
		"data_type: date",
		"clean-targets:",
	} {
		if !strings.Contains(string(project), want) {
			t.Fatalf("dbt_project.yml does not contain %q:\n%s", want, string(project))
		}
	}
	if strings.Contains(string(project), "analysis-paths") {
		t.Fatalf("dbt_project.yml should not contain analysis paths:\n%s", string(project))
	}

	schema, err := os.ReadFile(filepath.Join(source.Path, "models", "exports", "schema.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"name: events_ga4",
		"segmentstream:",
		"source_name: ga4",
		"export_name: events",
		"contract: events_v1",
		"name: event_date",
		"not_null",
	} {
		if !strings.Contains(string(schema), want) {
			t.Fatalf("schema.yml does not contain %q:\n%s", want, string(schema))
		}
	}

	model, err := os.ReadFile(filepath.Join(source.Path, "models", "exports", "events_ga4.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(model), "__SOURCE_NAME__") {
		t.Fatalf("model still contains source placeholder:\n%s", string(model))
	}
	for _, want := range []string{
		"segmentstream_start_date",
		"segmentstream_end_date",
		"from {{ ref('stg_events_ga4') }}",
		"where event_date >= date('{{ segmentstream_start_date }}')",
		"and event_date < date('{{ segmentstream_end_date }}')",
	} {
		if !strings.Contains(string(model), want) {
			t.Fatalf("model does not contain %q:\n%s", want, string(model))
		}
	}
	for _, notWant := range []string{
		"is_incremental()",
		"_dbt_max_partition",
	} {
		if strings.Contains(string(model), notWant) {
			t.Fatalf("model should not contain %q:\n%s", notWant, string(model))
		}
	}

	staging, err := os.ReadFile(filepath.Join(source.Path, "models", "staging", "stg_events_ga4.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"from {{ source('ga4_raw', 'events') }}",
		"date(cast(event_timestamp as timestamp)) as event_date",
	} {
		if !strings.Contains(string(staging), want) {
			t.Fatalf("staging model does not contain %q:\n%s", want, string(staging))
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
