package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/segmentstream/segmentstream-cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/internal/credentials"
	"github.com/segmentstream/segmentstream-cli/internal/googleoauth"
	"github.com/segmentstream/segmentstream-cli/internal/project"
	"github.com/segmentstream/segmentstream-cli/internal/warehouse"
)

func TestVersionCommand(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"segmentstream ",
		"commit: ",
		"date: ",
		"os/arch: ",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("version output %q does not contain %q", got, want)
		}
	}
}

func TestMainReturnsErrorCodeAndPrintsError(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := Main([]string{"does-not-exist"}, &out, &errOut)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "unknown command") {
		t.Fatalf("stderr = %q, want unknown command error", errOut.String())
	}
}

func TestInitJSONIsReadOnlyWhenWarehouseIsMissing(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Main([]string{"init", "--json"}, &out, &errOut)

	if code != cliresult.ExitReady {
		t.Fatalf("exit code = %d, want %d", code, cliresult.ExitReady)
	}
	if errOut.String() != "" {
		t.Fatalf("stderr = %q, want empty", errOut.String())
	}
	assertFileMissing(t, filepath.Join(root, "segmentstream.yml"))

	var envelope cliresult.Envelope
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("init --json output is not JSON: %v\n%s", err, out.String())
	}
	if envelope.SchemaVersion != cliresult.SchemaVersion || envelope.Ready {
		t.Fatalf("envelope = %+v, want schema version and not ready", envelope)
	}
	assertInitEnvelopeV2(t, envelope)
	assertWarehouseTypeNextAction(t, envelope.NextAction)
}

func TestInitWarehouseSelectsBigQueryAndScaffoldsDocs(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials: credentials.Store{HomeDir: filepath.Join(root, "home")},
	})
	cmd.SetArgs([]string{"init", "--warehouse", "bigquery"})
	err := cmd.Execute()

	if err != nil {
		t.Fatalf("init failed: %v; stderr=%q", err, errOut.String())
	}

	assertFileExists(t, filepath.Join(root, "segmentstream.yml"))
	assertFileExists(t, filepath.Join(root, "README.md"))
	assertFileExists(t, filepath.Join(root, "AGENTS.md"))
	assertFileMissing(t, filepath.Join(root, ".segmentstream", "docker-compose.yml"))

	gitignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gitignore), ".segmentstream/") {
		t.Fatalf(".gitignore = %q, want .segmentstream entry", string(gitignore))
	}

	config, _, err := (project.Store{Root: root}).LoadPartial()
	if err != nil {
		t.Fatal(err)
	}
	if config.Warehouse.Type != "bigquery" || config.Warehouse.Auth != "default-bigquery" {
		t.Fatalf("warehouse = %+v, want selected bigquery default auth", config.Warehouse)
	}
	if config.Warehouse.Project != "" || config.Warehouse.Dataset != "" {
		t.Fatalf("warehouse contains placeholders: %+v", config.Warehouse)
	}
}

func TestInitWarehouseJSONSelectsBigQueryAndReturnsEnvelope(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials: credentials.Store{HomeDir: filepath.Join(root, "home")},
	})
	cmd.SetArgs([]string{"init", "--warehouse", "bigquery", "--json"})
	err := cmd.Execute()

	if err != nil {
		t.Fatalf("init failed: %v; stderr=%q", err, errOut.String())
	}
	if strings.Contains(out.String(), "Selected warehouse") {
		t.Fatalf("json output contains human text: %q", out.String())
	}

	var envelope cliresult.Envelope
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("init --warehouse --json output is not JSON: %v\n%s", err, out.String())
	}
	assertInitEnvelopeV2(t, envelope)
	assertWarehouseAuthNextAction(t, envelope.NextAction)

	config, _, err := (project.Store{Root: root}).LoadPartial()
	if err != nil {
		t.Fatal(err)
	}
	if config.Warehouse.Type != "bigquery" || config.Warehouse.Auth != "default-bigquery" {
		t.Fatalf("warehouse = %+v, want selected bigquery default auth", config.Warehouse)
	}
}

