package bigquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/segmentstream/segmentstream-cli/cli/internal/credentials"
	"github.com/segmentstream/segmentstream-cli/cli/internal/project"
	"github.com/segmentstream/segmentstream-cli/cli/internal/warehouse"
	"github.com/segmentstream/segmentstream-cli/cli/internal/warehouse/bigquery/googleoauth"
	bq "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/option"
)

func TestCredentialPath(t *testing.T) {
	home := t.TempDir()
	path, err := NewConnector().CredentialPath(credentials.Store{HomeDir: home}, "default-bigquery")
	if err != nil {
		t.Fatalf("CredentialPath failed: %v", err)
	}
	want := filepath.Join(home, ".segmentstream", "bigquery", "default-bigquery.json")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestSaveServiceAccountKey(t *testing.T) {
	root := t.TempDir()
	keyPath := filepath.Join(root, "key.json")
	data := []byte(`{"type":"service_account","client_email":"test@example.iam.gserviceaccount.com","private_key":"secret"}`)
	if err := os.WriteFile(keyPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	path, err := NewConnector().SaveServiceAccountKey(credentials.Store{HomeDir: filepath.Join(root, "home")}, "default-bigquery", keyPath)
	if err != nil {
		t.Fatalf("SaveServiceAccountKey failed: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("stored key = %s, want source key", string(got))
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
		}
	}
}

func TestSaveServiceAccountKeyClearsAccessMarker(t *testing.T) {
	root := t.TempDir()
	store := credentials.Store{HomeDir: filepath.Join(root, "home")}
	connector := NewConnector()
	config := validWarehouseConfig()
	if err := connector.SaveAccessMarker(store, "default-bigquery", config); err != nil {
		t.Fatalf("SaveAccessMarker failed: %v", err)
	}
	matches, err := connector.HasMatchingAccessMarker(store, "default-bigquery", config)
	if err != nil {
		t.Fatal(err)
	}
	if !matches {
		t.Fatal("expected marker before replacing credential")
	}

	keyPath := filepath.Join(root, "key.json")
	data := []byte(`{"type":"service_account","client_email":"test@example.iam.gserviceaccount.com","private_key":"secret"}`)
	if err := os.WriteFile(keyPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := connector.SaveServiceAccountKey(store, "default-bigquery", keyPath); err != nil {
		t.Fatalf("SaveServiceAccountKey failed: %v", err)
	}

	matches, err = connector.HasMatchingAccessMarker(store, "default-bigquery", config)
	if err != nil {
		t.Fatal(err)
	}
	if matches {
		t.Fatal("expected marker to be cleared after replacing credential")
	}
}

func TestSaveOAuthCredential(t *testing.T) {
	root := t.TempDir()
	store := credentials.Store{HomeDir: filepath.Join(root, "home")}
	data, err := googleAuthorizedUserCredentialJSON(googleoauth.Credential{
		ClientID:     "client-id.apps.googleusercontent.com",
		ClientSecret: "client-secret",
		RefreshToken: "refresh-token",
		TokenURI:     "https://oauth2.googleapis.com/token",
		Scopes:       []string{"https://www.googleapis.com/auth/bigquery"},
	})
	if err != nil {
		t.Fatalf("googleAuthorizedUserCredentialJSON failed: %v", err)
	}

	path, err := NewConnector().SaveOAuthCredential(store, "default-bigquery", data)
	if err != nil {
		t.Fatalf("SaveOAuthCredential failed: %v", err)
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"type": "authorized_user"`,
		`"client_id": "client-id.apps.googleusercontent.com"`,
		`"client_secret": "client-secret"`,
		`"refresh_token": "refresh-token"`,
		`"token_uri": "https://oauth2.googleapis.com/token"`,
		`"https://www.googleapis.com/auth/bigquery"`,
	} {
		if !strings.Contains(string(stored), want) {
			t.Fatalf("stored OAuth credential = %s, want %q", string(stored), want)
		}
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
		}
	}
}

func TestSaveServiceAccountKeyRejectsWrongType(t *testing.T) {
	root := t.TempDir()
	keyPath := filepath.Join(root, "key.json")
	if err := os.WriteFile(keyPath, []byte(`{"type":"authorized_user"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := NewConnector().SaveServiceAccountKey(credentials.Store{HomeDir: filepath.Join(root, "home")}, "default-bigquery", keyPath)
	if err == nil {
		t.Fatal("expected wrong credential type error")
	}
}

func TestAccessMarkerMatchesWarehouseConfig(t *testing.T) {
	home := t.TempDir()
	store := credentials.Store{HomeDir: home}
	connector := NewConnector()
	config := validWarehouseConfig()
	if err := connector.SaveAccessMarker(store, "default-bigquery", config); err != nil {
		t.Fatalf("SaveAccessMarker failed: %v", err)
	}
	matches, err := connector.HasMatchingAccessMarker(store, "default-bigquery", config)
	if err != nil {
		t.Fatal(err)
	}
	if !matches {
		t.Fatal("expected marker to match")
	}
	config.Dataset = "other"
	matches, err = connector.HasMatchingAccessMarker(store, "default-bigquery", config)
	if err != nil {
		t.Fatal(err)
	}
	if matches {
		t.Fatal("expected marker mismatch for different dataset")
	}
}

func TestValidateConfigurationRejectsInvalidDatasetWithoutNetwork(t *testing.T) {
	result, err := NewConnector().ValidateConfiguration(context.Background(), "unused.json", project.Warehouse{
		Type:     "bigquery",
		Auth:     "default-bigquery",
		Project:  "example-project",
		Dataset:  "segmentstream-new",
		Location: "EU",
	}, warehouse.ConfigureOptions{})
	if err != nil {
		t.Fatalf("ValidateConfiguration failed: %v", err)
	}
	if result.Status != "invalid" {
		t.Fatalf("status = %q, want invalid", result.Status)
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %+v, want one", result.Diagnostics)
	}
	if !strings.Contains(result.Diagnostics[0].Message, "letters, numbers, and underscores") {
		t.Fatalf("diagnostic = %+v, want dataset guidance", result.Diagnostics[0])
	}
	if result.Diagnostics[0].Suggestion != "segmentstream_new" {
		t.Fatalf("suggestion = %q, want segmentstream_new", result.Diagnostics[0].Suggestion)
	}
}

func TestValidateConfigurationRejectsPlaceholderProjectWithoutNetwork(t *testing.T) {
	result, err := NewConnector().ValidateConfiguration(context.Background(), "unused.json", project.Warehouse{
		Type:     "bigquery",
		Auth:     "default-bigquery",
		Project:  "your-gcp-project",
		Dataset:  "segmentstream",
		Location: "EU",
	}, warehouse.ConfigureOptions{})
	if err != nil {
		t.Fatalf("ValidateConfiguration failed: %v", err)
	}
	if result.Status != "invalid" {
		t.Fatalf("status = %q, want invalid", result.Status)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Field != "warehouse.project" {
		t.Fatalf("diagnostics = %+v, want project diagnostic", result.Diagnostics)
	}
}

func TestValidateConfigurationMissingDatasetWithoutCreateIsInvalid(t *testing.T) {
	var insertRequests int
	connector, cleanup := connectorWithBigQueryServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/bigquery/v2/projects/example-project/datasets/dataset_one":
			writeGoogleError(w, http.StatusNotFound, "Not found: Dataset example-project:dataset_one")
		case r.Method == http.MethodPost && r.URL.Path == "/bigquery/v2/projects/example-project/datasets":
			insertRequests++
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer cleanup()

	result, err := connector.ValidateConfiguration(context.Background(), "unused.json", validWarehouseConfig(), warehouse.ConfigureOptions{})
	if err != nil {
		t.Fatalf("ValidateConfiguration failed: %v", err)
	}
	if result.Status != "invalid" {
		t.Fatalf("status = %q, want invalid", result.Status)
	}
	if insertRequests != 0 {
		t.Fatalf("insert requests = %d, want none", insertRequests)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].ID != "missing_dataset" {
		t.Fatalf("diagnostics = %+v, want missing_dataset", result.Diagnostics)
	}
	if !strings.Contains(result.Diagnostics[0].Suggestion, "--create-dataset") {
		t.Fatalf("suggestion = %q, want --create-dataset", result.Diagnostics[0].Suggestion)
	}
}

func TestValidateConfigurationCreatesMissingDataset(t *testing.T) {
	var insertRequests int
	connector, cleanup := connectorWithBigQueryServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/bigquery/v2/projects/example-project/datasets/dataset_one":
			writeGoogleError(w, http.StatusNotFound, "Not found: Dataset example-project:dataset_one")
		case r.Method == http.MethodPost && r.URL.Path == "/bigquery/v2/projects/example-project/datasets":
			insertRequests++
			var request bq.Dataset
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode insert request: %v", err)
			}
			if request.DatasetReference == nil ||
				request.DatasetReference.ProjectId != "example-project" ||
				request.DatasetReference.DatasetId != "dataset_one" ||
				request.Location != "EU" {
				t.Fatalf("insert request = %+v, want dataset reference and EU location", request)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"datasetReference":{"projectId":"example-project","datasetId":"dataset_one"},"location":"EU"}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer cleanup()

	result, err := connector.ValidateConfiguration(context.Background(), "unused.json", validWarehouseConfig(), warehouse.ConfigureOptions{CreateDataset: true})
	if err != nil {
		t.Fatalf("ValidateConfiguration failed: %v", err)
	}
	if result.Status != "valid" {
		t.Fatalf("status = %q, want valid", result.Status)
	}
	if insertRequests != 1 {
		t.Fatalf("insert requests = %d, want one", insertRequests)
	}
	if !hasValidation(result.Validations, "dataset_exists", "created") {
		t.Fatalf("validations = %+v, want created dataset validation", result.Validations)
	}
}

func TestValidateConfigurationCreateFlagDoesNotInsertExistingDataset(t *testing.T) {
	var insertRequests int
	connector, cleanup := connectorWithBigQueryServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/bigquery/v2/projects/example-project/datasets/dataset_one":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"datasetReference":{"projectId":"example-project","datasetId":"dataset_one"},"location":"EU"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/bigquery/v2/projects/example-project/datasets":
			insertRequests++
			writeGoogleError(w, http.StatusConflict, "Already Exists")
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer cleanup()

	result, err := connector.ValidateConfiguration(context.Background(), "unused.json", validWarehouseConfig(), warehouse.ConfigureOptions{CreateDataset: true})
	if err != nil {
		t.Fatalf("ValidateConfiguration failed: %v", err)
	}
	if result.Status != "valid" {
		t.Fatalf("status = %q, want valid", result.Status)
	}
	if insertRequests != 0 {
		t.Fatalf("insert requests = %d, want none", insertRequests)
	}
	if !hasValidation(result.Validations, "dataset_location", "ok") {
		t.Fatalf("validations = %+v, want existing dataset location validation", result.Validations)
	}
}

func TestValidateConfigurationRejectsLocationMismatch(t *testing.T) {
	connector, cleanup := connectorWithBigQueryServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/bigquery/v2/projects/example-project/datasets/dataset_one" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"datasetReference":{"projectId":"example-project","datasetId":"dataset_one"},"location":"US"}`)
	}))
	defer cleanup()

	result, err := connector.ValidateConfiguration(context.Background(), "unused.json", validWarehouseConfig(), warehouse.ConfigureOptions{})
	if err != nil {
		t.Fatalf("ValidateConfiguration failed: %v", err)
	}
	if result.Status != "invalid" {
		t.Fatalf("status = %q, want invalid", result.Status)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].ID != "location_mismatch" {
		t.Fatalf("diagnostics = %+v, want location_mismatch", result.Diagnostics)
	}
}

func TestValidateConfigurationCreateConflictRefetchesDataset(t *testing.T) {
	var getRequests int
	var insertRequests int
	connector, cleanup := connectorWithBigQueryServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/bigquery/v2/projects/example-project/datasets/dataset_one":
			getRequests++
			if getRequests == 1 {
				writeGoogleError(w, http.StatusNotFound, "Not found: Dataset example-project:dataset_one")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"datasetReference":{"projectId":"example-project","datasetId":"dataset_one"},"location":"EU"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/bigquery/v2/projects/example-project/datasets":
			insertRequests++
			writeGoogleError(w, http.StatusConflict, "Already Exists: Dataset example-project:dataset_one")
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer cleanup()

	result, err := connector.ValidateConfiguration(context.Background(), "unused.json", validWarehouseConfig(), warehouse.ConfigureOptions{CreateDataset: true})
	if err != nil {
		t.Fatalf("ValidateConfiguration failed: %v", err)
	}
	if result.Status != "valid" {
		t.Fatalf("status = %q, want valid", result.Status)
	}
	if getRequests != 2 || insertRequests != 1 {
		t.Fatalf("get requests = %d, insert requests = %d, want 2 and 1", getRequests, insertRequests)
	}
	if !hasValidation(result.Validations, "dataset_location", "ok") {
		t.Fatalf("validations = %+v, want existing dataset validation", result.Validations)
	}
}

func TestDestroyAbsentDatasetIsNoop(t *testing.T) {
	var deleteRequests int
	connector, cleanup := connectorWithBigQueryServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/bigquery/v2/projects/example-project/datasets/dataset_one":
			writeGoogleError(w, http.StatusNotFound, "Not found: Dataset example-project:dataset_one")
		case r.Method == http.MethodDelete:
			deleteRequests++
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer cleanup()

	result, err := connector.Destroy(context.Background(), "unused.json", validWarehouseConfig(), warehouse.DestroyOptions{})
	if err != nil {
		t.Fatalf("Destroy failed: %v", err)
	}
	if result.Status != "absent" || result.Project != "example-project" || result.Dataset != "dataset_one" {
		t.Fatalf("result = %+v, want absent dataset result", result)
	}
	if deleteRequests != 0 {
		t.Fatalf("delete requests = %d, want none", deleteRequests)
	}
}

func TestDestroyEmptyDatasetDeletesWithoutDeleteContents(t *testing.T) {
	var deleteRequests int
	var deleteContents string
	connector, cleanup := connectorWithBigQueryServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/bigquery/v2/projects/example-project/datasets/dataset_one":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"datasetReference":{"projectId":"example-project","datasetId":"dataset_one"},"location":"EU"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/bigquery/v2/projects/example-project/datasets/dataset_one/tables":
			if r.URL.Query().Get("maxResults") != "1" {
				t.Fatalf("maxResults = %q, want 1", r.URL.Query().Get("maxResults"))
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/bigquery/v2/projects/example-project/datasets/dataset_one":
			deleteRequests++
			deleteContents = r.URL.Query().Get("deleteContents")
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer cleanup()

	result, err := connector.Destroy(context.Background(), "unused.json", validWarehouseConfig(), warehouse.DestroyOptions{})
	if err != nil {
		t.Fatalf("Destroy failed: %v", err)
	}
	if result.Status != "destroyed" {
		t.Fatalf("result = %+v, want destroyed", result)
	}
	if deleteRequests != 1 || deleteContents != "" {
		t.Fatalf("delete requests = %d deleteContents=%q, want one delete without deleteContents", deleteRequests, deleteContents)
	}
}

func TestDestroyNonEmptyDatasetRequiresForce(t *testing.T) {
	var deleteRequests int
	connector, cleanup := connectorWithBigQueryServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/bigquery/v2/projects/example-project/datasets/dataset_one":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"datasetReference":{"projectId":"example-project","datasetId":"dataset_one"},"location":"EU"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/bigquery/v2/projects/example-project/datasets/dataset_one/tables":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"tables":[{"tableReference":{"projectId":"example-project","datasetId":"dataset_one","tableId":"events"},"type":"TABLE"}]}`)
		case r.Method == http.MethodDelete:
			deleteRequests++
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer cleanup()

	result, err := connector.Destroy(context.Background(), "unused.json", validWarehouseConfig(), warehouse.DestroyOptions{})
	if err != nil {
		t.Fatalf("Destroy failed: %v", err)
	}
	if result.Status != "not_empty" || !strings.Contains(result.Message, "--force") {
		t.Fatalf("result = %+v, want not_empty with force guidance", result)
	}
	if deleteRequests != 0 {
		t.Fatalf("delete requests = %d, want none", deleteRequests)
	}
}

func TestDestroyNonEmptyDatasetWithForceDeletesContents(t *testing.T) {
	var deleteRequests int
	var deleteContents string
	connector, cleanup := connectorWithBigQueryServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/bigquery/v2/projects/example-project/datasets/dataset_one":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"datasetReference":{"projectId":"example-project","datasetId":"dataset_one"},"location":"EU"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/bigquery/v2/projects/example-project/datasets/dataset_one/tables":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"tables":[{"tableReference":{"projectId":"example-project","datasetId":"dataset_one","tableId":"events"},"type":"TABLE"}]}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/bigquery/v2/projects/example-project/datasets/dataset_one":
			deleteRequests++
			deleteContents = r.URL.Query().Get("deleteContents")
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer cleanup()

	result, err := connector.Destroy(context.Background(), "unused.json", validWarehouseConfig(), warehouse.DestroyOptions{Force: true})
	if err != nil {
		t.Fatalf("Destroy failed: %v", err)
	}
	if result.Status != "destroyed" {
		t.Fatalf("result = %+v, want destroyed", result)
	}
	if deleteRequests != 1 || deleteContents != "true" {
		t.Fatalf("delete requests = %d deleteContents=%q, want force delete", deleteRequests, deleteContents)
	}
}

func TestNewServiceAcceptsAuthorizedUserCredentialFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "credential.json")
	data := []byte(`{
  "type": "authorized_user",
  "client_id": "client-id.apps.googleusercontent.com",
  "client_secret": "client-secret",
  "refresh_token": "refresh-token",
  "token_uri": "https://oauth2.googleapis.com/token"
}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := newService(context.Background(), path); err != nil {
		t.Fatalf("newService failed for authorized_user credential: %v", err)
	}
}

func TestBrowseDatasetsDoesNotRequestHiddenDatasets(t *testing.T) {
	var gotAll string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bigquery/v2/projects/example-project/datasets" {
			t.Fatalf("path = %q, want datasets list path", r.URL.Path)
		}
		gotAll = r.URL.Query().Get("all")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"datasets":[{"datasetReference":{"datasetId":"visible_dataset"},"location":"EU"}]}`)
	}))
	defer server.Close()

	service, err := bq.NewService(context.Background(), option.WithEndpoint(server.URL+"/bigquery/v2/"), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	result, err := NewConnector().browseDatasets(context.Background(), service, "example-project")
	if err != nil {
		t.Fatalf("browseDatasets failed: %v", err)
	}
	if gotAll != "" {
		t.Fatalf("all query param = %q, want empty so hidden datasets are excluded", gotAll)
	}
	if len(result.Children) != 1 || result.Children[0].ID != "visible_dataset" || result.Children[0].Location != "EU" {
		t.Fatalf("children = %+v, want visible dataset", result.Children)
	}
}

func TestBrowseProjectsReadsAllPages(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bigquery/v2/projects" {
			t.Fatalf("path = %q, want projects list path", r.URL.Path)
		}
		requests++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("pageToken") == "" {
			fmt.Fprint(w, `{"projects":[{"id":"project-one"}],"nextPageToken":"next"}`)
			return
		}
		fmt.Fprint(w, `{"projects":[{"id":"project-two"}]}`)
	}))
	defer server.Close()

	service, err := bq.NewService(context.Background(), option.WithEndpoint(server.URL+"/bigquery/v2/"), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	result, err := NewConnector().browseProjects(context.Background(), service)
	if err != nil {
		t.Fatalf("browseProjects failed: %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want two paged requests", requests)
	}
	if len(result.Children) != 2 || result.Children[0].ID != "project-one" || result.Children[1].ID != "project-two" {
		t.Fatalf("children = %+v, want both project pages", result.Children)
	}
}

func TestBrowseDatasetsReadsAllPages(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bigquery/v2/projects/example-project/datasets" {
			t.Fatalf("path = %q, want datasets list path", r.URL.Path)
		}
		requests++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("pageToken") == "" {
			fmt.Fprint(w, `{"datasets":[{"datasetReference":{"datasetId":"dataset_one"},"location":"EU"}],"nextPageToken":"next"}`)
			return
		}
		fmt.Fprint(w, `{"datasets":[{"datasetReference":{"datasetId":"dataset_two"},"location":"US"}]}`)
	}))
	defer server.Close()

	service, err := bq.NewService(context.Background(), option.WithEndpoint(server.URL+"/bigquery/v2/"), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	result, err := NewConnector().browseDatasets(context.Background(), service, "example-project")
	if err != nil {
		t.Fatalf("browseDatasets failed: %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want two paged requests", requests)
	}
	if len(result.Children) != 2 ||
		result.Children[0].ID != "dataset_one" ||
		result.Children[0].Location != "EU" ||
		result.Children[1].ID != "dataset_two" ||
		result.Children[1].Location != "US" {
		t.Fatalf("children = %+v, want both dataset pages", result.Children)
	}
}

func TestBrowseTablesReadsAllPages(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bigquery/v2/projects/example-project/datasets/dataset_one/tables" {
			t.Fatalf("path = %q, want tables list path", r.URL.Path)
		}
		requests++
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("pageToken") == "" {
			fmt.Fprint(w, `{"tables":[{"tableReference":{"tableId":"events"},"friendlyName":"Events","type":"TABLE"}],"nextPageToken":"next"}`)
			return
		}
		fmt.Fprint(w, `{"tables":[{"tableReference":{"tableId":"events_view"},"type":"VIEW"}]}`)
	}))
	defer server.Close()

	service, err := bq.NewService(context.Background(), option.WithEndpoint(server.URL+"/bigquery/v2/"), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	result, err := NewConnector().browseTables(context.Background(), service, "example-project", "dataset_one")
	if err != nil {
		t.Fatalf("browseTables failed: %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want two paged requests", requests)
	}
	if result.Level != "table" || result.Path != "example-project/dataset_one" {
		t.Fatalf("result = %+v, want table level and dataset path", result)
	}
	if len(result.Children) != 2 ||
		result.Children[0].ID != "events" ||
		result.Children[0].FriendlyName != "Events" ||
		result.Children[0].Type != "TABLE" ||
		result.Children[1].ID != "events_view" ||
		result.Children[1].Type != "VIEW" {
		t.Fatalf("children = %+v, want both table pages", result.Children)
	}
}

func TestBrowseTableSchemaReturnsNestedFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bigquery/v2/projects/example-project/datasets/dataset_one/tables/events" {
			t.Fatalf("path = %q, want table get path", r.URL.Path)
		}
		if r.URL.Query().Get("view") != "BASIC" {
			t.Fatalf("view query param = %q, want BASIC", r.URL.Query().Get("view"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"schema":{"fields":[{"name":"event_id","type":"STRING","mode":"REQUIRED","description":"Stable event id"},{"name":"event_params","type":"RECORD","mode":"REPEATED","fields":[{"name":"key","type":"STRING"},{"name":"value","type":"RECORD","fields":[{"name":"string_value","type":"STRING"}]}]}]}}`)
	}))
	defer server.Close()

	service, err := bq.NewService(context.Background(), option.WithEndpoint(server.URL+"/bigquery/v2/"), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	result, err := NewConnector().browseTableSchema(context.Background(), service, "example-project", "dataset_one", "events")
	if err != nil {
		t.Fatalf("browseTableSchema failed: %v", err)
	}
	if result.Level != "schema" || result.Path != "example-project/dataset_one/events" {
		t.Fatalf("result = %+v, want schema level and table path", result)
	}
	if len(result.Children) != 0 {
		t.Fatalf("children = %+v, want empty children", result.Children)
	}
	if len(result.Schema) != 2 ||
		result.Schema[0].Name != "event_id" ||
		result.Schema[0].Type != "STRING" ||
		result.Schema[0].Mode != "REQUIRED" ||
		result.Schema[0].Description != "Stable event id" ||
		len(result.Schema[1].Fields) != 2 ||
		len(result.Schema[1].Fields[1].Fields) != 1 ||
		result.Schema[1].Fields[1].Fields[0].Name != "string_value" {
		t.Fatalf("schema = %+v, want nested fields", result.Schema)
	}
}

