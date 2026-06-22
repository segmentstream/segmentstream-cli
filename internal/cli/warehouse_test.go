package cli

import (
	"bytes"
	"context"
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

func TestWarehouseConfigureCreateDatasetJSONForwardsOptionAndWritesCreatedResult(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery"); err != nil {
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
	if _, err := (project.Store{Root: root}).SelectWarehouse("bigquery"); err != nil {
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
	if _, err := store.SelectWarehouse("bigquery"); err != nil {
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
	testResult       warehouse.TestResult
	config           project.Warehouse
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

func (connector *fakeWarehouseConnector) ValidateConfiguration(ctx context.Context, credentialPath string, config project.Warehouse, options warehouse.ConfigureOptions) (warehouse.ConfigureResult, error) {
	_ = ctx
	_ = credentialPath
	connector.config = config
	connector.configureOptions = options
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