func TestInitShowsBrowseHintWhenWarehouseConfigIsMissing(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	home := filepath.Join(root, "home")
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery"); err != nil {
		t.Fatal(err)
	}
	writeNamedCredential(t, home, "default-bigquery")

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials: credentials.Store{HomeDir: home},
	})
	cmd.SetArgs([]string{"init"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"Next action: human_input",
		"Option: Configure BigQuery warehouse",
		"Command: segmentstream warehouse configure",
		"Input: Google Cloud project ID (string, --project, required)",
		"Input: BigQuery dataset ID (string, --dataset, required)",
		"Input: BigQuery dataset location (string, --location, required)",
		"Verify: segmentstream init --json",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("init output = %q, want %q", got, want)
		}
	}
}

func TestInitJSONIncludesBrowseHintWhenWarehouseConfigIsMissing(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	home := filepath.Join(root, "home")
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery"); err != nil {
		t.Fatal(err)
	}
	writeNamedCredential(t, home, "default-bigquery")

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials: credentials.Store{HomeDir: home},
	})
	cmd.SetArgs([]string{"init", "--json"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	var envelope cliresult.Envelope
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("init --json output is not JSON: %v\n%s", err, out.String())
	}
	assertInitEnvelopeV2(t, envelope)
	assertWarehouseConfigNextAction(t, envelope.NextAction)
}

func TestInitJSONDoesNotMutateExistingPartialConfig(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	home := filepath.Join(root, "home")
	configPath := filepath.Join(root, project.ConfigFileName)
	writeConfig(t, root, `version: 1
warehouse:
  type: bigquery
  auth: default-bigquery
`)
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	writeNamedCredential(t, home, "default-bigquery")

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials: credentials.Store{HomeDir: home},
	})
	cmd.SetArgs([]string{"init", "--json"})

	err = cmd.Execute()
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}
	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("init --json mutated config:\nbefore:\n%s\nafter:\n%s", string(before), string(after))
	}

	var envelope cliresult.Envelope
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("init --json output is not JSON: %v\n%s", err, out.String())
	}
	assertInitEnvelopeV2(t, envelope)
	assertWarehouseConfigNextAction(t, envelope.NextAction)
}

func TestInitJSONIncludesWarehouseAccessRunCommandWhenUntested(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	home := filepath.Join(root, "home")
	writeConfig(t, root, `version: 1
warehouse:
  type: bigquery
  auth: default-bigquery
  project: example-project
  dataset: segmentstream
  location: EU
`)
	writeNamedCredential(t, home, "default-bigquery")

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials: credentials.Store{HomeDir: home},
	})
	cmd.SetArgs([]string{"init", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	var envelope cliresult.Envelope
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("init --json output is not JSON: %v\n%s", err, out.String())
	}
	assertInitEnvelopeV2(t, envelope)
	if envelope.NextAction.Type != "run_command" ||
		envelope.NextAction.Stage != "warehouse_access" ||
		envelope.NextAction.Command != "segmentstream warehouse test --json" {
		t.Fatalf("next action = %+v, want warehouse access run command", envelope.NextAction)
	}
}

func TestInitJSONIncludesRunCommandWhenReady(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	home := filepath.Join(root, "home")
	writeConfig(t, root, `version: 1
warehouse:
  type: bigquery
  auth: default-bigquery
  project: example-project
  dataset: segmentstream
  location: EU
`)
	writeNamedCredential(t, home, "default-bigquery")
	if err := (credentials.Store{HomeDir: home}).SaveAccessMarker("default-bigquery", "example-project", "segmentstream", "EU"); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials: credentials.Store{HomeDir: home},
	})
	cmd.SetArgs([]string{"init", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	var envelope cliresult.Envelope
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("init --json output is not JSON: %v\n%s", err, out.String())
	}
	assertInitEnvelopeV2(t, envelope)
	if !envelope.Ready ||
		envelope.NextAction.Type != "run_command" ||
		envelope.NextAction.Stage != "ready" ||
		envelope.NextAction.Command != "segmentstream run" {
		t.Fatalf("envelope = %+v, want ready run command", envelope)
	}
}

