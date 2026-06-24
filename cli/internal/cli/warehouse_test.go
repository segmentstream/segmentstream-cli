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
	"time"

	"github.com/segmentstream/segmentstream-cli/cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/cli/internal/credentials"
	"github.com/segmentstream/segmentstream-cli/cli/internal/project"
	"github.com/segmentstream/segmentstream-cli/cli/internal/warehouse"
)

func TestWarehouseAuthStoresServiceAccountAndUpdatesConfig(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery", "default-bigquery"); err != nil {
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

	credentialPath, err := (credentials.Store{HomeDir: home}).CredentialPath("bigquery", "production-bigquery")
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
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery", "default-bigquery"); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	home := filepath.Join(root, "home")
	var oauthOptions warehouse.LoginOptions
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials: credentials.Store{HomeDir: home},
		WarehouseOAuth: func(ctx context.Context, out io.Writer, options warehouse.LoginOptions) ([]byte, error) {
			_ = ctx
			oauthOptions = options
			fmt.Fprintln(out, "fake oauth login")
			return []byte(`{
  "type": "authorized_user",
  "client_id": "client-id.apps.googleusercontent.com",
  "client_secret": "client-secret",
  "refresh_token": "refresh-token",
  "token_uri": "https://oauth2.googleapis.com/token",
  "scopes": ["https://www.googleapis.com/auth/bigquery"]
}`), nil
		},
	})
	cmd.SetArgs([]string{"warehouse", "auth", "login", "--name", "production-bigquery", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("warehouse auth login failed: %v", err)
	}
	if oauthOptions.Port != 0 {
		t.Fatalf("OAuth port = %d, want default 0", oauthOptions.Port)
	}

	credentialPath, err := (credentials.Store{HomeDir: home}).CredentialPath("bigquery", "production-bigquery")
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
	var authResponse struct {
		Data warehouseAuthResult `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &authResponse); err != nil {
		t.Fatalf("parse auth json output: %v\n%s", err, out.String())
	}
	if authResponse.Data.Method != "oauth" {
		t.Fatalf("auth method = %q, want oauth", authResponse.Data.Method)
	}
}

func TestWarehouseAuthLoginPassesPortToOAuth(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery", "default-bigquery"); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	var oauthOptions warehouse.LoginOptions
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials: credentials.Store{HomeDir: filepath.Join(root, "home")},
		WarehouseOAuth: func(ctx context.Context, out io.Writer, options warehouse.LoginOptions) ([]byte, error) {
			_ = ctx
			_ = out
			oauthOptions = options
			return []byte(`{"type":"authorized_user","client_id":"client-id.apps.googleusercontent.com","client_secret":"client-secret","refresh_token":"refresh-token","token_uri":"https://oauth2.googleapis.com/token"}`), nil
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
				WarehouseOAuth: func(ctx context.Context, out io.Writer, options warehouse.LoginOptions) ([]byte, error) {
					called = true
					return nil, nil
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
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery", "default-bigquery"); err != nil {
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

func TestWarehouseConfigureCreateDatasetJSONForwardsOptionAndWritesCreatedResult(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery", "default-bigquery"); err != nil {
		t.Fatal(err)
	}
	fake := &fakeWarehouseConnector{
		configureResult: warehouse.NewConfigureResult("bigquery", []warehouse.Validation{
			{ID: "dataset_exists", Field: "warehouse.dataset", Status: "created", Message: "Created dataset example-project:segmentstream_new in EU."},
		}, nil),
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials:       credentials.Store{HomeDir: filepath.Join(root, "home")},
		WarehouseRegistry: warehouse.NewRegistry(fake),
	})
	cmd.SetArgs([]string{"warehouse", "configure", "--project", "example-project", "--dataset", "segmentstream_new", "--location", "EU", "--create-dataset", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("warehouse configure failed: %v", err)
	}
	if !fake.configureOptions.CreateDataset {
		t.Fatal("CreateDataset option was not forwarded")
	}
	config, err := project.LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if config.Warehouse.Project != "example-project" || config.Warehouse.Dataset != "segmentstream_new" || config.Warehouse.Location != "EU" {
		t.Fatalf("saved warehouse = %+v", config.Warehouse)
	}
	var result warehouse.ConfigureResult
	decodeJSONResponseData(t, out.Bytes(), &result)
	if !hasWarehouseValidation(result.Validations, "dataset_exists", "created") {
		t.Fatalf("validations = %+v, want created dataset validation", result.Validations)
	}
}

func TestWarehouseConfigureCreatedTextOutput(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery", "default-bigquery"); err != nil {
		t.Fatal(err)
	}
	fake := &fakeWarehouseConnector{
		configureResult: warehouse.NewConfigureResult("bigquery", []warehouse.Validation{
			{ID: "dataset_exists", Field: "warehouse.dataset", Status: "created", Message: "Created dataset example-project:segmentstream_new in EU."},
		}, nil),
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials:       credentials.Store{HomeDir: filepath.Join(root, "home")},
		WarehouseRegistry: warehouse.NewRegistry(fake),
	})
	cmd.SetArgs([]string{"warehouse", "configure", "--project", "example-project", "--dataset", "segmentstream_new", "--location", "EU", "--create-dataset"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("warehouse configure failed: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"Warehouse configuration is valid.",
		"Created dataset example-project:segmentstream_new in EU.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("warehouse configure output = %q, want %q", got, want)
		}
	}
}

func TestWarehouseConfigureMissingDatasetDoesNotSaveConfig(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	store := project.Store{Root: root}
	if _, err := store.SelectWarehouse("bigquery", "default-bigquery"); err != nil {
		t.Fatal(err)
	}
	fake := &fakeWarehouseConnector{
		configureResult: warehouse.NewConfigureResult("bigquery", []warehouse.Validation{
			{ID: "dataset_exists", Field: "warehouse.dataset", Status: "not_found", Message: "Dataset example-project:missing_dataset does not exist in EU."},
		}, []cliresult.Diagnostic{
			{
				ID:         "missing_dataset",
				Field:      "warehouse.dataset",
				Message:    "Dataset example-project:missing_dataset does not exist in EU.",
				Suggestion: "segmentstream warehouse configure --project example-project --dataset missing_dataset --location EU --create-dataset",
			},
		}),
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials:       credentials.Store{HomeDir: filepath.Join(root, "home")},
		WarehouseRegistry: warehouse.NewRegistry(fake),
	})
	cmd.SetArgs([]string{"warehouse", "configure", "--project", "example-project", "--dataset", "missing_dataset", "--location", "EU"})

	err := cmd.Execute()
	if cliresult.ExitCode(err) != cliresult.ExitMisconfigured {
		t.Fatalf("exit code = %d, want %d; err = %v", cliresult.ExitCode(err), cliresult.ExitMisconfigured, err)
	}
	config, exists, loadErr := store.LoadPartial()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if !exists {
		t.Fatal("segmentstream.yml was removed")
	}
	if config.Warehouse.Project != "" || config.Warehouse.Dataset != "" || config.Warehouse.Location != "" {
		t.Fatalf("saved warehouse = %+v, want project/dataset/location unchanged", config.Warehouse)
	}
	got := out.String()
	for _, want := range []string{
		"Warehouse configuration is invalid.",
		"Next action: segmentstream warehouse configure --project example-project --dataset missing_dataset --location EU --create-dataset",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("warehouse configure output = %q, want %q", got, want)
		}
	}
}

func TestWarehouseConfigureHelpIncludesCreateDataset(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{})
	cmd.SetArgs([]string{"warehouse", "configure", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("warehouse configure --help failed: %v", err)
	}
	if !strings.Contains(out.String(), "--create-dataset") {
		t.Fatalf("help output = %s, want --create-dataset", out.String())
	}
}

func TestWarehouseDestroyJSONClearsConfiguredDatasetAndAccessMarker(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	home := filepath.Join(root, "home")
	writeConfig(t, root, `version: 1
warehouse:
  type: bigquery
  auth: production-bigquery
  project: example-project
  dataset: segmentstream
  location: EU
`)
	fake := &fakeWarehouseConnector{
		destroyResult: warehouse.NewDestroyResult("bigquery", "example-project", "segmentstream", "destroyed", "Destroyed dataset example-project:segmentstream."),
	}
	if err := fake.SaveAccessMarker(credentials.Store{HomeDir: home}, "production-bigquery", project.Warehouse{
		Type:     "bigquery",
		Auth:     "production-bigquery",
		Project:  "example-project",
		Dataset:  "segmentstream",
		Location: "EU",
	}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials:       credentials.Store{HomeDir: home},
		WarehouseRegistry: warehouse.NewRegistry(fake),
	})
	cmd.SetArgs([]string{"warehouse", "destroy", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("warehouse destroy failed: %v", err)
	}
	if !fake.destroyCalled {
		t.Fatal("destroy connector was not called")
	}
	if fake.destroyOptions.Force {
		t.Fatal("Force option = true, want false")
	}
	if fake.config.Project != "example-project" || fake.config.Dataset != "segmentstream" || fake.config.Location != "EU" {
		t.Fatalf("destroy config = %+v, want configured dataset", fake.config)
	}
	config, exists, err := (project.Store{Root: root}).LoadPartial()
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("segmentstream.yml was removed")
	}
	if config.Warehouse.Type != "bigquery" || config.Warehouse.Auth != "production-bigquery" {
		t.Fatalf("warehouse type/auth = %+v, want preserved", config.Warehouse)
	}
	if config.Warehouse.Project != "" || config.Warehouse.Dataset != "" || config.Warehouse.Location != "" {
		t.Fatalf("warehouse project/dataset/location = %+v, want cleared", config.Warehouse)
	}
	matches, err := fake.HasMatchingAccessMarker(credentials.Store{HomeDir: home}, "production-bigquery", project.Warehouse{
		Project:  "example-project",
		Dataset:  "segmentstream",
		Location: "EU",
	})
	if err != nil {
		t.Fatal(err)
	}
	if matches {
		t.Fatal("access marker still matches after destroy")
	}
	var result warehouse.DestroyResult
	decodeJSONResponseData(t, out.Bytes(), &result)
	if result.Status != "destroyed" || result.Project != "example-project" || result.Dataset != "segmentstream" {
		t.Fatalf("destroy result = %+v, want destroyed dataset", result)
	}
}

func TestWarehouseDestroyForceForwardsOption(t *testing.T) {
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
		destroyResult: warehouse.NewDestroyResult("bigquery", "example-project", "segmentstream", "destroyed", "Destroyed dataset example-project:segmentstream."),
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials:       credentials.Store{HomeDir: filepath.Join(root, "home")},
		WarehouseRegistry: warehouse.NewRegistry(fake),
	})
	cmd.SetArgs([]string{"warehouse", "destroy", "--force", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("warehouse destroy failed: %v", err)
	}
	if !fake.destroyOptions.Force {
		t.Fatal("Force option = false, want true")
	}
}

func TestWarehouseDestroyNonEmptyDatasetRequiresForce(t *testing.T) {
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
		destroyResult: warehouse.NewDestroyResult("bigquery", "example-project", "segmentstream", "not_empty", "Dataset example-project:segmentstream is not empty."),
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials:       credentials.Store{HomeDir: filepath.Join(root, "home")},
		WarehouseRegistry: warehouse.NewRegistry(fake),
	})
	cmd.SetArgs([]string{"warehouse", "destroy", "--json"})

	err := cmd.Execute()
	if cliresult.ExitCode(err) != cliresult.ExitMisconfigured {
		t.Fatalf("exit code = %d, want %d; err = %v", cliresult.ExitCode(err), cliresult.ExitMisconfigured, err)
	}
	if !strings.Contains(out.String(), "dataset_not_empty") || !strings.Contains(out.String(), "segmentstream warehouse destroy --force") {
		t.Fatalf("warehouse destroy output = %s, want force diagnostic", out.String())
	}
	config, err := project.LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if config.Warehouse.Project != "example-project" || config.Warehouse.Dataset != "segmentstream" || config.Warehouse.Location != "EU" {
		t.Fatalf("warehouse config = %+v, want unchanged", config.Warehouse)
	}
}

func TestWarehouseDestroyHelpIncludesCommandAndForce(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{})
	cmd.SetArgs([]string{"warehouse", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("warehouse --help failed: %v", err)
	}
	if !strings.Contains(out.String(), "destroy") {
		t.Fatalf("help output = %s, want destroy command", out.String())
	}

	out.Reset()
	cmd = newRootCommand(&out, &errOut, cliOptions{})
	cmd.SetArgs([]string{"warehouse", "destroy", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("warehouse destroy --help failed: %v", err)
	}
	if !strings.Contains(out.String(), "--force") {
		t.Fatalf("destroy help output = %s, want --force", out.String())
	}
}

func TestWarehouseBrowseDoesNotRequireConfiguredProject(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	home := filepath.Join(root, "home")
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery", "default-bigquery"); err != nil {
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
	var browseResponse struct {
		Data warehouseBrowseData `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &browseResponse); err != nil {
		t.Fatalf("parse browse json output: %v\n%s", err, out.String())
	}
	if browseResponse.Data.Level != "project" || len(browseResponse.Data.Children) != 1 || browseResponse.Data.Children[0].ID != "example-project" {
		t.Fatalf("browse data = %+v, want project result", browseResponse.Data)
	}
}

