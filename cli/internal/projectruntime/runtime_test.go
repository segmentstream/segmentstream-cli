package projectruntime

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/segmentstream/segmentstream-cli/cli/internal/project"
	"github.com/segmentstream/segmentstream-cli/cli/internal/version"
	"github.com/segmentstream/segmentstream-cli/cli/internal/warehouse"
	"github.com/segmentstream/segmentstream-cli/cli/internal/warehouse/bigquery"
	"gopkg.in/yaml.v3"
)

func testProvider() warehouse.Provider {
	return bigquery.NewConnector()
}

func TestPrepareCreatesExpectedRuntimeFiles(t *testing.T) {
	root := t.TempDir()
	withAnalyticsCoreRelease(t, "0.0.20")

	if err := Prepare(root, testConfig(), testProvider()); err != nil {
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
	if !strings.Contains(string(profiles), "https://www.googleapis.com/auth/bigquery") {
		t.Fatalf("profiles.yml does not contain BigQuery OAuth scope:\n%s", string(profiles))
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
	if !strings.Contains(string(dockerfile), "apt-get install -y --no-install-recommends git") {
		t.Fatalf("Dockerfile does not install git:\n%s", string(dockerfile))
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
		`ANALYTICS_CORE_PACKAGE_NAME = "segmentstream_analytics_core"`,
		"partitioned_dbt_select",
		"unpartitioned_dbt_select",
		"tag:segmentstream_partitioned",
		"tag:segmentstream_unpartitioned",
		"SegmentStreamDbtTranslator",
		"DagsterDbtTranslator",
		"AssetKey",
		"SourceAsset",
		"build_dbt_source_assets",
		"source_asset_key",
		"removesuffix(\"_raw\")",
		"json.loads",
		"DbtCliResource",
		"DailyPartitionsDefinition",
		`start_date="1970-01-01"`,
		"partitions_def=segmentstream_daily_partitions",
		"dbt_partition_vars",
		"dbt_partition_vars(context, segmentstream_config)",
		"dbt_project_vars",
		"dbt_project_vars(segmentstream_config)",
		"segmentstream_partitioned_dbt_assets",
		"segmentstream_unpartitioned_dbt_assets",
		"define_asset_job",
		"AssetSelection.all()",
		"segmentstream_materialize_all",
		"build_ingestion_assets",
		"*build_dbt_source_assets(manifest_path)",
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
		"analytics_core_package",
		"https://github.com/segmentstream/segmentstream.git",
		`"subdirectory": ANALYTICS_CORE_GIT_SUBDIRECTORY`,
		"SEGMENTSTREAM_ANALYTICS_CORE_REVISION",
		"SEGMENTSTREAM_ANALYTICS_CORE_LOCAL_PATH",
		`"deps"`,
		`"parse"`,
		`"--vars"`,
		"build_ingestion_assets",
		"dbt_partition_vars",
		"segmentstream_sources",
		"segmentstream_identity_key_sources",
		"segmentstream_identity_link_keys",
		"segmentstream_start_date",
		"segmentstream_end_date",
		"SUPPORTED_SOURCE_CONTRACT_SCHEMA_VERSIONS",
		"event_source_vars",
		"identity_key_source_vars",
		"parse_identity_link_keys",
		"normalize_positive_int",
		"discover_source_contract",
		"discover_events_model_name",
		`legacy_model = f"events_{name}"`,
	} {
		if !strings.Contains(string(dagsterResolver), want) {
			t.Fatalf("Dagster resolver does not contain %q:\n%s", want, string(dagsterResolver))
		}
	}
	if strings.Contains(string(dagsterResolver), "_dbt_max_partition") {
		t.Fatalf("Dagster resolver should not contain _dbt_max_partition:\n%s", string(dagsterResolver))
	}
	if strings.Contains(string(dagsterResolver), "write_core_events_model") ||
		strings.Contains(string(dagsterResolver), "render_core_events_model") {
		t.Fatalf("Dagster resolver should not generate core events SQL:\n%s", string(dagsterResolver))
	}

	runtimeReadme, err := os.ReadFile(filepath.Join(root, RuntimeDirName, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(runtimeReadme), "run.py") {
		t.Fatalf("runtime README should not mention run.py:\n%s", string(runtimeReadme))
	}
}

func TestAnalyticsCoreIntermediateEventsModelUsesValidBigQueryZeroRowQuery(t *testing.T) {
	eventsModel, err := os.ReadFile(filepath.Join("..", "..", "..", "analytics-core", "models", "intermediate", "events", "int_events__unioned.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"from (select 1) as empty_project",
		"where false",
	} {
		if !strings.Contains(string(eventsModel), want) {
			t.Fatalf("analytics-core events model does not contain %q:\n%s", want, string(eventsModel))
		}
	}
}

func TestAnalyticsCoreIdentityKeysModelsUseExpectedUnionAndDistinctShape(t *testing.T) {
	identityModel, err := os.ReadFile(filepath.Join("..", "..", "..", "analytics-core", "models", "intermediate", "identity", "keys", "int_identity_keys__unioned.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"segmentstream_identity_key_sources",
		"from (select 1) as empty_project",
		"where false",
		`ref(source["package_name"], source["identity_keys_model_name"])`,
		"observed_at",
		"date >= date('{{ segmentstream_start_date }}')",
		"date < date('{{ segmentstream_end_date }}')",
	} {
		if !strings.Contains(string(identityModel), want) {
			t.Fatalf("analytics-core identity union model does not contain %q:\n%s", want, string(identityModel))
		}
	}

	identityMart, err := os.ReadFile(filepath.Join("..", "..", "..", "analytics-core", "models", "marts", "identity_keys.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"select distinct",
		"segmentstream_source",
		"daily_first_observed_at",
		"daily_last_observed_at",
		"anonymous_id",
		"key_name",
		"key_value",
		"from {{ ref('int_identity_keys__daily_spans') }}",
	} {
		if !strings.Contains(string(identityMart), want) {
			t.Fatalf("analytics-core identity mart does not contain %q:\n%s", want, string(identityMart))
		}
	}

	dailySpansModel, err := os.ReadFile(filepath.Join("..", "..", "..", "analytics-core", "models", "intermediate", "identity", "keys", "int_identity_keys__daily_spans.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"min(observed_at) as daily_first_observed_at",
		"max(observed_at) as daily_last_observed_at",
		"date = {{ segmentstream_timestamp_to_date('observed_at') }}",
		"group by 1, 2, 3, 4, 5",
	} {
		if !strings.Contains(string(dailySpansModel), want) {
			t.Fatalf("analytics-core identity daily spans model does not contain %q:\n%s", want, string(dailySpansModel))
		}
	}
}

func TestAnalyticsCoreIdentityLinksModelsUseExpectedShape(t *testing.T) {
	configModel, err := os.ReadFile(filepath.Join("..", "..", "..", "analytics-core", "models", "intermediate", "identity", "links", "int_identity_link_key_config.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"segmentstream_identity_link_keys",
		"empty_identity_link_config",
		"key_name",
		"tier",
		"window_days",
		"max_distinct_anonymous_ids",
		"scope",
	} {
		if !strings.Contains(string(configModel), want) {
			t.Fatalf("identity link config model does not contain %q:\n%s", want, string(configModel))
		}
	}

	observationsModel, err := os.ReadFile(filepath.Join("..", "..", "..", "analytics-core", "models", "intermediate", "identity", "links", "int_identity_link_key_observations.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"from {{ ref('identity_keys') }}",
		"inner join identity_link_key_config",
		"when identity_link_key_config.scope = 'source' then identity_keys.segmentstream_source",
		"else '__segmentstream_project__'",
	} {
		if !strings.Contains(string(observationsModel), want) {
			t.Fatalf("identity link observations model does not contain %q:\n%s", want, string(observationsModel))
		}
	}

	candidatesModel, err := os.ReadFile(filepath.Join("..", "..", "..", "analytics-core", "models", "intermediate", "identity", "links", "int_identity_link_candidates.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"count(distinct anonymous_id) as distinct_anonymous_ids",
		"<= identity_link_key_spans.max_distinct_anonymous_ids",
		"source_key_span.anonymous_id < target_key_span.anonymous_id",
		"segmentstream_timestamp_diff_seconds(",
		"<= source_key_span.window_days * 86400",
	} {
		if !strings.Contains(string(candidatesModel), want) {
			t.Fatalf("identity link candidates model does not contain %q:\n%s", want, string(candidatesModel))
		}
	}

	valueSetsModel, err := os.ReadFile(filepath.Join("..", "..", "..", "analytics-core", "models", "intermediate", "identity", "links", "int_identity_link_deterministic_value_sets.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"where tier = 'deterministic'",
		"array_agg(distinct key_value order by key_value)",
		"key_value_set",
	} {
		if !strings.Contains(string(valueSetsModel), want) {
			t.Fatalf("identity link deterministic value sets model does not contain %q:\n%s", want, string(valueSetsModel))
		}
	}

	filteredModel, err := os.ReadFile(filepath.Join("..", "..", "..", "analytics-core", "models", "intermediate", "identity", "links", "int_identity_links__filtered.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"deterministic_conflicts",
		"source_value_sets.key_value_set != target_value_sets.key_value_set",
		"where deterministic_conflicts.anonymous_id_a is null",
		"when first_seen_at_a > first_seen_at_b then anonymous_id_a",
		"when anonymous_id_a > anonymous_id_b then anonymous_id_a",
		"source_first_seen_at",
		"target_first_seen_at",
		"source_first_seen_date",
		"target_first_seen_date",
	} {
		if !strings.Contains(string(filteredModel), want) {
			t.Fatalf("identity links filtered model does not contain %q:\n%s", want, string(filteredModel))
		}
	}

	identityLinksMart, err := os.ReadFile(filepath.Join("..", "..", "..", "analytics-core", "models", "marts", "identity_links.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"select distinct",
		"source_anonymous_id",
		"target_anonymous_id",
		"key_name",
		"key_value",
		"tier",
		"source_first_seen_at",
		"target_first_seen_at",
		"source_first_seen_date",
		"target_first_seen_date",
		"from {{ ref('int_identity_links__filtered') }}",
	} {
		if !strings.Contains(string(identityLinksMart), want) {
			t.Fatalf("identity_links mart does not contain %q:\n%s", want, string(identityLinksMart))
		}
	}

	dbtProject, err := os.ReadFile(filepath.Join("..", "..", "..", "analytics-core", "dbt_project.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"identity_links:",
		"+materialized: table",
		"+cluster_by:",
		"source_anonymous_id",
		"target_anonymous_id",
	} {
		if !strings.Contains(string(dbtProject), want) {
			t.Fatalf("analytics-core dbt_project.yml does not contain %q:\n%s", want, string(dbtProject))
		}
	}
}

func TestAnalyticsCoreIdentitiesModelsUseExpectedGraphShape(t *testing.T) {
	macro, err := os.ReadFile(filepath.Join("..", "..", "..", "analytics-core", "macros", "identity_connected_components.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"segmentstream_identity_connected_components",
		"label-propagation",
		"Unytics BigFunctions",
		"range(max_iterations | int)",
		"connected_component_id",
	} {
		if !strings.Contains(string(macro), want) {
			t.Fatalf("identity connected components macro does not contain %q:\n%s", want, string(macro))
		}
	}

	nodesModel, err := os.ReadFile(filepath.Join("..", "..", "..", "analytics-core", "models", "intermediate", "identity", "graph", "int_identity_graph_nodes.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"from {{ ref('identity_keys') }}",
		"min(daily_first_observed_at) as first_seen_at",
		"segmentstream_timestamp_to_date",
		"group by 1",
	} {
		if !strings.Contains(string(nodesModel), want) {
			t.Fatalf("identity graph nodes model does not contain %q:\n%s", want, string(nodesModel))
		}
	}

	edgesModel, err := os.ReadFile(filepath.Join("..", "..", "..", "analytics-core", "models", "intermediate", "identity", "graph", "int_identity_graph_edges.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"from {{ ref('identity_links') }}",
		"select distinct",
		"source_anonymous_id <= target_anonymous_id",
		"tier in ('deterministic', 'probabilistic')",
	} {
		if !strings.Contains(string(edgesModel), want) {
			t.Fatalf("identity graph edges model does not contain %q:\n%s", want, string(edgesModel))
		}
	}

	deterministicComponentsModel, err := os.ReadFile(filepath.Join("..", "..", "..", "analytics-core", "models", "intermediate", "identity", "graph", "int_identity_graph_deterministic_components.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"segmentstream_identity_graph_max_iterations",
		"where tier = 'deterministic'",
		"segmentstream_identity_connected_components",
		"row_number() over",
		"order by identity_graph_nodes.first_seen_at, connected_components.node_id",
	} {
		if !strings.Contains(string(deterministicComponentsModel), want) {
			t.Fatalf("identity deterministic components model does not contain %q:\n%s", want, string(deterministicComponentsModel))
		}
	}

	allComponentsModel, err := os.ReadFile(filepath.Join("..", "..", "..", "analytics-core", "models", "intermediate", "identity", "graph", "int_identity_graph_all_components.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"segmentstream_identity_graph_max_iterations",
		"segmentstream_identity_connected_components",
		"component_size",
		"identity_id",
	} {
		if !strings.Contains(string(allComponentsModel), want) {
			t.Fatalf("identity all components model does not contain %q:\n%s", want, string(allComponentsModel))
		}
	}

	resolvedModel, err := os.ReadFile(filepath.Join("..", "..", "..", "analytics-core", "models", "intermediate", "identity", "graph", "int_identities__resolved.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"segmentstream_identity_graph_max_component_size",
		"segmentstream_identity_graph_max_deterministic_component_size",
		"valid_deterministic_components",
		"valid_all_components",
		"coalesce(",
	} {
		if !strings.Contains(string(resolvedModel), want) {
			t.Fatalf("identity resolved model does not contain %q:\n%s", want, string(resolvedModel))
		}
	}

	identitiesMart, err := os.ReadFile(filepath.Join("..", "..", "..", "analytics-core", "models", "marts", "identities.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"select distinct",
		"anonymous_id",
		"identity_id",
		"first_seen_at",
		"first_seen_date",
		"from {{ ref('int_identities__resolved') }}",
	} {
		if !strings.Contains(string(identitiesMart), want) {
			t.Fatalf("identities mart does not contain %q:\n%s", want, string(identitiesMart))
		}
	}

	convergenceTest, err := os.ReadFile(filepath.Join("..", "..", "..", "analytics-core", "tests", "assert_identity_graph_connected_components_converged.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"int_identity_graph_edges",
		"int_identity_graph_deterministic_components",
		"int_identity_graph_all_components",
		"source_connected_component_id",
		"target_connected_component_id",
	} {
		if !strings.Contains(string(convergenceTest), want) {
			t.Fatalf("identity graph convergence test does not contain %q:\n%s", want, string(convergenceTest))
		}
	}

	martSchema, err := os.ReadFile(filepath.Join("..", "..", "..", "analytics-core", "models", "marts", "schema.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"- name: identities",
		"export_name: identities",
		"type: identities",
		"- segmentstream_unpartitioned",
		"- unique",
	} {
		if !strings.Contains(string(martSchema), want) {
			t.Fatalf("analytics-core marts schema does not contain %q:\n%s", want, string(martSchema))
		}
	}

	intermediateSchema, err := os.ReadFile(filepath.Join("..", "..", "..", "analytics-core", "models", "intermediate", "identity", "graph", "schema.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"- name: int_identity_graph_deterministic_components",
		"- name: int_identity_graph_all_components",
		"- segmentstream_unpartitioned",
	} {
		if !strings.Contains(string(intermediateSchema), want) {
			t.Fatalf("analytics-core identity intermediate schema does not contain %q:\n%s", want, string(intermediateSchema))
		}
	}

	dbtProject, err := os.ReadFile(filepath.Join("..", "..", "..", "analytics-core", "dbt_project.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"identities:",
		"+materialized: table",
		"identity_id",
		"anonymous_id",
	} {
		if !strings.Contains(string(dbtProject), want) {
			t.Fatalf("analytics-core dbt_project.yml does not contain %q:\n%s", want, string(dbtProject))
		}
	}
}

func TestPrepareRemovesStaleRuntimeFiles(t *testing.T) {
	root := t.TempDir()
	withAnalyticsCoreRelease(t, "0.0.20")
	stale := filepath.Join(root, RuntimeDirName, "stale.txt")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Prepare(root, testConfig(), testProvider()); err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale file still exists or stat failed with unexpected error: %v", err)
	}
}

func TestPrepareWritesRuntimeEnvAndStaticComposeFile(t *testing.T) {
	root := t.TempDir()
	withAnalyticsCoreRelease(t, "0.0.20")

	if err := Prepare(root, testConfig(), testProvider()); err != nil {
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
	if strings.Contains(string(compose), AnalyticsCoreLocalPathEnv) {
		t.Fatalf("docker-compose.yml should not contain analytics-core local override:\n%s", string(compose))
	}
	for _, want := range []string{
		"postgres:",
		"image: postgres:16-alpine",
		"condition: service_healthy",
		"env_file:",
		"- .env",
		"DAGSTER_HOME: /workspace/.segmentstream",
		"DAGSTER_PG_HOST: postgres",
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
		`SEGMENTSTREAM_ANALYTICS_CORE_REVISION="0.0.20"`,
		`GOOGLE_APPLICATION_CREDENTIALS="/home/segmentstream/.segmentstream/bigquery/production-bigquery.json"`,
		`SEGMENTSTREAM_BQ_PROJECT="example-project"`,
		`SEGMENTSTREAM_BQ_DATASET="segmentstream"`,
		`SEGMENTSTREAM_BQ_LOCATION="US"`,
	} {
		if !strings.Contains(string(env), want) {
			t.Fatalf(".env does not contain %q:\n%s", want, string(env))
		}
	}
	if _, err := os.Stat(filepath.Join(root, RuntimeDirName, analyticsCoreComposeOverrideFile)); !os.IsNotExist(err) {
		t.Fatalf("analytics-core override should not exist for release mode, stat error = %v", err)
	}
}

func TestPrepareRequiresLocalAnalyticsCorePathForDevBuild(t *testing.T) {
	root := t.TempDir()
	withAnalyticsCoreRelease(t, "dev")

	err := Prepare(root, testConfig(), testProvider())
	if err == nil {
		t.Fatal("expected Prepare to fail")
	}
	if !strings.Contains(err.Error(), AnalyticsCoreLocalPathEnv) ||
		!strings.Contains(err.Error(), "dev SegmentStream CLI build") {
		t.Fatalf("error = %v, want local analytics-core path requirement", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, RuntimeDirName)); !os.IsNotExist(statErr) {
		t.Fatalf("runtime dir should not be created, stat error = %v", statErr)
	}
}

func TestPrepareWritesLocalAnalyticsCoreOverride(t *testing.T) {
	root := t.TempDir()
	withAnalyticsCoreRelease(t, "dev")
	localPath := withLocalAnalyticsCore(t, root)

	if err := Prepare(root, testConfig(), testProvider()); err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	env, err := os.ReadFile(filepath.Join(root, RuntimeDirName, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), AnalyticsCoreLocalPathEnv+"="+strconv.Quote(filepath.ToSlash(localPath))) {
		t.Fatalf(".env does not contain local analytics-core path:\n%s", string(env))
	}
	if strings.Contains(string(env), AnalyticsCoreRevisionEnv) {
		t.Fatalf(".env should not contain analytics-core revision in local mode:\n%s", string(env))
	}

	override, err := os.ReadFile(filepath.Join(root, RuntimeDirName, analyticsCoreComposeOverrideFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`source: "${SEGMENTSTREAM_ANALYTICS_CORE_LOCAL_PATH}"`,
		"target: /opt/segmentstream/analytics-core",
		"read_only: true",
	} {
		if !strings.Contains(string(override), want) {
			t.Fatalf("override does not contain %q:\n%s", want, string(override))
		}
	}
}

func TestPrepareRejectsInvalidLocalAnalyticsCorePath(t *testing.T) {
	root := t.TempDir()
	withAnalyticsCoreRelease(t, "dev")
	t.Setenv(AnalyticsCoreLocalPathEnv, filepath.Join(root, "missing"))

	err := Prepare(root, testConfig(), testProvider())
	if err == nil {
		t.Fatal("expected Prepare to fail")
	}
	if !strings.Contains(err.Error(), AnalyticsCoreLocalPathEnv) ||
		!strings.Contains(err.Error(), "not accessible") {
		t.Fatalf("error = %v, want invalid local analytics-core path", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, RuntimeDirName)); !os.IsNotExist(statErr) {
		t.Fatalf("runtime dir should not be created, stat error = %v", statErr)
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

func withAnalyticsCoreRelease(t *testing.T, release string) {
	t.Helper()
	t.Setenv(AnalyticsCoreLocalPathEnv, "")
	previous := currentVersion
	currentVersion = func() version.Info {
		return version.Info{Version: release}
	}
	t.Cleanup(func() {
		currentVersion = previous
	})
}

func withLocalAnalyticsCore(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "analytics-core")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "dbt_project.yml"), []byte("name: segmentstream_analytics_core\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(AnalyticsCoreLocalPathEnv, path)
	return path
}