func TestQueryDryRunsSelectThenExecutesAndMapsRows(t *testing.T) {
	var dryRunRequest bq.Job
	var queryRequest bq.QueryRequest
	connector, cleanup := connectorWithBigQueryServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/bigquery/v2/projects/example-project/jobs":
			if err := json.NewDecoder(r.Body).Decode(&dryRunRequest); err != nil {
				t.Fatalf("decode dry run request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"statistics":{"query":{"statementType":"SELECT","totalBytesProcessed":"12345"}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/bigquery/v2/projects/example-project/queries":
			if err := json.NewDecoder(r.Body).Decode(&queryRequest); err != nil {
				t.Fatalf("decode query request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{
				"jobComplete": true,
				"schema": {"fields": [
					{"name": "payload", "type": "STRING"},
					{"name": "event_name", "type": "STRING"}
				]},
				"rows": [{"f": [
					{"v": "{\"event\":\"purchase\"}"},
					{"v": "purchase"}
				]}],
				"totalRows": "1"
			}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer cleanup()

	rows, err := connector.Query(context.Background(), "unused.json", validWarehouseConfig(), warehouse.QueryOptions{
		SQL:                "SELECT payload FROM events",
		MaxRows:            7,
		Timeout:            45 * time.Second,
		MaximumBytesBilled: 12345,
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if dryRunRequest.JobReference == nil ||
		dryRunRequest.JobReference.ProjectId != "example-project" ||
		dryRunRequest.JobReference.Location != "EU" ||
		dryRunRequest.Configuration == nil ||
		!dryRunRequest.Configuration.DryRun ||
		dryRunRequest.Configuration.Query == nil ||
		dryRunRequest.Configuration.Query.Query != "SELECT payload FROM events" ||
		dryRunRequest.Configuration.Query.UseLegacySql == nil ||
		*dryRunRequest.Configuration.Query.UseLegacySql ||
		dryRunRequest.Configuration.Query.MaximumBytesBilled != 12345 {
		t.Fatalf("dry run request = %+v, want guarded Standard SQL dry run", dryRunRequest)
	}
	if queryRequest.Query != "SELECT payload FROM events" ||
		queryRequest.Location != "EU" ||
		queryRequest.MaxResults != 7 ||
		queryRequest.TimeoutMs != 45000 ||
		queryRequest.MaximumBytesBilled != 12345 ||
		queryRequest.UseLegacySql == nil ||
		*queryRequest.UseLegacySql {
		t.Fatalf("query request = %+v, want forwarded query options", queryRequest)
	}
	if len(rows) != 1 || rows[0]["payload"] != `{"event":"purchase"}` || rows[0]["event_name"] != "purchase" {
		t.Fatalf("rows = %+v, want plain row objects", rows)
	}
}

func TestQueryRejectsNonSelectDryRunStatement(t *testing.T) {
	var queryRequests int
	connector, cleanup := connectorWithBigQueryServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/bigquery/v2/projects/example-project/jobs":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"statistics":{"query":{"statementType":"DELETE"}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/bigquery/v2/projects/example-project/queries":
			queryRequests++
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer cleanup()

	_, err := connector.Query(context.Background(), "unused.json", validWarehouseConfig(), warehouse.QueryOptions{
		SQL:     "DELETE FROM events WHERE true",
		MaxRows: 100,
		Timeout: 30 * time.Second,
	})
	if queryRequests != 0 {
		t.Fatalf("query requests = %d, want none", queryRequests)
	}
	assertQueryError(t, err, "non_select_query")
}

func TestQueryReturnsDryRunDiagnostic(t *testing.T) {
	connector, cleanup := connectorWithBigQueryServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/bigquery/v2/projects/example-project/jobs" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		writeGoogleError(w, http.StatusBadRequest, "Syntax error: Unexpected keyword")
	}))
	defer cleanup()

	_, err := connector.Query(context.Background(), "unused.json", validWarehouseConfig(), warehouse.QueryOptions{
		SQL:     "not sql",
		MaxRows: 100,
		Timeout: 30 * time.Second,
	})
	assertQueryError(t, err, "query_dry_run_failed")
}

func TestQueryReturnsExecutionDiagnostic(t *testing.T) {
	connector, cleanup := connectorWithBigQueryServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/bigquery/v2/projects/example-project/jobs":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"statistics":{"query":{"statementType":"SELECT"}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/bigquery/v2/projects/example-project/queries":
			writeGoogleError(w, http.StatusForbidden, "Access Denied")
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer cleanup()

	_, err := connector.Query(context.Background(), "unused.json", validWarehouseConfig(), warehouse.QueryOptions{
		SQL:     "SELECT * FROM forbidden_table",
		MaxRows: 100,
		Timeout: 30 * time.Second,
	})
	assertQueryError(t, err, "query_execution_failed")
}

func TestQueryLocationMismatchReturnsRecoveryActions(t *testing.T) {
	connector, cleanup := connectorWithBigQueryServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/bigquery/v2/projects/example-project/jobs":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"statistics":{"query":{"statementType":"SELECT"}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/bigquery/v2/projects/example-project/queries":
			writeGoogleError(w, http.StatusNotFound, "Not found: Dataset raw-project:raw_dataset was not found in location US")
		case r.Method == http.MethodGet && r.URL.Path == "/bigquery/v2/projects/raw-project/datasets/raw_dataset":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"datasetReference":{"projectId":"raw-project","datasetId":"raw_dataset"},"location":"EU"}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer cleanup()

	config := validWarehouseConfig()
	config.Dataset = "segmentstream"
	config.Location = "US"
	_, err := connector.Query(context.Background(), "unused.json", config, warehouse.QueryOptions{
		SQL:     "SELECT * FROM `raw-project.raw_dataset.events` LIMIT 5",
		MaxRows: 100,
		Timeout: 30 * time.Second,
	})
	var queryErr warehouse.QueryError
	if !errors.As(err, &queryErr) {
		t.Fatalf("error = %v, want warehouse.QueryError", err)
	}
	if len(queryErr.Diagnostics) != 1 || queryErr.Diagnostics[0].ID != "query_location_mismatch" {
		t.Fatalf("diagnostics = %+v, want query_location_mismatch", queryErr.Diagnostics)
	}
	for _, want := range []string{
		"BigQuery looked for source dataset raw-project.raw_dataset in warehouse.location US, but it is in EU.",
		"Not found: Dataset raw-project:raw_dataset was not found in location US",
	} {
		if !strings.Contains(queryErr.Diagnostics[0].Message, want) {
			t.Fatalf("diagnostic message = %q, want %q", queryErr.Diagnostics[0].Message, want)
		}
	}
	if queryErr.Diagnostics[0].Suggestion != "Recreate the configured SegmentStream dataset example-project.segmentstream in EU, then rerun warehouse test." {
		t.Fatalf("suggestion = %q, want clear recreate guidance", queryErr.Diagnostics[0].Suggestion)
	}
	if len(queryErr.Actions) != 2 ||
		queryErr.Actions[0].Command != "segmentstream warehouse destroy --json" ||
		!strings.Contains(queryErr.Actions[1].Command, "--location EU") ||
		!strings.Contains(queryErr.Actions[1].Command, "--create-dataset") {
		t.Fatalf("actions = %+v, want destroy and reconfigure commands", queryErr.Actions)
	}
}

func TestQueryCancelsIncompleteJob(t *testing.T) {
	var cancelRequests int
	var cancelLocation string
	connector, cleanup := connectorWithBigQueryServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/bigquery/v2/projects/example-project/jobs":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"statistics":{"query":{"statementType":"SELECT"}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/bigquery/v2/projects/example-project/queries":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"jobComplete":false,"jobReference":{"projectId":"example-project","jobId":"job_123","location":"EU"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/bigquery/v2/projects/example-project/jobs/job_123/cancel":
			cancelRequests++
			cancelLocation = r.URL.Query().Get("location")
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer cleanup()

	_, err := connector.Query(context.Background(), "unused.json", validWarehouseConfig(), warehouse.QueryOptions{
		SQL:     "SELECT * FROM slow_table",
		MaxRows: 100,
		Timeout: time.Second,
	})
	assertQueryError(t, err, "query_timeout")
	if cancelRequests != 1 || cancelLocation != "EU" {
		t.Fatalf("cancel requests = %d location=%q, want one EU cancel", cancelRequests, cancelLocation)
	}
}

func TestQueryRejectsDuplicateColumnNames(t *testing.T) {
	connector, cleanup := connectorWithBigQueryServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/bigquery/v2/projects/example-project/jobs":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"statistics":{"query":{"statementType":"SELECT"}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/bigquery/v2/projects/example-project/queries":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{
				"jobComplete": true,
				"schema": {"fields": [
					{"name": "same", "type": "STRING"},
					{"name": "same", "type": "STRING"}
				]},
				"rows": [{"f": [{"v": "one"}, {"v": "two"}]}]
			}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer cleanup()

	_, err := connector.Query(context.Background(), "unused.json", validWarehouseConfig(), warehouse.QueryOptions{
		SQL:     "SELECT 1 AS same, 2 AS same",
		MaxRows: 100,
		Timeout: 30 * time.Second,
	})
	assertQueryError(t, err, "duplicate_column_names")
}

func TestBrowseRejectsInvalidPathsBeforeCreatingClient(t *testing.T) {
	for _, path := range []string{
		"example-project//dataset_one",
		"example-project/dataset_one/events/extra",
	} {
		_, err := NewConnector().Browse(context.Background(), "", path)
		if err == nil {
			t.Fatalf("Browse(%q) succeeded, want invalid path error", path)
		}
		if !strings.Contains(err.Error(), "invalid BigQuery browse path") {
			t.Fatalf("Browse(%q) error = %v, want invalid path error", path, err)
		}
	}
}

func assertQueryError(t *testing.T, err error, id string) {
	t.Helper()
	var queryErr warehouse.QueryError
	if !errors.As(err, &queryErr) {
		t.Fatalf("error = %v, want warehouse.QueryError", err)
	}
	if len(queryErr.Diagnostics) != 1 || queryErr.Diagnostics[0].ID != id {
		t.Fatalf("diagnostics = %+v, want %s", queryErr.Diagnostics, id)
	}
}

func connectorWithBigQueryServer(t *testing.T, handler http.Handler) (Connector, func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	service, err := bq.NewService(context.Background(), option.WithEndpoint(server.URL+"/bigquery/v2/"), option.WithoutAuthentication())
	if err != nil {
		server.Close()
		t.Fatalf("NewService failed: %v", err)
	}
	return Connector{
		newService: func(context.Context, string) (*bq.Service, error) {
			return service, nil
		},
	}, server.Close
}

func validWarehouseConfig() project.Warehouse {
	return project.Warehouse{
		Type:     "bigquery",
		Auth:     "default-bigquery",
		Project:  "example-project",
		Dataset:  "dataset_one",
		Location: "EU",
	}
}

func writeGoogleError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"error":{"code":%d,"message":%q}}`, status, message)
}

func hasValidation(validations []warehouse.Validation, id, status string) bool {
	for _, validation := range validations {
		if validation.ID == id && validation.Status == status {
			return true
		}
	}
	return false
}
