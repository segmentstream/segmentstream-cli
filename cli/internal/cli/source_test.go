package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/segmentstream/segmentstream-cli/cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/cli/internal/project"
	sourcepkg "github.com/segmentstream/segmentstream-cli/cli/internal/source"
)

func TestSourceContractsJSONOutput(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"source", "contracts", "--type", "events", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("source contracts failed: %v", err)
	}

	var result sourceContractDetailResult
	response := decodeJSONResponseData(t, out.Bytes(), &result)
	if response.Command != "source.contracts" {
		t.Fatalf("command = %q, want source.contracts", response.Command)
	}
	if result.SchemaVersion != cliresult.SchemaVersion || result.Contract.Type != "events" || len(result.Columns) == 0 {
		t.Fatalf("result = %+v, want events contract with columns", result)
	}
}

func TestSourceContractsConversionsJSONOutput(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"source", "contracts", "--type", "conversion_events", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("source contracts failed: %v", err)
	}

	var result sourceContractDetailResult
	response := decodeJSONResponseData(t, out.Bytes(), &result)
	if response.Command != "source.contracts" {
		t.Fatalf("command = %q, want source.contracts", response.Command)
	}
	if result.SchemaVersion != cliresult.SchemaVersion ||
		result.Contract.Type != "conversion_events" ||
		result.Model.Name != "conversion_events" ||
		len(result.Columns) != 5 {
		t.Fatalf("result = %+v, want conversion_events contract with columns", result)
	}
	if result.Columns[4].Name != "conversion_value" || result.Columns[4].Required {
		t.Fatalf("conversion value column = %+v, want optional conversion_value", result.Columns[4])
	}
}

func TestSourceScaffoldReturnsActionableJSON(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"source", "scaffold", "ga4", "--type", "events", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("source scaffold failed: %v", err)
	}

	assertFileMissing(t, filepath.Join(root, "sources", "ga4", "README.md"))

	var result sourceScaffoldResult
	response := decodeJSONResponseData(t, out.Bytes(), &result)
	if response.Command != "source.scaffold" {
		t.Fatalf("command = %q, want source.scaffold", response.Command)
	}
	if !containsString(result.CreatedFiles, "sources/ga4/models/events.sql") ||
		containsString(result.CreatedFiles, "sources/ga4/README.md") {
		t.Fatalf("created files = %+v, want template-derived files without README", result.CreatedFiles)
	}
	if result.Contract.Type != "events" ||
		result.Contract.SchemaVersion != 1 ||
		result.Contract.Model != "events" ||
		result.Contract.Partition != "event_date" ||
		!containsString(result.Contract.RequiredColumns, "event_id") ||
		!containsString(result.Contract.RequiredColumns, "event_date") ||
		len(result.Contract.Columns) != 7 {
		t.Fatalf("contract = %+v, want events contract summary", result.Contract)
	}
	if len(result.Unresolved) != 2 ||
		result.Unresolved[0].ID != "raw_source_binding" ||
		result.Unresolved[0].Path != "sources/ga4/models/schema.yml" ||
		result.Unresolved[0].Marker != "SEGMENTSTREAM_TODO(raw_source_binding)" ||
		result.Unresolved[1].ID != "model_mapping" ||
		result.Unresolved[1].Path != "sources/ga4/models/events.sql" ||
		result.Unresolved[1].Marker != "SEGMENTSTREAM_TODO(model_mapping)" {
		t.Fatalf("unresolved = %+v, want raw binding and model mapping items", result.Unresolved)
	}
	if result.Verify.Command != "segmentstream source verify ga4 --json" {
		t.Fatalf("verify = %+v, want source verify command", result.Verify)
	}
}

func TestSourceScaffoldRequiresType(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"source", "scaffold", "ga4"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected source scaffold to require --type")
	}
}