func TestWarehouseBrowseTableJSONForwardsPath(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	home := filepath.Join(root, "home")
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery", "default-bigquery"); err != nil {
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
	decodeJSONResponseData(t, out.Bytes(), &result)
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
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery", "default-bigquery"); err != nil {
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
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery", "default-bigquery"); err != nil {
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

func TestWarehouseQueryHelpIncludesSafetyFlags(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{})
	cmd.SetArgs([]string{"warehouse", "query", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("warehouse query --help failed: %v", err)
	}
	for _, want := range []string{"--sql", "--max-rows", "--timeout", "--maximum-bytes-billed"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("help output = %s, want %s", out.String(), want)
		}
	}
}

func TestWarehouseQueryJSONReturnsRowsAndForwardsDefaults(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	home := filepath.Join(root, "home")
	writeConfig(t, root, `version: 1
warehouse:
  type: bigquery
  auth: production-bigquery
  project: example-project
  dataset: segmentstream
  location: EU
`)
	writeNamedCredential(t, home, "production-bigquery")
	fake := &fakeWarehouseConnector{
		queryRows: []map[string]any{
			{"payload": `{"event":"purchase"}`, "event_name": "purchase"},
		},
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials:       credentials.Store{HomeDir: home},
		WarehouseRegistry: warehouse.NewRegistry(fake),
	})
	cmd.SetArgs([]string{"warehouse", "query", "--sql", "SELECT payload FROM events", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("warehouse query failed: %v", err)
	}
	if !fake.queryCalled {
		t.Fatal("query connector was not called")
	}
	if fake.queryOptions.SQL != "SELECT payload FROM events" ||
		fake.queryOptions.MaxRows != defaultWarehouseQueryMaxRows ||
		fake.queryOptions.Timeout != defaultWarehouseQueryTimeout ||
		fake.queryOptions.MaximumBytesBilled != 0 {
		t.Fatalf("query options = %+v, want defaults", fake.queryOptions)
	}
	if fake.config.Project != "example-project" || fake.config.Location != "EU" {
		t.Fatalf("query config = %+v, want configured warehouse", fake.config)
	}

	var rows []map[string]any
	response := decodeJSONResponseData(t, out.Bytes(), &rows)
	if response.Command != "warehouse.query" || response.Status != string(cliresult.StatusOK) {
		t.Fatalf("response = %+v, want warehouse.query ok", response)
	}
	if len(rows) != 1 || rows[0]["payload"] != `{"event":"purchase"}` || rows[0]["event_name"] != "purchase" {
		t.Fatalf("rows = %+v, want query rows", rows)
	}
}