func TestInitDoesNotOverwriteExistingConfig(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)

	config := `version: 1
warehouse:
  type: bigquery
  auth: existing-bigquery
  project: existing-project
  dataset: existing_dataset
`
	if err := os.WriteFile(filepath.Join(root, "segmentstream.yml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials: credentials.Store{HomeDir: filepath.Join(root, "home")},
	})
	cmd.SetArgs([]string{"init", "--warehouse", "bigquery"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("init failed: %v; stderr=%q", err, errOut.String())
	}

	data, err := os.ReadFile(filepath.Join(root, "segmentstream.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != config {
		t.Fatalf("segmentstream.yml was overwritten:\n%s", string(data))
	}
}

func TestInitDoesNotOverwriteExistingAgentGuide(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("custom agent guide\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials: credentials.Store{HomeDir: filepath.Join(root, "home")},
	})
	cmd.SetArgs([]string{"init", "--warehouse", "bigquery"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("init failed: %v; stderr=%q", err, errOut.String())
	}

	data, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "custom agent guide\n" {
		t.Fatalf("AGENTS.md was overwritten:\n%s", string(data))
	}
}

func TestInitDoesNotOverwriteExistingReadme(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("custom readme\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials: credentials.Store{HomeDir: filepath.Join(root, "home")},
	})
	cmd.SetArgs([]string{"init", "--warehouse", "bigquery"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("init failed: %v; stderr=%q", err, errOut.String())
	}

	data, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "custom readme\n" {
		t.Fatalf("README.md was overwritten:\n%s", string(data))
	}
}

func TestPrepareCommandIsNotRegistered(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"prepare"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected prepare command to be unavailable")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("error = %v, want unknown command message", err)
	}
}

func TestSourceContractsHumanOutput(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"source", "contracts"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("source contracts command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"Supported source contracts:",
		"events (schema_version: 1, supported, default)",
		"Canonical event stream",
		"segmentstream source contracts --type events",
		"segmentstream source create <name> --type events",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("source contracts output = %q, want %q", got, want)
		}
	}
	for _, notWant := range []string{
		"costs",
		"conversions",
		"events_v1",
	} {
		if strings.Contains(got, notWant) {
			t.Fatalf("source contracts output = %q, did not want %q", got, notWant)
		}
	}
}

func TestSourceContractsJSONOutput(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"source", "contracts", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("source contracts --json failed: %v", err)
	}

	var result struct {
		SchemaVersion string `json:"schema_version"`
		Contracts     []struct {
			Contract struct {
				Type          string `json:"type"`
				SchemaVersion int    `json:"schema_version"`
			} `json:"contract"`
			Default bool `json:"default"`
			Actions []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"actions"`
		} `json:"contracts"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("source contracts --json output is not JSON: %v\n%s", err, out.String())
	}
	if result.SchemaVersion != cliresult.SchemaVersion || len(result.Contracts) != 1 {
		t.Fatalf("result = %+v, want one schema-versioned contract", result)
	}
	contract := result.Contracts[0]
	if contract.Contract.Type != "events" || contract.Contract.SchemaVersion != 1 || !contract.Default {
		t.Fatalf("contract = %+v, want default events/1", contract)
	}
	if len(contract.Actions) != 2 ||
		contract.Actions[0].Command != "segmentstream source contracts --type events --json" ||
		contract.Actions[1].Command != "segmentstream source create <name> --type events --json" {
		t.Fatalf("actions = %+v, want inspect and create actions", contract.Actions)
	}
}

func TestSourceContractsTypeJSONIncludesFullSchema(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"source", "contracts", "--type", "events", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("source contracts --type events --json failed: %v", err)
	}

	var result struct {
		Contract struct {
			Type          string `json:"type"`
			SchemaVersion int    `json:"schema_version"`
		} `json:"contract"`
		Model struct {
			Name      string `json:"name"`
			Partition string `json:"partition"`
		} `json:"model"`
		Columns []struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			Required bool   `json:"required"`
		} `json:"columns"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("source contract detail output is not JSON: %v\n%s", err, out.String())
	}
	if result.Contract.Type != "events" || result.Contract.SchemaVersion != 1 {
		t.Fatalf("contract = %+v, want events/1", result.Contract)
	}
	if result.Model.Name != "events" || result.Model.Partition != "event_date" {
		t.Fatalf("model = %+v, want events partitioned by event_date", result.Model)
	}
	if len(result.Columns) != 7 ||
		result.Columns[0].Name != "event_id" ||
		result.Columns[0].Type != "STRING" ||
		!result.Columns[0].Required ||
		result.Columns[6].Name != "event_date" ||
		!result.Columns[6].Required {
		t.Fatalf("columns = %+v, want events schema", result.Columns)
	}
}

func TestSourceContractsRejectsUnknownType(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"source", "contracts", "--type", "costs"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected source contracts to reject unknown type")
	}
	if !strings.Contains(err.Error(), `unknown source contract type "costs"`) ||
		!strings.Contains(err.Error(), "supported types: events") {
		t.Fatalf("error = %v, want clear unknown type message", err)
	}
}

func TestSourceCreateCreatesLocalSourcePackageJSON(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeConfig(t, root, `version: 1
warehouse:
  type: bigquery
  auth: production-bigquery
  project: example-project
  dataset: segmentstream
