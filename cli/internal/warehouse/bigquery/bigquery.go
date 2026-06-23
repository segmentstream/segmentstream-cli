package bigquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/segmentstream/segmentstream-cli/cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/cli/internal/credentials"
	"github.com/segmentstream/segmentstream-cli/cli/internal/project"
	"github.com/segmentstream/segmentstream-cli/cli/internal/warehouse"
	"github.com/segmentstream/segmentstream-cli/cli/internal/warehouse/bigquery/googleoauth"
	bq "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const (
	warehouseType   = "bigquery"
	displayName     = "BigQuery"
	defaultAuthName = "default-bigquery"
	defaultLocation = "US"
)

var backtickTableReferencePattern = regexp.MustCompile("`([^`]+)`")

type Options struct {
	OAuthLogin warehouse.OAuthLogin
}

type Connector struct {
	newService func(context.Context, string) (*bq.Service, error)
	oauthLogin warehouse.OAuthLogin
}

type accessMarker struct {
	Project   string `json:"project"`
	Dataset   string `json:"dataset"`
	Location  string `json:"location"`
	CheckedAt string `json:"checked_at"`
}

func NewConnector(options ...Options) Connector {
	opts := Options{}
	if len(options) > 0 {
		opts = options[0]
	}
	return Connector{
		newService: newService,
		oauthLogin: opts.OAuthLogin,
	}
}

func (connector Connector) Type() string {
	return warehouseType
}

func (connector Connector) DisplayName() string {
	return displayName
}

func (connector Connector) DefaultAuthName() string {
	return defaultAuthName
}

func (connector Connector) AuthMethods() []string {
	return []string{"oauth", "service_account_key"}
}

func (connector Connector) SelectWarehouseAccept() cliresult.NextActionAccept {
	return cliresult.NextActionAccept{
		Method:  warehouseType,
		Label:   "Use BigQuery",
		Command: "segmentstream init --warehouse bigquery",
		Value:   warehouseType,
	}
}

func (connector Connector) AuthenticateAccepts() []cliresult.NextActionAccept {
	return []cliresult.NextActionAccept{
		{
			Method:  "oauth",
			Label:   "Google OAuth",
			Command: "segmentstream warehouse auth login",
			Inputs: []cliresult.NextActionInput{
				{
					Name:     "port",
					Type:     "integer",
					Flag:     "--port",
					Label:    "OAuth loopback callback port",
					Required: false,
				},
			},
		},
		{
			Method:  "service_account_key",
			Label:   "Service-account key file",
			Command: "segmentstream warehouse auth",
			Inputs: []cliresult.NextActionInput{
				{
					Name:     "path",
					Type:     "filepath",
					Flag:     "--service-account-key",
					Label:    "Service-account JSON key path",
					Required: true,
				},
			},
		},
	}
}

func (connector Connector) ConfigureAccept() cliresult.NextActionAccept {
	return cliresult.NextActionAccept{
		Method:  "warehouse_config",
		Label:   "Configure BigQuery warehouse",
		Command: "segmentstream warehouse configure",
		Inputs: []cliresult.NextActionInput{
			{
				Name:     "project",
				Type:     "string",
				Flag:     "--project",
				Label:    "Google Cloud project ID",
				Required: true,
			},
			{
				Name:     "dataset",
				Type:     "string",
				Flag:     "--dataset",
				Label:    "BigQuery dataset ID",
				Required: true,
			},
			{
				Name:     "location",
				Type:     "string",
				Flag:     "--location",
				Label:    "BigQuery dataset location",
				Required: true,
			},
			{
				Name:     "create_dataset",
				Type:     "boolean",
				Flag:     "--create-dataset",
				Label:    "Create the BigQuery dataset if missing",
				Required: false,
			},
		},
	}
}

func (connector Connector) CredentialPath(store credentials.Store, name string) (string, error) {
	return store.CredentialPath(warehouseType, name)
}

func (connector Connector) HasCredential(store credentials.Store, name string) (bool, error) {
	return store.HasCredential(warehouseType, name)
}

func (connector Connector) SaveServiceAccountKey(store credentials.Store, name, sourcePath string) (string, error) {
	if strings.TrimSpace(sourcePath) == "" {
		return "", errors.New("--service-account-key is required")
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", fmt.Errorf("read service account key: %w", err)
	}
	if err := validateServiceAccountJSON(data); err != nil {
		return "", err
	}
	return store.SaveCredentialData(warehouseType, name, data)
}

func (connector Connector) LoginOAuth(ctx context.Context, out io.Writer, options warehouse.LoginOptions) ([]byte, error) {
	if connector.oauthLogin != nil {
		return connector.oauthLogin(ctx, out, options)
	}
	credential, err := googleoauth.LoginWithOptions(ctx, out, googleoauth.LoginOptions{
		Port: options.Port,
	})
	if err != nil {
		return nil, err
	}
	return googleAuthorizedUserCredentialJSON(credential)
}