func TestWarehouseQueryForwardsCustomLimits(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	home := filepath.Join(root, "home")
	writeConfig(t, root, `version: 1
warehouse:
  type: bigquery
  auth: production-bigquery
  project: example-project
  dataset: segmentstream
  location: EU
`)
	writeNamedCredential(t, home, "production-bigquery")
	fake := &fakeWarehouseConnector{}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials:       credentials.Store{HomeDir: home},
		WarehouseRegistry: warehouse.NewRegistry(fake),
	})
	cmd.SetArgs([]string{
		"warehouse", "query",
		"--sql", " SELECT 1 ",
		"--max-rows", "7",
		"--timeout", "45s",
		"--maximum-bytes-billed", "12345",
		"--json",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("warehouse query failed: %v", err)
	}
	if fake.queryOptions.SQL != "SELECT 1" ||
		fake.queryOptions.MaxRows != 7 ||
		fake.queryOptions.Timeout != 45*time.Second ||
		fake.queryOptions.MaximumBytesBilled != 12345 {
		t.Fatalf("query options = %+v, want custom limits", fake.queryOptions)
	}
}

func TestWarehouseQueryInvalidOptionsDoNotCallConnector(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	fake := &fakeWarehouseConnector{}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		WarehouseRegistry: warehouse.NewRegistry(fake),
	})
	cmd.SetArgs([]string{
		"warehouse", "query",
		"--max-rows", "0",
		"--timeout", "3m",
		"--maximum-bytes-billed", "-1",
		"--json",
	})

	err := cmd.Execute()
	if cliresult.ExitCode(err) != cliresult.ExitMisconfigured {
		t.Fatalf("exit code = %d, want %d; err = %v", cliresult.ExitCode(err), cliresult.ExitMisconfigured, err)
	}
	if fake.queryCalled {
		t.Fatal("query connector was called for invalid options")
	}
	var response testJSONResponse
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("json output is not JSON: %v\n%s", err, out.String())
	}
	if response.Status != string(cliresult.StatusInvalid) ||
		!strings.Contains(out.String(), "missing_sql") ||
		!strings.Contains(out.String(), "invalid_max_rows") ||
		!strings.Contains(out.String(), "invalid_timeout") ||
		!strings.Contains(out.String(), "invalid_maximum_bytes_billed") {
		t.Fatalf("response = %+v output=%s, want option diagnostics", response, out.String())
	}
}

