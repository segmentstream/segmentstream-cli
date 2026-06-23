package bigquery

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/segmentstream/segmentstream-cli/cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/cli/internal/project"
	"github.com/segmentstream/segmentstream-cli/cli/internal/warehouse"
	bq "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

type Connector struct {
	newService func(context.Context, string) (*bq.Service, error)
}

func NewConnector() Connector {
	return Connector{newService: newService}
}

func (connector Connector) Type() string {
	return "bigquery"
}

func (connector Connector) Browse(ctx context.Context, credentialPath string, path string) (warehouse.BrowseResult, error) {
	parts, err := parseBrowsePath(path)
	if err != nil {
		return warehouse.BrowseResult{}, err
	}
	service, err := connector.service(ctx, credentialPath)
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

func (connector Connector) ValidateConfiguration(ctx context.Context, credentialPath string, config project.Warehouse, options warehouse.ConfigureOptions) (warehouse.ConfigureResult, error) {
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

	service, err := connector.service(ctx, credentialPath)
	if err != nil {
		return warehouse.ConfigureResult{}, err
	}
	dataset, err := service.Datasets.Get(config.Project, config.Dataset).Do()
	if err != nil {
		if isHTTPStatus(err, 404) {
			if !options.CreateDataset {
				validations = append(validations, warehouse.Validation{
					ID:      "dataset_exists",
					Field:   "warehouse.dataset",
					Status:  "not_found",
					Message: fmt.Sprintf("Dataset %s:%s does not exist in %s.", config.Project, config.Dataset, location),
				})
				diagnostics = append(diagnostics, cliresult.Diagnostic{
					ID:         "missing_dataset",
					Field:      "warehouse.dataset",
					Message:    fmt.Sprintf("Dataset %s:%s does not exist in %s.", config.Project, config.Dataset, location),
					Suggestion: fmt.Sprintf("segmentstream warehouse configure --project %s --dataset %s --location %s --create-dataset", config.Project, config.Dataset, location),
				})
				return warehouse.NewConfigureResult(connector.Type(), validations, diagnostics), nil
			}
			createdDataset, err := service.Datasets.Insert(config.Project, &bq.Dataset{
				DatasetReference: &bq.DatasetReference{
					ProjectId: config.Project,
					DatasetId: config.Dataset,
				},
				Location: location,
			}).Do()
			if err != nil {
				if isHTTPStatus(err, 409) {
					dataset, err = service.Datasets.Get(config.Project, config.Dataset).Do()
					if err != nil {
						return warehouse.ConfigureResult{}, fmt.Errorf("check BigQuery dataset after create conflict: %w", explainGoogleAPIError(err))
					}
					return validateExistingDataset(connector.Type(), config, location, dataset, validations), nil
				}
				return warehouse.ConfigureResult{}, fmt.Errorf("create BigQuery dataset: %w", explainGoogleAPIError(err))
			}
			createdLocation := location
			if createdDataset.Location != "" {
				createdLocation = createdDataset.Location
			}
			validations = append(validations, warehouse.Validation{
				ID:      "dataset_exists",
				Field:   "warehouse.dataset",
				Status:  "created",
				Message: fmt.Sprintf("Created dataset %s:%s in %s.", config.Project, config.Dataset, createdLocation),
			})
			return warehouse.NewConfigureResult(connector.Type(), validations, nil), nil
		}
		return warehouse.ConfigureResult{}, fmt.Errorf("check BigQuery dataset: %w", explainGoogleAPIError(err))
	}
	return validateExistingDataset(connector.Type(), config, location, dataset, validations), nil
}

func validateExistingDataset(warehouseType string, config project.Warehouse, location string, dataset *bq.Dataset, validations []warehouse.Validation) warehouse.ConfigureResult {
	var diagnostics []cliresult.Diagnostic
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
	return warehouse.NewConfigureResult(warehouseType, validations, diagnostics)
}

func (connector Connector) Test(ctx context.Context, credentialPath string, config project.Warehouse) (warehouse.TestResult, error) {
	service, err := connector.service(ctx, credentialPath)
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

func (connector Connector) Query(ctx context.Context, credentialPath string, config project.Warehouse, options warehouse.QueryOptions) ([]map[string]any, error) {
	service, err := connector.service(ctx, credentialPath)
	if err != nil {
		return nil, err
	}

	queryLocation := strings.TrimSpace(config.Location)
	if queryLocation == "" {
		queryLocation = project.DefaultLocation
	}
	dryRunQuery := &bq.JobConfigurationQuery{
		Query:        options.SQL,
		UseLegacySql: boolPointer(false),
	}
	if options.MaximumBytesBilled > 0 {
		dryRunQuery.MaximumBytesBilled = options.MaximumBytesBilled
	}
	dryRunJob, err := service.Jobs.Insert(config.Project, &bq.Job{
		JobReference: &bq.JobReference{
			ProjectId: config.Project,
			Location:  queryLocation,
		},
		Configuration: &bq.JobConfiguration{
			DryRun: true,
			Query:  dryRunQuery,
		},
	}).Context(ctx).Do()
	if err != nil {
		return nil, warehouse.NewQueryError(
			"query_dry_run_failed",
			"--sql",
			fmt.Sprintf("BigQuery dry run failed: %s", explainGoogleAPIError(err).Error()),
			"",
		)
	}
	statementType := bigQueryStatementType(dryRunJob)
	if statementType != "SELECT" {
		message := "Only read-only SELECT queries are supported."
		if statementType != "" {
			message = fmt.Sprintf("Only read-only SELECT queries are supported; BigQuery reported statement type %s.", statementType)
		}
		return nil, warehouse.NewQueryError(
			"non_select_query",
			"--sql",
			message,
			"Use a SELECT statement without DDL, DML, scripts, calls, exports, or assertions.",
		)
	}

	queryRequest := &bq.QueryRequest{
		Query:        options.SQL,
		UseLegacySql: boolPointer(false),
		Location:     queryLocation,
		MaxResults:   options.MaxRows,
		TimeoutMs:    int64(options.Timeout / time.Millisecond),
	}
	if options.MaximumBytesBilled > 0 {
		queryRequest.MaximumBytesBilled = options.MaximumBytesBilled
	}
	response, err := service.Jobs.Query(config.Project, queryRequest).Context(ctx).Do()
	if err != nil {
		return nil, warehouse.NewQueryError(
			"query_execution_failed",
			"--sql",
			fmt.Sprintf("BigQuery query failed: %s", explainGoogleAPIError(err).Error()),
			"",
		)
	}
	if response != nil && !response.JobComplete {
		cancelBigQueryQuery(ctx, service, config.Project, queryLocation, response.JobReference)
		return nil, warehouse.NewQueryError(
			"query_timeout",
			"--timeout",
			"BigQuery query did not complete before the timeout.",
			"Increase --timeout or add a more selective WHERE/LIMIT clause.",
		)
	}
	rows, err := bigQueryRows(response)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func bigQueryStatementType(job *bq.Job) string {
	if job == nil || job.Statistics == nil || job.Statistics.Query == nil {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(job.Statistics.Query.StatementType))
}

func cancelBigQueryQuery(ctx context.Context, service *bq.Service, projectID, location string, reference *bq.JobReference) {
	if reference == nil || reference.JobId == "" {
		return
	}
	cancelProject := projectID
	if reference.ProjectId != "" {
		cancelProject = reference.ProjectId
	}
	call := service.Jobs.Cancel(cancelProject, reference.JobId).Context(ctx)
	cancelLocation := location
	if reference.Location != "" {
		cancelLocation = reference.Location
	}
	if cancelLocation != "" {
		call = call.Location(cancelLocation)
	}
	_, _ = call.Do()
}

func bigQueryRows(response *bq.QueryResponse) ([]map[string]any, error) {
	if response == nil || response.Schema == nil || len(response.Schema.Fields) == 0 {
		return []map[string]any{}, nil
	}
	fields := response.Schema.Fields
	names := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for index, field := range fields {
		if field == nil || strings.TrimSpace(field.Name) == "" {
			return nil, warehouse.NewQueryError(
				"missing_column_name",
				"--sql",
				fmt.Sprintf("Query result column %d does not have a name.", index+1),
				"Alias every selected expression with a unique name.",
			)
		}
		if _, ok := seen[field.Name]; ok {
			return nil, warehouse.NewQueryError(
				"duplicate_column_names",
				"--sql",
				fmt.Sprintf("Query returned duplicate column name %q.", field.Name),
				"Alias selected columns with unique names.",
			)
		}
		seen[field.Name] = struct{}{}
		names = append(names, field.Name)
	}

	rows := make([]map[string]any, 0, len(response.Rows))
	for _, row := range response.Rows {
		item := make(map[string]any, len(fields))
		for index, field := range fields {
			var value any
			if row != nil && index < len(row.F) && row.F[index] != nil {
				value = convertBigQueryValue(field, row.F[index].V)
			}
			item[names[index]] = value
		}
		rows = append(rows, item)
	}
	return rows, nil
}

func convertBigQueryValue(field *bq.TableFieldSchema, value any) any {
	if value == nil || field == nil {
		return value
	}
	if strings.EqualFold(field.Mode, "REPEATED") {
		values, ok := value.([]any)
		if !ok {
			return value
		}
		elementField := *field
		elementField.Mode = ""
		converted := make([]any, 0, len(values))
		for _, item := range values {
			converted = append(converted, convertBigQueryValue(&elementField, unwrapBigQueryCellValue(item)))
		}
		return converted
	}
	if strings.EqualFold(field.Type, "RECORD") || strings.EqualFold(field.Type, "STRUCT") {
		if record, ok := convertBigQueryRecord(field.Fields, value); ok {
			return record
		}
	}
	return value
}

func convertBigQueryRecord(fields []*bq.TableFieldSchema, value any) (map[string]any, bool) {
	record, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	cells, ok := record["f"].([]any)
	if !ok {
		return nil, false
	}
	item := make(map[string]any, len(fields))
	for index, field := range fields {
		if field == nil || field.Name == "" {
			continue
		}
		var value any
		if index < len(cells) {
			value = convertBigQueryValue(field, unwrapBigQueryCellValue(cells[index]))
		}
		item[field.Name] = value
	}
	return item, true
}

func unwrapBigQueryCellValue(value any) any {
	cell, ok := value.(map[string]any)
	if !ok {
		return value
	}
	if unwrapped, ok := cell["v"]; ok {
		return unwrapped
	}
	return value
}

func (connector Connector) service(ctx context.Context, credentialPath string) (*bq.Service, error) {
	if connector.newService != nil {
		return connector.newService(ctx, credentialPath)
	}
	return newService(ctx, credentialPath)
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
