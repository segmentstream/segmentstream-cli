package bigquery

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/segmentstream/segmentstream-cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/internal/project"
	"github.com/segmentstream/segmentstream-cli/internal/warehouse"
	bq "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

type Connector struct{}

func NewConnector() Connector {
	return Connector{}
}

func (connector Connector) Type() string {
	return "bigquery"
}

func (connector Connector) Browse(ctx context.Context, credentialPath string, path string) (warehouse.BrowseResult, error) {
	parts, err := parseBrowsePath(path)
	if err != nil {
		return warehouse.BrowseResult{}, err
	}
	service, err := newService(ctx, credentialPath)
	if err != nil {
		return warehouse.BrowseResult{}, err
	}
	switch len(parts) {
	case 0:
		return connector.browseProjects(ctx, service)
	case 1:
		return connector.browseDatasets(ctx, service, parts[0])
	case 2:
		return connector.browseTables(ctx, service, parts[0], parts[1])
	case 3:
		return connector.browseTableSchema(ctx, service, parts[0], parts[1], parts[2])
	default:
		return warehouse.BrowseResult{}, fmt.Errorf("invalid BigQuery browse path %q; use <project>, <project>/<dataset>, or <project>/<dataset>/<table>", path)
	}
}

func (connector Connector) ValidateConfiguration(ctx context.Context, credentialPath string, config project.Warehouse) (warehouse.ConfigureResult, error) {
	var validations []warehouse.Validation
	var diagnostics []cliresult.Diagnostic

	if config.Project == "" || config.Project == "your-gcp-project" {
		diagnostics = append(diagnostics, cliresult.Diagnostic{
			ID:      "invalid_project",
			Field:   "warehouse.project",
			Message: "warehouse.project must be set to a real Google Cloud project ID.",
		})
	} else {
		validations = append(validations, warehouse.Validation{ID: "project", Field: "warehouse.project", Status: "ok"})
	}

	if err := project.ValidateBigQueryDatasetID(config.Dataset); err != nil {
		diagnostics = append(diagnostics, cliresult.Diagnostic{
			ID:         "invalid_dataset",
			Field:      "warehouse.dataset",
			Message:    err.Error(),
			Suggestion: suggestDataset(config.Dataset),
		})
	} else {
		validations = append(validations, warehouse.Validation{ID: "dataset", Field: "warehouse.dataset", Status: "ok"})
	}

	location := strings.TrimSpace(config.Location)
	if location == "" {
		diagnostics = append(diagnostics, cliresult.Diagnostic{
			ID:         "invalid_location",
			Field:      "warehouse.location",
			Message:    "warehouse.location is required.",
			Suggestion: project.DefaultLocation,
		})
	} else {
		validations = append(validations, warehouse.Validation{ID: "location", Field: "warehouse.location", Status: "ok"})
	}

	if len(diagnostics) > 0 {
		return warehouse.NewConfigureResult(connector.Type(), validations, diagnostics), nil
	}

	service, err := newService(ctx, credentialPath)
	if err != nil {
		return warehouse.ConfigureResult{}, err
	}
	dataset, err := service.Datasets.Get(config.Project, config.Dataset).Do()
	if err != nil {
		if isHTTPStatus(err, 404) {
			validations = append(validations, warehouse.Validation{
				ID:      "dataset_exists",
				Field:   "warehouse.dataset",
				Status:  "not_found",
				Message: "Dataset does not exist yet; SegmentStream will need create permissions before run.",
			})
			return warehouse.NewConfigureResult(connector.Type(), validations, nil), nil
		}
		return warehouse.ConfigureResult{}, fmt.Errorf("check BigQuery dataset: %w", explainGoogleAPIError(err))
	}
	if !strings.EqualFold(dataset.Location, location) {
		diagnostics = append(diagnostics, cliresult.Diagnostic{
			ID:      "location_mismatch",
			Field:   "warehouse.location",
			Message: fmt.Sprintf("Dataset %s.%s is in %s, not %s.", config.Project, config.Dataset, dataset.Location, location),
		})
	} else {
		validations = append(validations, warehouse.Validation{
			ID:      "dataset_location",
			Field:   "warehouse.location",
			Status:  "ok",
			Message: fmt.Sprintf("Existing dataset location is %s.", dataset.Location),
		})
	}
	return warehouse.NewConfigureResult(connector.Type(), validations, diagnostics), nil
}

