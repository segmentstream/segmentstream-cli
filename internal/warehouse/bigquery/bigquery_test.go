package bigquery

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
