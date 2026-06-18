package projectruntime

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/segmentstream/segmentstream-cli/internal/project"
	"gopkg.in/yaml.v3"
)

func TestPrepareCreatesExpectedRuntimeFiles(t *testing.T) {
	root := t.TempDir()

	if err := Prepare(root, testConfig()); err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	for _, relative := range []string{
		"Dockerfile",
		"README.md",
		"docker-compose.yml",
		"dagster.yaml",
		"dbt_project.yml",
		"profiles.yml",
		".env",
		filepath.Join("dagster", "definitions.py"),
		filepath.Join("dagster", "segmentstream.py"),
		filepath.Join("dbt", "models", "exports", "schema.yml"),
		filepath.Join("dbt", "models", "staging"),
		filepath.Join("dbt", "macros"),
		filepath.Join("dbt", "snapshots"),
	} {
		path := filepath.Join(root, RuntimeDirName, relative)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected generated path %s: %v", relative, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, RuntimeDirName, "dagster", "run.py")); !os.IsNotExist(err) {
		t.Fatalf("dagster/run.py should not be generated, stat error = %v", err)
	}

	profiles, err := os.ReadFile(filepath.Join(root, RuntimeDirName, "profiles.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(profiles), `project: "{{ env_var('SEGMENTSTREAM_BQ_PROJECT') }}"`) {
		t.Fatalf("profiles.yml does not contain BigQuery project env var:\n%s", string(profiles))
	}
	if strings.Contains(string(profiles), "example-project") {
		t.Fatalf("profiles.yml should be static, got rendered project:\n%s", string(profiles))
	}

	dockerfile, err := os.ReadFile(filepath.Join(root, RuntimeDirName, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dockerfile), "dagster-postgres") {
		t.Fatalf("Dockerfile does not install dagster-postgres:\n%s", string(dockerfile))
	}

	dagsterConfig, err := os.ReadFile(filepath.Join(root, RuntimeDirName, "dagster.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var parsedDagsterConfig map[string]any
	if err := yaml.Unmarshal(dagsterConfig, &parsedDagsterConfig); err != nil {
		t.Fatalf("dagster.yaml is not valid YAML: %v\n%s", err, string(dagsterConfig))
	}
	for _, want := range []string{
		"storage:",
		"postgres:",
		"postgres_db:",
		"env: DAGSTER_PG_USERNAME",
		"env: DAGSTER_PG_PASSWORD",
		"env: DAGSTER_PG_HOST",
		"env: DAGSTER_PG_DB",
	} {
		if !strings.Contains(string(dagsterConfig), want) {
			t.Fatalf("dagster.yaml does not contain %q:\n%s", want, string(dagsterConfig))
		}
	}

	definitions, err := os.ReadFile(filepath.Join(root, RuntimeDirName, "dagster", "definitions.py"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"dbt_assets",
		"DbtCliResource",
		"DailyPartitionsDefinition",
		`start_date="1970-01-01"`,
		"partitions_def=segmentstream_daily_partitions",
		"dbt_partition_vars",
		"define_asset_job",
		"AssetSelection.all()",
		"segmentstream_materialize_all",
		"build_ingestion_assets",
	} {
		if !strings.Contains(string(definitions), want) {
			t.Fatalf("Dagster definitions do not contain %q:\n%s", want, string(definitions))
		}
	}
	for _, notWant := range []string{
		"@op",
		"def run_segmentstream_dbt",
		"@job",
	} {
		if strings.Contains(string(definitions), notWant) {
			t.Fatalf("Dagster definitions should not contain %q:\n%s", notWant, string(definitions))
		}
	}

	dagsterResolver, err := os.ReadFile(filepath.Join(root, RuntimeDirName, "dagster", "segmentstream.py"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"segmentstream.yml",
		"packages.yml",
		"events.sql",
		`"deps"`,
		`"parse"`,
		"build_ingestion_assets",
		"dbt_partition_vars",
		"segmentstream_start_date",
		"segmentstream_end_date",
		"discover_events_model_name",
		`ref("{source.package_name}", "{source.events_model_name}")`,
		`legacy_model = f"events_{name}"`,
		"where event_date >= date('{{ segmentstream_start_date }}')",
		"and event_date < date('{{ segmentstream_end_date }}')",
	} {
		if !strings.Contains(string(dagsterResolver), want) {
			t.Fatalf("Dagster resolver does not contain %q:\n%s", want, string(dagsterResolver))
		}
	}
	if strings.Contains(string(dagsterResolver), "_dbt_max_partition") {
		t.Fatalf("Dagster resolver should not contain _dbt_max_partition:\n%s", string(dagsterResolver))
	}

	runtimeReadme, err := os.ReadFile(filepath.Join(root, RuntimeDirName, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(runtimeReadme), "run.py") {
		t.Fatalf("runtime README should not mention run.py:\n%s", string(runtimeReadme))
	}
}

func TestDagsterResolverEmptyProjectModelUsesValidBigQueryZeroRowQuery(t *testing.T) {
	root := t.TempDir()

	if err := Prepare(root, testConfig()); err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	dagsterResolver, err := os.ReadFile(filepath.Join(root, RuntimeDirName, "dagster", "segmentstream.py"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"from (select 1) as empty_project",
		"where false",
	} {
		if !strings.Contains(string(dagsterResolver), want) {
			t.Fatalf("Dagster resolver empty project model does not contain %q:\n%s", want, string(dagsterResolver))
		}
	}
}

func TestPrepareRemovesStaleRuntimeFiles(t *testing.T) {
	root := t.TempDir()
	stale := filepath.Join(root, RuntimeDirName, "stale.txt")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Prepare(root, testConfig()); err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale file still exists or stat failed with unexpected error: %v", err)
	}
}

func TestPrepareWritesRuntimeEnvAndStaticComposeFile(t *testing.T) {
	root := t.TempDir()

	if err := Prepare(root, testConfig()); err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	compose, err := os.ReadFile(filepath.Join(root, RuntimeDirName, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(compose, &parsed); err != nil {
		t.Fatalf("docker-compose.yml is not valid YAML: %v\n%s", err, string(compose))
	}
	if !strings.Contains(string(compose), `source: "${SEGMENTSTREAM_HOST_HOME}"`) {
		t.Fatalf("docker-compose.yml does not use static host env var mount:\n%s", string(compose))
	}
	for _, want := range []string{
		"postgres:",
		"image: postgres:16-alpine",
		"condition: service_healthy",
		"DAGSTER_HOME: /workspace/.segmentstream",
		"DAGSTER_PG_HOST: postgres",
		`GOOGLE_APPLICATION_CREDENTIALS: "${SEGMENTSTREAM_BQ_CREDENTIALS}"`,
		"segmentstream-postgres-data:",
	} {
		if !strings.Contains(string(compose), want) {
			t.Fatalf("docker-compose.yml does not contain %q:\n%s", want, string(compose))
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	hostHome, err := filepath.Abs(filepath.Join(home, ".segmentstream"))
	if err != nil {
		t.Fatal(err)
	}
	env, err := os.ReadFile(filepath.Join(root, RuntimeDirName, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`SEGMENTSTREAM_HOST_HOME=` + strconv.Quote(filepath.ToSlash(hostHome)),
		`SEGMENTSTREAM_BQ_CREDENTIALS="/home/segmentstream/.segmentstream/bigquery/production-bigquery.json"`,
		`SEGMENTSTREAM_BQ_PROJECT="example-project"`,
		`SEGMENTSTREAM_BQ_DATASET="segmentstream"`,
		`SEGMENTSTREAM_BQ_LOCATION="US"`,
	} {
		if !strings.Contains(string(env), want) {
			t.Fatalf(".env does not contain %q:\n%s", want, string(env))
		}
	}
}

func TestValidateRuntimeDirRejectsUnexpectedPath(t *testing.T) {
	root := t.TempDir()
	err := validateRuntimeDir(root, filepath.Join(root, "outside"))
	if err == nil {
		t.Fatal("expected path safety error")
	}
	if !strings.Contains(err.Error(), "refusing to remove runtime directory") {
		t.Fatalf("error = %v, want path safety refusal", err)
	}
}

func testConfig() project.Config {
	return project.Config{
		Version: 1,
		Warehouse: project.Warehouse{
			Type:     "bigquery",
			Auth:     "production-bigquery",
			Project:  "example-project",
			Dataset:  "segmentstream",
			Location: "US",
		},
	}
}