func TestWarehouseQueryTextOutputPrintsRowsAsJSON(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	home := filepath.Join(root, "home")
	writeConfig(t, root, `version: 1
warehouse:
  type: bigquery
  auth: production-bigquery
  project: example-project
  dataset: segmentstream
  location: EU
`)
	writeNamedCredential(t, home, "production-bigquery")
	fake := &fakeWarehouseConnector{
		queryRows: []map[string]any{{"ok": "1"}},
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut, cliOptions{
		Credentials:       credentials.Store{HomeDir: home},
		WarehouseRegistry: warehouse.NewRegistry(fake),
	})
	cmd.SetArgs([]string{"warehouse", "query", "--sql", "SELECT 1 AS ok"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("warehouse query failed: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "Data:") || !strings.Contains(got, `"ok": "1"`) || !strings.HasPrefix(strings.TrimSpace(got), "[") {
		t.Fatalf("warehouse query output = %q, want row JSON without Data heading", got)
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
	verified, err := fake.HasMatchingAccessMarker(credentials.Store{HomeDir: home}, "production-bigquery", project.Warehouse{
		Type:     "bigquery",
		Auth:     "production-bigquery",
		Project:  "example-project",
		Dataset:  "segmentstream",
		Location: "EU",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !verified {
		t.Fatal("expected access marker to be saved")
	}
}

func hasWarehouseValidation(validations []warehouse.Validation, id, status string) bool {
	for _, validation := range validations {
		if validation.ID == id && validation.Status == status {
			return true
		}
	}
	return false
}

type fakeWarehouseConnector struct {
	browseResult     warehouse.BrowseResult
	browsePath       string
	configureResult  warehouse.ConfigureResult
	configureOptions warehouse.ConfigureOptions
	destroyResult    warehouse.DestroyResult
	destroyOptions   warehouse.DestroyOptions
	destroyCalled    bool
	testResult       warehouse.TestResult
	config           project.Warehouse
	queryRows        []map[string]any
	queryOptions     warehouse.QueryOptions
	queryCalled      bool
}

type fakeAccessMarker struct {
	Project  string `json:"project"`
	Dataset  string `json:"dataset"`
	Location string `json:"location"`
}

func (connector *fakeWarehouseConnector) Type() string {
	return "bigquery"
}

func (connector *fakeWarehouseConnector) DisplayName() string {
	return "BigQuery"
}

func (connector *fakeWarehouseConnector) DefaultAuthName() string {
	return "default-bigquery"
}

func (connector *fakeWarehouseConnector) AuthMethods() []string {
	return []string{"oauth", "service_account_key"}
}

func (connector *fakeWarehouseConnector) SelectWarehouseAccept() cliresult.NextActionAccept {
	return cliresult.NextActionAccept{
		Method:  "bigquery",
		Label:   "Use BigQuery",
		Command: "segmentstream init --warehouse bigquery",
		Value:   "bigquery",
	}
}

func (connector *fakeWarehouseConnector) AuthenticateAccepts() []cliresult.NextActionAccept {
	return nil
}

func (connector *fakeWarehouseConnector) ConfigureAccept() cliresult.NextActionAccept {
	return cliresult.NextActionAccept{
		Method:  "warehouse_config",
		Label:   "Configure BigQuery warehouse",
		Command: "segmentstream warehouse configure",
	}
}

func (connector *fakeWarehouseConnector) CredentialPath(store credentials.Store, name string) (string, error) {
	return store.CredentialPath(connector.Type(), name)
}

func (connector *fakeWarehouseConnector) HasCredential(store credentials.Store, name string) (bool, error) {
	return store.HasCredential(connector.Type(), name)
}

func (connector *fakeWarehouseConnector) SaveServiceAccountKey(store credentials.Store, name, sourcePath string) (string, error) {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", err
	}
	return store.SaveCredentialData(connector.Type(), name, data)
}

func (connector *fakeWarehouseConnector) LoginOAuth(ctx context.Context, out io.Writer, options warehouse.LoginOptions) ([]byte, error) {
	_ = ctx
	_ = out
	_ = options
	return nil, fmt.Errorf("fake oauth login is not configured")
}

func (connector *fakeWarehouseConnector) SaveOAuthCredential(store credentials.Store, name string, credential []byte) (string, error) {
	return store.SaveCredentialData(connector.Type(), name, credential)
}

func (connector *fakeWarehouseConnector) HasMatchingAccessMarker(store credentials.Store, name string, config project.Warehouse) (bool, error) {
	var marker fakeAccessMarker
	found, err := store.ReadAccessMarker(connector.Type(), name, &marker)
	if err != nil || !found {
		return found, err
	}
	return marker.Project == config.Project &&
		marker.Dataset == config.Dataset &&
		strings.EqualFold(marker.Location, config.Location), nil
}

func (connector *fakeWarehouseConnector) SaveAccessMarker(store credentials.Store, name string, config project.Warehouse) error {
	return store.SaveAccessMarker(connector.Type(), name, fakeAccessMarker{
		Project:  config.Project,
		Dataset:  config.Dataset,
		Location: config.Location,
	})
}

func (connector *fakeWarehouseConnector) ConfigDiagnostics(config project.Warehouse) []cliresult.Diagnostic {
	_ = config
	return nil
}

func (connector *fakeWarehouseConnector) RuntimeEnvironment(config project.Warehouse) []warehouse.EnvVar {
	_ = config
	return nil
}

func (connector *fakeWarehouseConnector) DBTProfileYAML(config project.Warehouse) string {
	_ = config
	return "segmentstream: {}\n"
}

func (connector *fakeWarehouseConnector) Browse(ctx context.Context, credentialPath string, path string) (warehouse.BrowseResult, error) {
	_ = ctx
	_ = credentialPath
	connector.browsePath = path
	return connector.browseResult, nil
}

func (connector *fakeWarehouseConnector) ValidateConfiguration(ctx context.Context, credentialPath string, config project.Warehouse, options warehouse.ConfigureOptions) (warehouse.ConfigureResult, error) {
	_ = ctx
	_ = credentialPath
	connector.config = config
	connector.configureOptions = options
	return connector.configureResult, nil
}

func (connector *fakeWarehouseConnector) Destroy(ctx context.Context, credentialPath string, config project.Warehouse, options warehouse.DestroyOptions) (warehouse.DestroyResult, error) {
	_ = ctx
	_ = credentialPath
	connector.destroyCalled = true
	connector.config = config
	connector.destroyOptions = options
	return connector.destroyResult, nil
}

func (connector *fakeWarehouseConnector) Test(context.Context, string, project.Warehouse) (warehouse.TestResult, error) {
	return connector.testResult, nil
}

func (connector *fakeWarehouseConnector) Query(ctx context.Context, credentialPath string, config project.Warehouse, options warehouse.QueryOptions) ([]map[string]any, error) {
	_ = ctx
	_ = credentialPath
	connector.queryCalled = true
	connector.config = config
	connector.queryOptions = options
	return connector.queryRows, nil
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