`)
	configBefore, err := os.ReadFile(filepath.Join(root, "segmentstream.yml"))
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"source", "create", "ga4", "--type", "events", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("source create command failed: %v", err)
	}

	assertFileExists(t, filepath.Join(root, "sources", "ga4", "contract.yml"))
	assertFileExists(t, filepath.Join(root, "sources", "ga4", "dbt_project.yml"))
	assertFileExists(t, filepath.Join(root, "sources", "ga4", "source.yml"))
	assertFileExists(t, filepath.Join(root, "sources", "ga4", "models", "events.sql"))
	assertFileExists(t, filepath.Join(root, "sources", "ga4", "models", "schema.yml"))
	assertFileMissing(t, filepath.Join(root, "sources", "ga4", "models", "staging"))
	assertFileMissing(t, filepath.Join(root, "sources", "ga4", "models", "exports"))
	assertFileMissing(t, filepath.Join(root, "sources", "ga4", "macros"))
	assertFileMissing(t, filepath.Join(root, "sources", "ga4", "seeds"))
	assertFileMissing(t, filepath.Join(root, "sources", "ga4", "snapshots"))
	assertFileMissing(t, filepath.Join(root, "sources", "ga4", "tests"))

	configAfter, err := os.ReadFile(filepath.Join(root, "segmentstream.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(configAfter) != string(configBefore) {
		t.Fatalf("source create mutated segmentstream.yml:\nbefore:\n%s\nafter:\n%s", string(configBefore), string(configAfter))
	}

	var result struct {
		SchemaVersion string   `json:"schema_version"`
		Directory     string   `json:"directory"`
		CreatedFiles  []string `json:"created_files"`
		Contract      struct {
			Type          string `json:"type"`
			SchemaVersion int    `json:"schema_version"`
		} `json:"contract"`
		Actions []struct {
			Type    string `json:"type"`
			Path    string `json:"path"`
			Snippet string `json:"snippet"`
		} `json:"actions"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("source create --json output is not JSON: %v\n%s", err, out.String())
	}
	if result.Directory != "sources/ga4" {
		t.Fatalf("directory = %q, want sources/ga4", result.Directory)
	}
	if strings.Join(result.CreatedFiles, "\x00") != strings.Join([]string{
		"sources/ga4/contract.yml",
		"sources/ga4/dbt_project.yml",
		"sources/ga4/models/events.sql",
		"sources/ga4/models/schema.yml",
		"sources/ga4/source.yml",
	}, "\x00") {
		t.Fatalf("created files = %+v, want minimal package files", result.CreatedFiles)
	}
	if result.Contract.Type != "events" || result.Contract.SchemaVersion != 1 {
		t.Fatalf("contract = %+v, want events/1", result.Contract)
	}
	if len(result.Actions) != 2 ||
		result.Actions[0].Type != "implement" ||
		result.Actions[0].Path != "sources/ga4/models/events.sql" ||
		result.Actions[1].Type != "tell_user" ||
		!strings.Contains(result.Actions[1].Snippet, "path: ./sources/ga4") {
		t.Fatalf("actions = %+v, want implement and tell_user", result.Actions)
	}
}

func TestSourceCreateRequiresType(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"source", "create", "ga4"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected source create to require --type")
	}
	if !strings.Contains(err.Error(), "--type is required") {
		t.Fatalf("error = %v, want --type requirement", err)
	}
}