func (connector Connector) Test(ctx context.Context, credentialPath string, config project.Warehouse) (warehouse.TestResult, error) {
	service, err := newService(ctx, credentialPath)
	if err != nil {
		return warehouse.TestResult{}, err
	}

	var checks []warehouse.AccessCheck
	dataset, err := service.Datasets.Get(config.Project, config.Dataset).Do()
	if err != nil {
		message := explainGoogleAPIError(err).Error()
		checks = append(checks,
			warehouse.AccessCheck{ID: "connect", OK: false, Message: message},
			warehouse.AccessCheck{ID: "read", OK: false, Message: message},
			warehouse.AccessCheck{ID: "create_table", OK: false, Message: "Skipped because connect failed."},
			warehouse.AccessCheck{ID: "query_in_location", OK: false, Message: "Skipped because connect failed."},
		)
		return warehouse.NewTestResult(connector.Type(), checks), nil
	}
	checks = append(checks, warehouse.AccessCheck{ID: "connect", OK: true})

	if _, err := service.Tables.List(config.Project, config.Dataset).MaxResults(1).Do(); err != nil {
		checks = append(checks, warehouse.AccessCheck{ID: "read", OK: false, Message: explainGoogleAPIError(err).Error()})
	} else {
		checks = append(checks, warehouse.AccessCheck{ID: "read", OK: true})
	}

	tableID := fmt.Sprintf("__segmentstream_probe_%d", time.Now().UTC().UnixNano())
	created := false
	_, err = service.Tables.Insert(config.Project, config.Dataset, &bq.Table{
		TableReference: &bq.TableReference{
			ProjectId: config.Project,
			DatasetId: config.Dataset,
			TableId:   tableID,
		},
		Schema: &bq.TableSchema{
			Fields: []*bq.TableFieldSchema{
				{Name: "probe_id", Type: "STRING"},
			},
		},
	}).Do()
	if err != nil {
		checks = append(checks, warehouse.AccessCheck{ID: "create_table", OK: false, Message: explainGoogleAPIError(err).Error()})
	} else {
		created = true
		checks = append(checks, warehouse.AccessCheck{ID: "create_table", OK: true})
	}
	if created {
		if err := service.Tables.Delete(config.Project, config.Dataset, tableID).Do(); err != nil {
			checks[len(checks)-1].Message = "Probe table was created, but cleanup failed: " + explainGoogleAPIError(err).Error()
		}
	}

	queryLocation := config.Location
	if queryLocation == "" {
		queryLocation = dataset.Location
	}
	_, err = service.Jobs.Query(config.Project, &bq.QueryRequest{
		Query:        "select 1 as ok",
		UseLegacySql: boolPointer(false),
		Location:     queryLocation,
	}).Do()
	if err != nil {
		checks = append(checks, warehouse.AccessCheck{ID: "query_in_location", OK: false, Message: explainGoogleAPIError(err).Error()})
	} else {
		checks = append(checks, warehouse.AccessCheck{ID: "query_in_location", OK: true})
	}

	return warehouse.NewTestResult(connector.Type(), checks), nil
}

func (connector Connector) browseProjects(ctx context.Context, service *bq.Service) (warehouse.BrowseResult, error) {
	var children []warehouse.BrowseChild
	err := service.Projects.List().Pages(ctx, func(response *bq.ProjectList) error {
		for _, item := range response.Projects {
			children = append(children, warehouse.BrowseChild{
				ID:           item.Id,
				FriendlyName: item.FriendlyName,
			})
		}
		return nil
	})
	if err != nil {
		return warehouse.BrowseResult{}, fmt.Errorf("list BigQuery projects: %w", explainGoogleAPIError(err))
	}
	return warehouse.NewBrowseResult(connector.Type(), "project", "", children), nil
}

func (connector Connector) browseDatasets(ctx context.Context, service *bq.Service, projectID string) (warehouse.BrowseResult, error) {
	var children []warehouse.BrowseChild
	err := service.Datasets.List(projectID).Pages(ctx, func(response *bq.DatasetList) error {
		for _, item := range response.Datasets {
			child := warehouse.BrowseChild{
				FriendlyName: item.FriendlyName,
				Location:     item.Location,
			}
			if item.DatasetReference != nil {
				child.ID = item.DatasetReference.DatasetId
			}
			children = append(children, child)
		}
		return nil
	})
	if err != nil {
		return warehouse.BrowseResult{}, fmt.Errorf("list BigQuery datasets: %w", explainGoogleAPIError(err))
	}
	return warehouse.NewBrowseResult(connector.Type(), "dataset", projectID, children), nil
}

