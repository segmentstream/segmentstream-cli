package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/segmentstream/segmentstream-cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/internal/credentials"
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

	if code != cliresult.ExitNeedsUserDecision {
		t.Fatalf("exit code = %d, want %d", code, cliresult.ExitNeedsUserDecision)
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
	if envelope.NextAction.Type != "ask_user" {
		t.Fatalf("next action = %+v, want ask_user", envelope.NextAction)
	}
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

	if err == nil {
		t.Fatal("expected init to return needs auth")
	}
	if cliresult.ExitCode(err) != cliresult.ExitNeedsAuth {
		t.Fatalf("exit code = %d, want %d; stderr=%q", cliresult.ExitCode(err), cliresult.ExitNeedsAuth, errOut.String())
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

	if err == nil {
		t.Fatal("expected init to return needs auth")
	}
	if cliresult.ExitCode(err) != cliresult.ExitNeedsAuth {
		t.Fatalf("exit code = %d, want %d; stderr=%q", cliresult.ExitCode(err), cliresult.ExitNeedsAuth, errOut.String())
	}
	if strings.Contains(out.String(), "Selected warehouse") {
		t.Fatalf("json output contains human text: %q", out.String())
	}

	var envelope cliresult.Envelope
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("init --warehouse --json output is not JSON: %v\n%s", err, out.String())
	}
	if envelope.NextAction.Command != "segmentstream warehouse auth --service-account-key <path>" {
		t.Fatalf("next action = %+v, want warehouse auth command", envelope.NextAction)
	}

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
	if err == nil {
		t.Fatal("expected init to return needs configuration")
	}
	if cliresult.ExitCode(err) != cliresult.ExitMisconfigured {
		t.Fatalf("exit code = %d, want %d", cliresult.ExitCode(err), cliresult.ExitMisconfigured)
	}
	got := out.String()
	for _, want := range []string{
		"Run: segmentstream warehouse configure --project <project> --dataset <dataset> --location <location>",
		"Hint: Use warehouse browse to discover accessible projects, datasets, and locations before configuring.",
		"segmentstream warehouse browse --json",
		"segmentstream warehouse browse --path <project> --json",
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
	if err == nil {
		t.Fatal("expected init to return needs configuration")
	}
	if cliresult.ExitCode(err) != cliresult.ExitMisconfigured {
		t.Fatalf("exit code = %d, want %d", cliresult.ExitCode(err), cliresult.ExitMisconfigured)
	}

	var envelope cliresult.Envelope
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("init --json output is not JSON: %v\n%s", err, out.String())
	}
	if len(envelope.NextAction.Hints) != 1 {
		t.Fatalf("hints = %+v, want one browse hint", envelope.NextAction.Hints)
	}
	hint := envelope.NextAction.Hints[0]
	if hint.ID != "browse_warehouse_before_configure" {
		t.Fatalf("hint id = %q, want browse_warehouse_before_configure", hint.ID)
	}
	if len(hint.Commands) != 2 ||
		hint.Commands[0] != "segmentstream warehouse browse --json" ||
		hint.Commands[1] != "segmentstream warehouse browse --path <project> --json" {
		t.Fatalf("hint commands = %+v, want browse commands", hint.Commands)
	}
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
	if err == nil {
		t.Fatal("expected init to return needs configuration")
	}
	if cliresult.ExitCode(err) != cliresult.ExitMisconfigured {
		t.Fatalf("exit code = %d, want %d", cliresult.ExitCode(err), cliresult.ExitMisconfigured)
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
	if envelope.NextAction.Command != "segmentstream warehouse configure --project <project> --dataset <dataset> --location <location>" {
		t.Fatalf("next action = %+v, want warehouse configure command", envelope.NextAction)
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
	if err == nil {
		t.Fatal("expected init to return needs auth")
	}
	if cliresult.ExitCode(err) != cliresult.ExitNeedsAuth {
		t.Fatalf("exit code = %d, want %d; stderr=%q", cliresult.ExitCode(err), cliresult.ExitNeedsAuth, errOut.String())
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
	if err == nil {
		t.Fatal("expected init to return needs auth")
	}
	if cliresult.ExitCode(err) != cliresult.ExitNeedsAuth {
		t.Fatalf("exit code = %d, want %d; stderr=%q", cliresult.ExitCode(err), cliresult.ExitNeedsAuth, errOut.String())
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
	if err == nil {
		t.Fatal("expected init to return needs auth")
	}
	if cliresult.ExitCode(err) != cliresult.ExitNeedsAuth {
		t.Fatalf("exit code = %d, want %d; stderr=%q", cliresult.ExitCode(err), cliresult.ExitNeedsAuth, errOut.String())
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
