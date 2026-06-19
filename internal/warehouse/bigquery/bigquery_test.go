package bigquery

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/segmentstream/segmentstream-cli/internal/project"
	bq "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/option"
)

func TestValidateConfigurationRejectsInvalidDatasetWithoutNetwork(t *testing.T) {
	result, err := NewConnector().ValidateConfiguration(context.Background(), "unused.json", project.Warehouse{
		Type:     "bigquery",
		Auth:     "default-bigquery",
		Project:  "example-project",
		Dataset:  "segmentstream-new",
		Location: "EU",
	})
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
	})
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
