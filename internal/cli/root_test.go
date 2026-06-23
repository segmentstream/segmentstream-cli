package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/segmentstream/segmentstream-cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/internal/credentials"
	"github.com/segmentstream/segmentstream-cli/internal/project"
	sourcepkg "github.com/segmentstream/segmentstream-cli/internal/source"
	"github.com/segmentstream/segmentstream-cli/internal/warehouse/bigquery"
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

func TestVersionCommandJSONOutput(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"version", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("version --json command failed: %v", err)
	}

	var result struct {
		SchemaVersion string `json:"schema_version"`
		Command       string `json:"command"`
		Status        string `json:"status"`
		Data          struct {
			Version string `json:"version"`
			Commit  string `json:"commit"`
			Date    string `json:"date"`
			OS      string `json:"os"`
			Arch    string `json:"arch"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("version --json output is not JSON: %v\n%s", err, out.String())
	}
	if result.SchemaVersion != cliresult.SchemaVersion ||
		result.Command != "version" ||
		result.Status != string(cliresult.StatusOK) ||
		result.Data.Version == "" ||
		result.Data.OS == "" ||
		result.Data.Arch == "" {
		t.Fatalf("result = %+v, want structured version response", result)
	}
	if errOut.String() != "" {
		t.Fatalf("stderr = %q, want empty", errOut.String())
	}
}

type testJSONResponse struct {
	SchemaVersion string          `json:"schema_version"`
	Command       string          `json:"command"`
	Status        string          `json:"status"`
	Data          json.RawMessage `json:"data"`
}

func decodeJSONResponseData(t *testing.T, output []byte, data any) testJSONResponse {
	t.Helper()
	var response testJSONResponse
	if err := json.Unmarshal(output, &response); err != nil {
		t.Fatalf("structured JSON output is not JSON: %v\n%s", err, string(output))
	}
	if response.SchemaVersion != cliresult.SchemaVersion {
		t.Fatalf("schema version = %q, want %q", response.SchemaVersion, cliresult.SchemaVersion)
	}
	if data != nil {
		if err := json.Unmarshal(response.Data, data); err != nil {
			t.Fatalf("structured JSON data is not expected shape: %v\n%s", err, string(output))
		}
	}
	return response
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

func TestMainReturnsJSONErrorWhenJSONFlagIsSet(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := Main([]string{"--json", "does-not-exist"}, &out, &errOut)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if errOut.String() != "" {
		t.Fatalf("stderr = %q, want empty", errOut.String())
	}
	var response testJSONResponse
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("json error output is not JSON: %v\n%s", err, out.String())
	}
	if response.Status != string(cliresult.StatusError) ||
		!strings.Contains(string(response.Data)+out.String(), "unknown command") {
		t.Fatalf("response = %+v output=%q, want unknown command JSON error", response, out.String())
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

	var data initResponseData
	decodeJSONResponseData(t, out.Bytes(), &data)
	envelope := data.Envelope
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

	var data initResponseData
	decodeJSONResponseData(t, out.Bytes(), &data)
	envelope := data.Envelope
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
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery", "default-bigquery"); err != nil {
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
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery", "default-bigquery"); err != nil {
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

	var data initResponseData
	decodeJSONResponseData(t, out.Bytes(), &data)
	envelope := data.Envelope
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

	var data initResponseData
	decodeJSONResponseData(t, out.Bytes(), &data)
	envelope := data.Envelope
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
sources:
  - name: ga4
    path: ./sources/ga4
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

	var data initResponseData
	decodeJSONResponseData(t, out.Bytes(), &data)
	envelope := data.Envelope
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
sources:
  - name: ga4
    path: ./sources/ga4
`)
	if _, err := sourcepkg.Create(root, "ga4", "events"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := sourcepkg.SavePassing(root, project.Source{Name: "ga4", Path: "./sources/ga4"}, "2026-06-16", "2026-06-23", time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	writeNamedCredential(t, home, "default-bigquery")
	if err := bigquery.NewConnector().SaveAccessMarker(credentials.Store{HomeDir: home}, "default-bigquery", project.Warehouse{
		Type:     "bigquery",
		Auth:     "default-bigquery",
		Project:  "example-project",
		Dataset:  "segmentstream",
		Location: "EU",
	}); err != nil {
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

	var data initResponseData
	decodeJSONResponseData(t, out.Bytes(), &data)
	envelope := data.Envelope
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
	if accept.Method != "warehouse_config" || accept.Command != "segmentstream warehouse configure" || len(accept.Inputs) != 4 {
		t.Fatalf("accept = %+v, want warehouse configure inputs", accept)
	}
	want := []struct {
		name      string
		inputType string
		flag      string
		required  bool
	}{
		{name: "project", inputType: "string", flag: "--project", required: true},
		{name: "dataset", inputType: "string", flag: "--dataset", required: true},
		{name: "location", inputType: "string", flag: "--location", required: true},
		{name: "create_dataset", inputType: "boolean", flag: "--create-dataset", required: false},
	}
	for i, wantInput := range want {
		input := accept.Inputs[i]
		if input.Name != wantInput.name ||
			input.Type != wantInput.inputType ||
			input.Flag != wantInput.flag ||
			input.Label == "" ||
			input.Required != wantInput.required {
			t.Fatalf("input[%d] = %+v, want %+v", i, input, wantInput)
		}
	}
}

func writeNamedCredential(t *testing.T, home, name string) {
	t.Helper()
	path, err := (credentials.Store{HomeDir: home}).CredentialPath("bigquery", name)
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