func (connector Connector) browseTables(ctx context.Context, service *bq.Service, projectID, datasetID string) (warehouse.BrowseResult, error) {
	var children []warehouse.BrowseChild
	err := service.Tables.List(projectID, datasetID).Pages(ctx, func(response *bq.TableList) error {
		for _, item := range response.Tables {
			child := warehouse.BrowseChild{
				FriendlyName: item.FriendlyName,
				Type:         item.Type,
			}
			if item.TableReference != nil {
				child.ID = item.TableReference.TableId
			}
			children = append(children, child)
		}
		return nil
	})
	if err != nil {
		return warehouse.BrowseResult{}, fmt.Errorf("list BigQuery tables: %w", explainGoogleAPIError(err))
	}
	return warehouse.NewBrowseResult(connector.Type(), "table", joinBrowsePath(projectID, datasetID), children), nil
}

func (connector Connector) browseTableSchema(ctx context.Context, service *bq.Service, projectID, datasetID, tableID string) (warehouse.BrowseResult, error) {
	table, err := service.Tables.Get(projectID, datasetID, tableID).View("BASIC").Do()
	if err != nil {
		return warehouse.BrowseResult{}, fmt.Errorf("get BigQuery table schema: %w", explainGoogleAPIError(err))
	}
	result := warehouse.NewBrowseResult(connector.Type(), "schema", joinBrowsePath(projectID, datasetID, tableID), []warehouse.BrowseChild{})
	if table.Schema != nil {
		result.Schema = browseSchemaFields(table.Schema.Fields)
	}
	return result, nil
}

func parseBrowsePath(path string) ([]string, error) {
	trimmed := strings.Trim(strings.TrimSpace(path), "/")
	if trimmed == "" {
		return nil, nil
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) > 3 {
		return nil, fmt.Errorf("invalid BigQuery browse path %q; use <project>, <project>/<dataset>, or <project>/<dataset>/<table>", path)
	}
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
		if parts[i] == "" {
			return nil, fmt.Errorf("invalid BigQuery browse path %q; path segments must not be empty", path)
		}
	}
	return parts, nil
}

func joinBrowsePath(parts ...string) string {
	return strings.Join(parts, "/")
}

func browseSchemaFields(fields []*bq.TableFieldSchema) []warehouse.BrowseField {
	result := make([]warehouse.BrowseField, 0, len(fields))
	for _, field := range fields {
		if field == nil {
			continue
		}
		result = append(result, warehouse.BrowseField{
			Name:        field.Name,
			Type:        field.Type,
			Mode:        field.Mode,
			Description: field.Description,
			Fields:      browseSchemaFields(field.Fields),
		})
	}
	return result
}

func newService(ctx context.Context, credentialPath string) (*bq.Service, error) {
	if strings.TrimSpace(credentialPath) == "" {
		return nil, errors.New("BigQuery credential path is required")
	}
	service, err := bq.NewService(ctx, option.WithCredentialsFile(credentialPath), option.WithScopes(bq.BigqueryScope))
	if err != nil {
		return nil, fmt.Errorf("create BigQuery client: %w", err)
	}
	return service, nil
}

func explainGoogleAPIError(err error) error {
	var googleErr *googleapi.Error
	if errors.As(err, &googleErr) {
		if googleErr.Message != "" {
			return fmt.Errorf("BigQuery API returned %d: %s", googleErr.Code, googleErr.Message)
		}
		return fmt.Errorf("BigQuery API returned %d", googleErr.Code)
	}
	return err
}

func isHTTPStatus(err error, status int) bool {
	var googleErr *googleapi.Error
	return errors.As(err, &googleErr) && googleErr.Code == status
}

func suggestDataset(dataset string) string {
	if dataset == "" {
		return ""
	}
	replacer := strings.NewReplacer("-", "_", " ", "_", ".", "_")
	return replacer.Replace(dataset)
}

func boolPointer(value bool) *bool {
	return &value
}