func (connector Connector) SaveOAuthCredential(store credentials.Store, name string, credential []byte) (string, error) {
	return store.SaveCredentialData(warehouseType, name, credential)
}

func (connector Connector) HasMatchingAccessMarker(store credentials.Store, name string, config project.Warehouse) (bool, error) {
	var marker accessMarker
	found, err := store.ReadAccessMarker(warehouseType, name, &marker)
	if err != nil || !found {
		return found, err
	}
	return marker.Project == config.Project &&
		marker.Dataset == config.Dataset &&
		strings.EqualFold(marker.Location, config.Location), nil
}

func (connector Connector) SaveAccessMarker(store credentials.Store, name string, config project.Warehouse) error {
	return store.SaveAccessMarker(warehouseType, name, accessMarker{
		Project:   config.Project,
		Dataset:   config.Dataset,
		Location:  config.Location,
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

func (connector Connector) ConfigDiagnostics(config project.Warehouse) []cliresult.Diagnostic {
	var diagnostics []cliresult.Diagnostic
	if config.Auth == "" {
		diagnostics = append(diagnostics, cliresult.Diagnostic{
			ID:         "missing_auth",
			Field:      "warehouse.auth",
			Message:    "warehouse.auth is required.",
			Suggestion: defaultAuthName,
		})
	}
	if config.Project == "" || config.Project == "your-gcp-project" {
		diagnostics = append(diagnostics, cliresult.Diagnostic{
			ID:      "missing_project",
			Field:   "warehouse.project",
			Message: "warehouse.project must be set to a real Google Cloud project ID.",
		})
	}
	if config.Dataset == "" {
		diagnostics = append(diagnostics, cliresult.Diagnostic{
			ID:      "missing_dataset",
			Field:   "warehouse.dataset",
			Message: "warehouse.dataset must be set to a BigQuery dataset ID.",
		})
	} else if err := validateDatasetID(config.Dataset); err != nil {
		diagnostics = append(diagnostics, cliresult.Diagnostic{
			ID:         "invalid_dataset",
			Field:      "warehouse.dataset",
			Message:    err.Error(),
			Suggestion: suggestDataset(config.Dataset),
		})
	}
	if config.Location == "" {
		diagnostics = append(diagnostics, cliresult.Diagnostic{
			ID:         "missing_location",
			Field:      "warehouse.location",
			Message:    "warehouse.location must be set to the BigQuery dataset location.",
			Suggestion: defaultLocation,
		})
	}
	return diagnostics
}

func (connector Connector) RuntimeEnvironment(config project.Warehouse) []warehouse.EnvVar {
	return []warehouse.EnvVar{
		{Name: "GOOGLE_APPLICATION_CREDENTIALS", Value: "/home/segmentstream/.segmentstream/bigquery/" + config.Auth + ".json"},
		{Name: "SEGMENTSTREAM_BQ_PROJECT", Value: config.Project},
		{Name: "SEGMENTSTREAM_BQ_DATASET", Value: config.Dataset},
		{Name: "SEGMENTSTREAM_BQ_LOCATION", Value: config.Location},
	}
}

func (connector Connector) DBTProfileYAML(config project.Warehouse) string {
	_ = config
	return `segmentstream:
  target: default
  outputs:
    default:
      type: bigquery
      method: oauth
      project: "{{ env_var('SEGMENTSTREAM_BQ_PROJECT') }}"
      dataset: "{{ env_var('SEGMENTSTREAM_BQ_DATASET') }}"
      location: "{{ env_var('SEGMENTSTREAM_BQ_LOCATION', 'US') }}"
      threads: 4
      priority: interactive
      retries: 1
      scopes:
        - https://www.googleapis.com/auth/bigquery
`
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

	if err := validateDatasetID(config.Dataset); err != nil {
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

func (connector Connector) Destroy(ctx context.Context, credentialPath string, config project.Warehouse, options warehouse.DestroyOptions) (warehouse.DestroyResult, error) {
	service, err := connector.service(ctx, credentialPath)
	if err != nil {
		return warehouse.DestroyResult{}, err
	}
	projectID := strings.TrimSpace(config.Project)
	datasetID := strings.TrimSpace(config.Dataset)

	if _, err := service.Datasets.Get(projectID, datasetID).Context(ctx).Do(); err != nil {
		if isHTTPStatus(err, 404) {
			return warehouse.NewDestroyResult(
				connector.Type(),
				projectID,
				datasetID,
				"absent",
				fmt.Sprintf("Dataset %s:%s does not exist; nothing to destroy.", projectID, datasetID),
			), nil
		}
		return warehouse.DestroyResult{}, fmt.Errorf("check BigQuery dataset before destroy: %w", explainGoogleAPIError(err))
	}

	tables, err := service.Tables.List(projectID, datasetID).MaxResults(1).Context(ctx).Do()
	if err != nil {
		return warehouse.DestroyResult{}, fmt.Errorf("list BigQuery dataset contents before destroy: %w", explainGoogleAPIError(err))
	}
	if tables != nil && len(tables.Tables) > 0 && !options.Force {
		found := "an object"
		if tables.Tables[0] != nil && tables.Tables[0].TableReference != nil && tables.Tables[0].TableReference.TableId != "" {
			found = fmt.Sprintf("object %q", tables.Tables[0].TableReference.TableId)
		}
		return warehouse.NewDestroyResult(
			connector.Type(),
			projectID,
			datasetID,
			"not_empty",
			fmt.Sprintf("Dataset %s:%s is not empty (%s found); rerun with --force to delete it and its contents.", projectID, datasetID, found),
		), nil
	}

	deleteCall := service.Datasets.Delete(projectID, datasetID).Context(ctx)
	if options.Force {
		deleteCall = deleteCall.DeleteContents(true)
	}
	if err := deleteCall.Do(); err != nil {
		if isHTTPStatus(err, 404) {
			return warehouse.NewDestroyResult(
				connector.Type(),
				projectID,
				datasetID,
				"absent",
				fmt.Sprintf("Dataset %s:%s does not exist; nothing to destroy.", projectID, datasetID),
			), nil
		}
		return warehouse.DestroyResult{}, fmt.Errorf("delete BigQuery dataset: %w", explainGoogleAPIError(err))
	}
	return warehouse.NewDestroyResult(
		connector.Type(),
		projectID,
		datasetID,
		"destroyed",
		fmt.Sprintf("Destroyed dataset %s:%s.", projectID, datasetID),
	), nil
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
		queryLocation = defaultLocation
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
		return nil, connector.queryFailure(ctx, service, config, options.SQL, "query_dry_run_failed", "--sql", "BigQuery dry run failed", err)
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
		return nil, connector.queryFailure(ctx, service, config, options.SQL, "query_execution_failed", "--sql", "BigQuery query failed", err)
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

func (connector Connector) queryFailure(ctx context.Context, service *bq.Service, config project.Warehouse, sql, id, field, prefix string, err error) warehouse.QueryError {
	providerMessage := explainGoogleAPIError(err).Error()
	message := fmt.Sprintf("%s: %s", prefix, providerMessage)
	if !isBigQueryLocationError(providerMessage) {
		return warehouse.NewQueryError(id, field, message, "")
	}

	queryLocation := strings.TrimSpace(config.Location)
	if queryLocation == "" {
		queryLocation = defaultLocation
	}
	inference, inferred := connector.inferSingleSourceLocation(ctx, service, sql, config.Project)
	suggestion := "BigQuery reported a location mismatch. Confirm the raw dataset location with segmentstream warehouse browse, then recreate the configured dataset in that location."
	var actions []cliresult.Action
	if inferred && inference.Location != "" && !strings.EqualFold(inference.Location, queryLocation) {
		message = fmt.Sprintf("%s: BigQuery looked for %s in warehouse.location %s, but %s in %s. Raw BigQuery error: %s",
			prefix,
			formatBigQuerySourceDatasets(inference.References),
			queryLocation,
			sourceDatasetVerb(inference.References),
			inference.Location,
			providerMessage,
		)
		configureCommand := fmt.Sprintf("segmentstream warehouse configure --project %s --dataset %s --location %s --create-dataset --json", config.Project, config.Dataset, inference.Location)
		suggestion = fmt.Sprintf("Recreate the configured SegmentStream dataset %s.%s in %s, then rerun warehouse test.", config.Project, config.Dataset, inference.Location)
		actions = []cliresult.Action{
			{
				Type:    "run_command",
				Label:   "Destroy the configured dataset",
				Command: "segmentstream warehouse destroy --json",
			},
			{
				Type:    "run_command",
				Label:   "Recreate the configured dataset in the source location",
				Command: configureCommand,
			},
		}
	}
	return warehouse.NewQueryErrorWithActions("query_location_mismatch", field, message, suggestion, actions)
}

func isBigQueryLocationError(message string) bool {
	lower := strings.ToLower(message)
	if !strings.Contains(lower, "location") {
		return false
	}
	for _, marker := range []string{
		"not found",
		"different locations",
		"same location",
		"location mismatch",
		"wrong location",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

type bigQueryLocationInference struct {
	Location   string
	References []bigQueryTableReference
}

func (connector Connector) inferSingleSourceLocation(ctx context.Context, service *bq.Service, sql, defaultProject string) (bigQueryLocationInference, bool) {
	refs := extractBigQueryTableReferences(sql, defaultProject)
	if len(refs) == 0 {
		return bigQueryLocationInference{}, false
	}
	locations := make(map[string]string)
	for _, ref := range refs {
		dataset, err := service.Datasets.Get(ref.ProjectID, ref.DatasetID).Context(ctx).Do()
		if err != nil || dataset == nil || strings.TrimSpace(dataset.Location) == "" {
			return bigQueryLocationInference{}, false
		}
		normalized := strings.ToUpper(strings.TrimSpace(dataset.Location))
		locations[normalized] = strings.TrimSpace(dataset.Location)
	}
	if len(locations) != 1 {
		return bigQueryLocationInference{}, false
	}
	for _, location := range locations {
		return bigQueryLocationInference{Location: location, References: refs}, true
	}
	return bigQueryLocationInference{}, false
}

func formatBigQuerySourceDatasets(refs []bigQueryTableReference) string {
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		names = append(names, ref.ProjectID+"."+ref.DatasetID)
	}
	if len(names) == 1 {
		return "source dataset " + names[0]
	}
	return "source datasets " + strings.Join(names, ", ")
}

func sourceDatasetVerb(refs []bigQueryTableReference) string {
	if len(refs) == 1 {
		return "it is"
	}
	return "they are"
}

type bigQueryTableReference struct {
	ProjectID string
	DatasetID string
}

func extractBigQueryTableReferences(sql, defaultProject string) []bigQueryTableReference {
	matches := backtickTableReferencePattern.FindAllStringSubmatch(sql, -1)
	refs := make([]bigQueryTableReference, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		parts := strings.Split(strings.TrimSpace(match[1]), ".")
		var projectID, datasetID string
		switch len(parts) {
		case 2:
			projectID = strings.TrimSpace(defaultProject)
			datasetID = strings.TrimSpace(parts[0])
		case 3:
			projectID = strings.TrimSpace(parts[0])
			datasetID = strings.TrimSpace(parts[1])
		default:
			continue
		}
		if projectID == "" || datasetID == "" {
			continue
		}
		key := projectID + "." + datasetID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		refs = append(refs, bigQueryTableReference{
			ProjectID: projectID,
			DatasetID: datasetID,
		})
	}
	return refs
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

func googleAuthorizedUserCredentialJSON(credential googleoauth.Credential) ([]byte, error) {
	if strings.TrimSpace(credential.ClientID) == "" {
		return nil, errors.New("Google OAuth client_id is required")
	}
	if strings.TrimSpace(credential.ClientSecret) == "" {
		return nil, errors.New("Google OAuth client_secret is required")
	}
	if strings.TrimSpace(credential.RefreshToken) == "" {
		return nil, errors.New("Google OAuth refresh_token is required")
	}
	if strings.TrimSpace(credential.TokenURI) == "" {
		return nil, errors.New("Google OAuth token_uri is required")
	}

	payload := struct {
		Type         string   `json:"type"`
		ClientID     string   `json:"client_id"`
		ClientSecret string   `json:"client_secret"`
		RefreshToken string   `json:"refresh_token"`
		TokenURI     string   `json:"token_uri"`
		Scopes       []string `json:"scopes,omitempty"`
	}{
		Type:         "authorized_user",
		ClientID:     strings.TrimSpace(credential.ClientID),
		ClientSecret: strings.TrimSpace(credential.ClientSecret),
		RefreshToken: strings.TrimSpace(credential.RefreshToken),
		TokenURI:     strings.TrimSpace(credential.TokenURI),
		Scopes:       credential.Scopes,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal Google OAuth credential: %w", err)
	}
	return append(data, '\n'), nil
}

func validateServiceAccountJSON(data []byte) error {
	var payload struct {
		Type        string `json:"type"`
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("service account key is not valid JSON: %w", err)
	}
	if payload.Type != "service_account" {
		return fmt.Errorf("service account key has type %q, want service_account", payload.Type)
	}
	if strings.TrimSpace(payload.ClientEmail) == "" || strings.TrimSpace(payload.PrivateKey) == "" {
		return errors.New("service account key is missing client_email or private_key")
	}
	return nil
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

func validateDatasetID(dataset string) error {
	if dataset == "" {
		return errors.New("missing required field warehouse.dataset")
	}
	if len(dataset) > 1024 {
		return errors.New("warehouse.dataset must be 1024 characters or fewer")
	}
	for _, char := range dataset {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '_' {
			continue
		}
		return fmt.Errorf("invalid warehouse.dataset %q; BigQuery dataset IDs may contain only letters, numbers, and underscores", dataset)
	}
	return nil
}

func boolPointer(value bool) *bool {
	return &value
}