func TestSourceVerifyRunsTemplateDbtTestsInDocker(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)
	if _, err := sourcepkg.Create(root, "ga4", "events"); err != nil {
		t.Fatal(err)
	}
	withCurrentTime(t, time.Date(2026, 6, 22, 10, 30, 0, 0, time.UTC))

	runner := &stubCommandRunner{
		results: []stubCommandResult{
			{output: "docker info"},
			{output: "Docker Compose version v2.32.0"},
			{output: "compose built"},
			{output: "deps done"},
			{output: "tests passed"},
		},
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
	cmd.SetArgs([]string{"source", "verify", "ga4", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("source verify failed: %v", err)
	}

	runtimeDir := filepath.Join(root, ".segmentstream")
	if len(runner.calls) != 5 {
		t.Fatalf("docker calls = %v, want 5 calls", runner.calls)
	}
	assertCommand(t, runner.calls[0], "docker", []string{"info", "--format", "{{json .ServerVersion}}"}, "")
	assertCommand(t, runner.calls[1], "docker", []string{"compose", "version"}, "")
	assertCommand(t, runner.calls[2], "docker", []string{"compose", "build", "segmentstream"}, runtimeDir)
	assertCommand(t, runner.calls[3], "docker", []string{
		"compose", "run", "--rm", "--no-deps", "segmentstream",
		"dbt", "deps",
		"--project-dir", "/workspace/sources/ga4",
		"--profiles-dir", "/workspace/.segmentstream",
	}, runtimeDir)
	testArgs := runner.calls[4].Args
	for _, want := range []string{
		"compose",
		"run",
		"dbt",
		"test",
		"--project-dir",
		"/workspace/sources/ga4",
		"--profiles-dir",
		"/workspace/.segmentstream",
		"--select",
		"tag:segmentstream_source_verify",
		`"segmentstream_start_date":"2026-06-16"`,
		`"segmentstream_end_date":"2026-06-23"`,
	} {
		if !strings.Contains(strings.Join(testArgs, " "), want) {
			t.Fatalf("dbt test args = %v, want %q", testArgs, want)
		}
	}

	var result sourceVerifyResult
	response := decodeJSONResponseData(t, out.Bytes(), &result)
	if response.Command != "source.verify" {
		t.Fatalf("command = %q, want source.verify", response.Command)
	}
	if result.Status != "passed" ||
		result.Source != "ga4" ||
		result.StartDate != "2026-06-16" ||
		result.EndExclusiveDate != "2026-06-23" ||
		result.Fingerprint == "" {
		t.Fatalf("result = %+v, want passed verification window", result)
	}
	assertFileExists(t, filepath.Join(root, "sources", "ga4", ".segmentstream", "verification.json"))
	status, err := sourcepkg.Check(root, project.Source{Name: "ga4", Path: "./sources/ga4"})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !status.Valid {
		t.Fatalf("status = %+v, want valid marker", status)
	}
}

func TestSourceVerifyJSONFailsWhenDockerCLIIsMissing(t *testing.T) {
	root := writeSourceVerifyPreflightProject(t)
	withWorkingDirectory(t, root)

	runner := &stubCommandRunner{lookPathErr: exec.ErrNotFound}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
	cmd.SetArgs([]string{"source", "verify", "ga4", "--json"})

	err := cmd.Execute()
	if cliresult.ExitCode(err) != cliresult.ExitGenericError {
		t.Fatalf("exit code = %d, want %d; err=%v", cliresult.ExitCode(err), cliresult.ExitGenericError, err)
	}
	if !strings.Contains(out.String(), "Docker is required to run source verification") {
		t.Fatalf("json output = %s, want missing Docker diagnostic", out.String())
	}
	if len(runner.calls) != 0 {
		t.Fatalf("docker commands were run even though docker is missing: %v", runner.calls)
	}
}

func TestSourceVerifyJSONFailsWhenDockerEngineIsUnavailable(t *testing.T) {
	root := writeSourceVerifyPreflightProject(t)
	withWorkingDirectory(t, root)

	runner := &stubCommandRunner{
		results: []stubCommandResult{
			{output: "Cannot connect to Docker daemon", err: errors.New("docker info failed")},
		},
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
	cmd.SetArgs([]string{"source", "verify", "ga4", "--json"})

	err := cmd.Execute()
	if cliresult.ExitCode(err) != cliresult.ExitGenericError {
		t.Fatalf("exit code = %d, want %d; err=%v", cliresult.ExitCode(err), cliresult.ExitGenericError, err)
	}
	if !strings.Contains(out.String(), "Docker Engine is not running") ||
		!strings.Contains(out.String(), "Cannot connect to Docker daemon") {
		t.Fatalf("json output = %s, want Docker Engine diagnostic", out.String())
	}
	if len(runner.calls) != 1 {
		t.Fatalf("docker calls = %v, want docker info only", runner.calls)
	}
	assertCommand(t, runner.calls[0], "docker", []string{"info", "--format", "{{json .ServerVersion}}"}, "")
}

func TestSourceVerifyJSONFailsWhenDockerComposeIsUnavailable(t *testing.T) {
	root := writeSourceVerifyPreflightProject(t)
	withWorkingDirectory(t, root)

	runner := &stubCommandRunner{
		results: []stubCommandResult{
			{output: "docker info"},
			{output: "docker: 'compose' is not a docker command", err: errors.New("exit status 1")},
		},
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{CommandRunner: runner})
	cmd.SetArgs([]string{"source", "verify", "ga4", "--json"})

	err := cmd.Execute()
	if cliresult.ExitCode(err) != cliresult.ExitGenericError {
		t.Fatalf("exit code = %d, want %d; err=%v", cliresult.ExitCode(err), cliresult.ExitGenericError, err)
	}
	if !strings.Contains(out.String(), "Docker Compose V2 is required") ||
		!strings.Contains(out.String(), "not a docker command") {
		t.Fatalf("json output = %s, want Docker Compose diagnostic", out.String())
	}
	if len(runner.calls) != 2 {
		t.Fatalf("docker calls = %v, want docker info and compose version", runner.calls)
	}
	assertCommand(t, runner.calls[1], "docker", []string{"compose", "version"}, "")
}

func TestSourceVerifyJSONIncludesContractMigrationGuide(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeConfig(t, root, `version: 1
warehouse:
  type: bigquery
  auth: production-bigquery
  project: example-project
  dataset: segmentstream
sources:
  - name: sdk_identity
    path: ./sources/sdk_identity
`)
	sourcePath := filepath.Join(root, "sources", "sdk_identity")
	if err := os.MkdirAll(sourcePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "contract.yml"), []byte(`type: identity_keys
schema_version: 1
model:
  name: identity_keys
  partition: date
`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Main([]string{"source", "verify", "sdk_identity", "--json"}, &out, &errOut)

	if code != cliresult.ExitGenericError {
		t.Fatalf("exit code = %d, want %d", code, cliresult.ExitGenericError)
	}
	if errOut.String() != "" {
		t.Fatalf("stderr = %q, want empty", errOut.String())
	}
	var result sourcepkg.ContractMigrationRequiredError
	response := decodeJSONResponseData(t, out.Bytes(), &result)
	if response.Command != "source.verify" || response.Status != string(cliresult.StatusError) {
		t.Fatalf("response = %+v, want source.verify error", response)
	}
	if result.ContractType != "identity_keys" ||
		result.FromSchemaVersion != 1 ||
		result.ToSchemaVersion != 2 ||
		result.SourceName != "sdk_identity" ||
		!strings.Contains(result.MigrationGuide, "observed_at") ||
		!strings.Contains(result.MigrationGuide, "models/identity_keys.sql") ||
		result.NextCommand != "segmentstream source verify sdk_identity" {
		t.Fatalf("migration data = %+v, want actionable migration guide", result)
	}
	if !strings.Contains(out.String(), "source_contract_migration_required") ||
		!strings.Contains(out.String(), "Apply the migration guide") {
		t.Fatalf("json output = %s, want structured migration diagnostic", out.String())
	}
}

func writeSourceVerifyPreflightProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeValidConfig(t, root)
	if _, err := sourcepkg.Create(root, "ga4", "events"); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestSourceRejectsRemovedSubcommands(t *testing.T) {
	for _, args := range [][]string{
		{"source", "create", "ga4"},
		{"source", "init", "ga4"},
	} {
		var out bytes.Buffer
		var errOut bytes.Buffer
		cmd := NewRootCommand(&out, &errOut)
		cmd.SetArgs(args)

		err := cmd.Execute()
		if err == nil {
			t.Fatalf("expected %v to fail", args)
		}
		if !strings.Contains(err.Error(), "unknown source command") {
			t.Fatalf("error = %v, want unknown source command", err)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