func TestSourceInitCreatesLocalSourcePackageFromDefaultContract(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"source", "init", "ga4"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("source init command failed: %v", err)
	}

	assertFileExists(t, filepath.Join(root, "sources", "ga4", "dbt_project.yml"))
	assertFileExists(t, filepath.Join(root, "sources", "ga4", "contract.yml"))
	assertFileExists(t, filepath.Join(root, "sources", "ga4", "source.yml"))
	assertFileExists(t, filepath.Join(root, "sources", "ga4", "models", "events.sql"))
	assertFileExists(t, filepath.Join(root, "sources", "ga4", "models", "schema.yml"))
	assertFileMissing(t, filepath.Join(root, "sources", "ga4", "models", "exports"))
	assertFileMissing(t, filepath.Join(root, "sources", "ga4", "models", "staging"))

	for _, want := range []string{
		`Created source "ga4" at sources/ga4`,
		"Contract: events (schema_version: 1)",
		"Implement:",
		"sources/ga4/models/events.sql",
		"sources:",
		"  - name: ga4",
		"    path: ./sources/ga4",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("source init output = %q, want %q", out.String(), want)
		}
	}
}

func TestSourceInitFailsWhenProjectIsMissing(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"source", "init", "ga4"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected source init to fail")
	}
	if !strings.Contains(err.Error(), "segmentstream.yml was not found") {
		t.Fatalf("error = %v, want missing config message", err)
	}
}

func TestAuthCommandIsNotRegistered(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"auth"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected auth command to be unavailable")
	}
}

func TestWarehouseAuthStoresServiceAccountAndUpdatesConfig(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery"); err != nil {
		t.Fatal(err)
	}
	keyPath := writeServiceAccountKey(t, root)

	var out bytes.Buffer
	var errOut bytes.Buffer
	home := filepath.Join(root, "home")
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials: credentials.Store{HomeDir: home},
	})
	cmd.SetArgs([]string{"warehouse", "auth", "--service-account-key", keyPath, "--name", "production-bigquery", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("warehouse auth failed: %v", err)
	}

	credentialPath, err := (credentials.Store{HomeDir: home}).BigQueryCredentialPath("production-bigquery")
	if err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, credentialPath)

	config, _, err := (project.Store{Root: root}).LoadPartial()
	if err != nil {
		t.Fatal(err)
	}
	if config.Warehouse.Auth != "production-bigquery" {
		t.Fatalf("warehouse.auth = %q, want production-bigquery", config.Warehouse.Auth)
	}
}

func TestWarehouseAuthLoginStoresOAuthCredentialAndUpdatesConfig(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery"); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	home := filepath.Join(root, "home")
	var oauthOptions googleoauth.LoginOptions
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials: credentials.Store{HomeDir: home},
		WarehouseOAuth: func(ctx context.Context, out io.Writer, options googleoauth.LoginOptions) (credentials.GoogleOAuthCredential, error) {
			_ = ctx
			oauthOptions = options
			fmt.Fprintln(out, "fake oauth login")
			return credentials.GoogleOAuthCredential{
				ClientID:     "client-id.apps.googleusercontent.com",
				ClientSecret: "client-secret",
				RefreshToken: "refresh-token",
				TokenURI:     "https://oauth2.googleapis.com/token",
				Scopes:       []string{"https://www.googleapis.com/auth/bigquery"},
			}, nil
		},
	})
	cmd.SetArgs([]string{"warehouse", "auth", "login", "--name", "production-bigquery", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("warehouse auth login failed: %v", err)
	}
	if oauthOptions.Port != 0 {
		t.Fatalf("OAuth port = %d, want default 0", oauthOptions.Port)
	}

	credentialPath, err := (credentials.Store{HomeDir: home}).BigQueryCredentialPath("production-bigquery")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(credentialPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"type": "authorized_user"`,
		`"client_id": "client-id.apps.googleusercontent.com"`,
		`"refresh_token": "refresh-token"`,
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("credential = %s, want %q", string(data), want)
		}
	}

	config, _, err := (project.Store{Root: root}).LoadPartial()
	if err != nil {
		t.Fatal(err)
	}
	if config.Warehouse.Auth != "production-bigquery" {
		t.Fatalf("warehouse.auth = %q, want production-bigquery", config.Warehouse.Auth)
	}
	if !strings.Contains(out.String(), `"method": "oauth"`) {
		t.Fatalf("json output = %s, want oauth method", out.String())
	}
}

func TestWarehouseAuthLoginPassesPortToOAuth(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery"); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	var oauthOptions googleoauth.LoginOptions
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials: credentials.Store{HomeDir: filepath.Join(root, "home")},
		WarehouseOAuth: func(ctx context.Context, out io.Writer, options googleoauth.LoginOptions) (credentials.GoogleOAuthCredential, error) {
			_ = ctx
			_ = out
			oauthOptions = options
			return credentials.GoogleOAuthCredential{
				ClientID:     "client-id.apps.googleusercontent.com",
				ClientSecret: "client-secret",
				RefreshToken: "refresh-token",
				TokenURI:     "https://oauth2.googleapis.com/token",
				Scopes:       []string{"https://www.googleapis.com/auth/bigquery"},
			}, nil
		},
	})
	cmd.SetArgs([]string{"warehouse", "auth", "login", "--port", "40473"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("warehouse auth login failed: %v", err)
	}
	if oauthOptions.Port != 40473 {
		t.Fatalf("OAuth port = %d, want 40473", oauthOptions.Port)
	}
}

func TestWarehouseAuthLoginRejectsInvalidPort(t *testing.T) {
	for _, port := range []string{"-1", "65536"} {
		t.Run(port, func(t *testing.T) {
			var out bytes.Buffer
			var errOut bytes.Buffer
			called := false
			cmd := newRootCommand(&out, &errOut, cliOptions{
				WarehouseOAuth: func(ctx context.Context, out io.Writer, options googleoauth.LoginOptions) (credentials.GoogleOAuthCredential, error) {
					called = true
					return credentials.GoogleOAuthCredential{}, nil
				},
			})
			cmd.SetArgs([]string{"warehouse", "auth", "login", "--port=" + port})

			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "invalid --port") {
				t.Fatalf("error = %v, want invalid --port", err)
			}
			if called {
				t.Fatal("OAuth login was called for invalid port")
			}
		})
	}
}

func TestWarehouseAuthLoginHelpIncludesPort(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{})
	cmd.SetArgs([]string{"warehouse", "auth", "login", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("warehouse auth login --help failed: %v", err)
	}
	if !strings.Contains(out.String(), "--port") {
		t.Fatalf("help output = %s, want --port", out.String())
	}
}

func TestWarehouseConfigureJSONWritesValidConfig(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery"); err != nil {
		t.Fatal(err)
	}
	fake := &fakeWarehouseConnector{
		configureResult: warehouse.NewConfigureResult("bigquery", []warehouse.Validation{
			{ID: "dataset", Field: "warehouse.dataset", Status: "ok"},
			{ID: "location", Field: "warehouse.location", Status: "ok"},
		}, nil),
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials:       credentials.Store{HomeDir: filepath.Join(root, "home")},
		WarehouseRegistry: warehouse.NewRegistry(fake),
	})
	cmd.SetArgs([]string{"warehouse", "configure", "--project", "example-project", "--dataset", "segmentstream_new", "--location", "EU", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("warehouse configure failed: %v", err)
	}
	if fake.config.Project != "example-project" || fake.config.Dataset != "segmentstream_new" || fake.config.Location != "EU" {
		t.Fatalf("connector config = %+v", fake.config)
	}

	config, err := project.LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if config.Warehouse.Project != "example-project" || config.Warehouse.Dataset != "segmentstream_new" || config.Warehouse.Location != "EU" {
		t.Fatalf("saved warehouse = %+v", config.Warehouse)
	}
}

func TestWarehouseBrowseDoesNotRequireConfiguredProject(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	home := filepath.Join(root, "home")
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery"); err != nil {
		t.Fatal(err)
	}
	writeNamedCredential(t, home, "default-bigquery")
	fake := &fakeWarehouseConnector{
		browseResult: warehouse.NewBrowseResult("bigquery", "project", "", []warehouse.BrowseChild{
			{ID: "example-project"},
		}),
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials:       credentials.Store{HomeDir: home},
		WarehouseRegistry: warehouse.NewRegistry(fake),
	})
	cmd.SetArgs([]string{"warehouse", "browse", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("warehouse browse failed: %v", err)
	}
	if fake.browsePath != "" {
		t.Fatalf("browse path = %q, want empty project-list path", fake.browsePath)
	}
	if !strings.Contains(out.String(), `"level": "project"`) || !strings.Contains(out.String(), `"id": "example-project"`) {
		t.Fatalf("browse output = %s, want project result", out.String())
	}
}

func TestWarehouseBrowseTableJSONForwardsPath(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	home := filepath.Join(root, "home")
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery"); err != nil {
		t.Fatal(err)
	}
	writeNamedCredential(t, home, "default-bigquery")
	fake := &fakeWarehouseConnector{
		browseResult: warehouse.NewBrowseResult("bigquery", "table", "example-project/dataset_one", []warehouse.BrowseChild{
			{ID: "events", FriendlyName: "Events", Type: "TABLE"},
		}),
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials:       credentials.Store{HomeDir: home},
		WarehouseRegistry: warehouse.NewRegistry(fake),
	})
	cmd.SetArgs([]string{"warehouse", "browse", "--path", "example-project/dataset_one", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("warehouse browse failed: %v", err)
	}
	if fake.browsePath != "example-project/dataset_one" {
		t.Fatalf("browse path = %q, want table-list path", fake.browsePath)
	}
	var result warehouse.BrowseResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("warehouse browse output is not JSON: %v\n%s", err, out.String())
	}
	if result.Level != "table" ||
		result.Path != "example-project/dataset_one" ||
		len(result.Children) != 1 ||
		result.Children[0].ID != "events" ||
		result.Children[0].Type != "TABLE" {
		t.Fatalf("result = %+v, want table result", result)
	}
}

func TestWarehouseBrowseTableTextOutput(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	home := filepath.Join(root, "home")
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery"); err != nil {
		t.Fatal(err)
	}
	writeNamedCredential(t, home, "default-bigquery")
	fake := &fakeWarehouseConnector{
		browseResult: warehouse.NewBrowseResult("bigquery", "table", "example-project/dataset_one", []warehouse.BrowseChild{
			{ID: "events", FriendlyName: "Events", Type: "TABLE"},
		}),
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials:       credentials.Store{HomeDir: home},
		WarehouseRegistry: warehouse.NewRegistry(fake),
	})
	cmd.SetArgs([]string{"warehouse", "browse", "--path", "example-project/dataset_one"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("warehouse browse failed: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"Tables in example-project/dataset_one:",
		"- events (Events, TABLE)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("warehouse browse output = %q, want %q", got, want)
		}
	}
}

func TestWarehouseBrowseSchemaTextOutput(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	home := filepath.Join(root, "home")
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery"); err != nil {
		t.Fatal(err)
	}
	writeNamedCredential(t, home, "default-bigquery")
	browseResult := warehouse.NewBrowseResult("bigquery", "schema", "example-project/dataset_one/events", []warehouse.BrowseChild{})
	browseResult.Schema = []warehouse.BrowseField{
		{Name: "event_id", Type: "STRING", Mode: "REQUIRED", Description: "Stable event id"},
		{Name: "event_params", Type: "RECORD", Mode: "REPEATED", Fields: []warehouse.BrowseField{
			{Name: "key", Type: "STRING"},
		}},
	}
	fake := &fakeWarehouseConnector{browseResult: browseResult}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials:       credentials.Store{HomeDir: home},
		WarehouseRegistry: warehouse.NewRegistry(fake),
	})
	cmd.SetArgs([]string{"warehouse", "browse", "--path", "example-project/dataset_one/events"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("warehouse browse failed: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"Schema for example-project/dataset_one/events:",
		"- event_id STRING REQUIRED - Stable event id",
		"  - key STRING",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("warehouse browse output = %q, want %q", got, want)
		}
	}
}

func TestWarehouseTestSavesAccessMarker(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeConfig(t, root, `version: 1
warehouse:
  type: bigquery
  auth: production-bigquery
  project: example-project
  dataset: segmentstream
  location: EU
`)
	fake := &fakeWarehouseConnector{
		testResult: warehouse.NewTestResult("bigquery", []warehouse.AccessCheck{
			{ID: "connect", OK: true},
			{ID: "read", OK: true},
			{ID: "create_table", OK: true},
			{ID: "query_in_location", OK: true},
		}),
	}
	home := filepath.Join(root, "home")

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials:       credentials.Store{HomeDir: home},
		WarehouseRegistry: warehouse.NewRegistry(fake),
	})
	cmd.SetArgs([]string{"warehouse", "test", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("warehouse test failed: %v", err)
	}
	verified, err := (credentials.Store{HomeDir: home}).HasMatchingAccessMarker("production-bigquery", "example-project", "segmentstream", "EU")
	if err != nil {
		t.Fatal(err)
	}
	if !verified {
		t.Fatal("expected access marker to be saved")
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
	switch envelope.NextAction.Type {
	case "human_input":
		if envelope.NextAction.Verify != "segmentstream init --json" {
			t.Fatalf("verify = %q, want segmentstream init --json", envelope.NextAction.Verify)
		}
	case "run_command":
		if strings.Contains(envelope.NextAction.Command, "<") {
			t.Fatalf("run command contains placeholder: %+v", envelope.NextAction)
		}
	default:
		t.Fatalf("next action type = %q, want human_input or run_command", envelope.NextAction.Type)
	}
}

func assertWarehouseTypeNextAction(t *testing.T, action cliresult.NextAction) {
	t.Helper()
	if action.Type != "human_input" || action.Stage != "warehouse_type" {
		t.Fatalf("next action = %+v, want warehouse_type human_input", action)
	}
	if len(action.Accepts) != 1 {
		t.Fatalf("accepts = %+v, want one option", action.Accepts)
	}
	accept := action.Accepts[0]
	if accept.Method != "bigquery" ||
		accept.Command != "segmentstream init --warehouse bigquery" ||
		accept.Value != "bigquery" ||
		len(accept.Inputs) != 0 {
		t.Fatalf("accept = %+v, want BigQuery selection", accept)
	}
}

func assertWarehouseAuthNextAction(t *testing.T, action cliresult.NextAction) {
	t.Helper()
	if action.Type != "human_input" || action.Stage != "warehouse_auth" {
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
		t.Fatalf("input = %+v, want required filepath input", input)
	}
}

func assertWarehouseConfigNextAction(t *testing.T, action cliresult.NextAction) {
	t.Helper()
	if action.Type != "human_input" || action.Stage != "warehouse_config" {
		t.Fatalf("next action = %+v, want warehouse_config human_input", action)
	}
	if len(action.Accepts) != 1 {
		t.Fatalf("accepts = %+v, want one config option", action.Accepts)
	}
	accept := action.Accepts[0]
	if accept.Method != "warehouse_config" || accept.Command != "segmentstream warehouse configure" || len(accept.Inputs) != 3 {
		t.Fatalf("accept = %+v, want warehouse configure inputs", accept)
	}
	want := []struct {
		name string
		flag string
	}{
		{name: "project", flag: "--project"},
		{name: "dataset", flag: "--dataset"},
		{name: "location", flag: "--location"},
	}
	for i, wantInput := range want {
		input := accept.Inputs[i]
		if input.Name != wantInput.name ||
			input.Type != "string" ||
			input.Flag != wantInput.flag ||
			input.Label == "" ||
			!input.Required {
			t.Fatalf("input[%d] = %+v, want %+v", i, input, wantInput)
		}
	}
}

type fakeWarehouseConnector struct {
	browseResult    warehouse.BrowseResult
	browsePath      string
	configureResult warehouse.ConfigureResult
	testResult      warehouse.TestResult
	config          project.Warehouse
}

func (connector *fakeWarehouseConnector) Type() string {
	return "bigquery"
}

func (connector *fakeWarehouseConnector) Browse(ctx context.Context, credentialPath string, path string) (warehouse.BrowseResult, error) {
	_ = ctx
	_ = credentialPath
	connector.browsePath = path
	return connector.browseResult, nil
}

func (connector *fakeWarehouseConnector) ValidateConfiguration(ctx context.Context, credentialPath string, config project.Warehouse) (warehouse.ConfigureResult, error) {
	_ = ctx
	_ = credentialPath
	connector.config = config
	return connector.configureResult, nil
}

func (connector *fakeWarehouseConnector) Test(context.Context, string, project.Warehouse) (warehouse.TestResult, error) {
	return connector.testResult, nil
}

func writeServiceAccountKey(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "service-account.json")
	data := `{"type":"service_account","client_email":"test@example.iam.gserviceaccount.com","private_key":"-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----\n"}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeNamedCredential(t *testing.T, home, name string) {
	t.Helper()
	path, err := (credentials.Store{HomeDir: home}).BigQueryCredentialPath(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		t.Fatal(err)
	}
}

func withWorkingDirectory(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %s: %v", path, err)
	}
}
