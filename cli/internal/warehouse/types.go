package warehouse

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/segmentstream/segmentstream-cli/cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/cli/internal/credentials"
	"github.com/segmentstream/segmentstream-cli/cli/internal/project"
)

type Provider interface {
	Type() string
	DisplayName() string
	DefaultAuthName() string
	AuthMethods() []string
	SelectWarehouseAccept() cliresult.NextActionAccept
	AuthenticateAccepts() []cliresult.NextActionAccept
	ConfigureAccept() cliresult.NextActionAccept
	CredentialPath(credentials.Store, string) (string, error)
	HasCredential(credentials.Store, string) (bool, error)
	SaveServiceAccountKey(credentials.Store, string, string) (string, error)
	LoginOAuth(context.Context, io.Writer, LoginOptions) ([]byte, error)
	SaveOAuthCredential(credentials.Store, string, []byte) (string, error)
	HasMatchingAccessMarker(credentials.Store, string, project.Warehouse) (bool, error)
	SaveAccessMarker(credentials.Store, string, project.Warehouse) error
	ConfigDiagnostics(project.Warehouse) []cliresult.Diagnostic
	RuntimeEnvironment(project.Warehouse) []EnvVar
	DBTProfileYAML(project.Warehouse) string
	Browse(ctx context.Context, credentialPath string, path string) (BrowseResult, error)
	ValidateConfiguration(ctx context.Context, credentialPath string, config project.Warehouse, options ConfigureOptions) (ConfigureResult, error)
	Destroy(ctx context.Context, credentialPath string, config project.Warehouse, options DestroyOptions) (DestroyResult, error)
	Test(ctx context.Context, credentialPath string, config project.Warehouse) (TestResult, error)
	Query(ctx context.Context, credentialPath string, config project.Warehouse, options QueryOptions) ([]map[string]any, error)
}

type LoginOptions struct {
	Port int
}

type OAuthLogin func(context.Context, io.Writer, LoginOptions) ([]byte, error)

type EnvVar struct {
	Name  string
	Value string
}

type ConfigureOptions struct {
	CreateDataset bool
}

type DestroyOptions struct {
	Force bool
}

type QueryOptions struct {
	SQL                string
	MaxRows            int64
	Timeout            time.Duration
	MaximumBytesBilled int64
}

type Registry struct {
	providers map[string]Provider
}

func NewRegistry(providers ...Provider) Registry {
	registry := Registry{providers: make(map[string]Provider, len(providers))}
	for _, provider := range providers {
		registry.providers[provider.Type()] = provider
	}
	return registry
}

func (registry Registry) IsZero() bool {
	return registry.providers == nil
}

func (registry Registry) Provider(warehouseType string) (Provider, error) {
	provider, ok := registry.providers[warehouseType]
	if !ok {
		return nil, fmt.Errorf("unsupported warehouse.type %q", warehouseType)
	}
	return provider, nil
}

func (registry Registry) Providers() []Provider {
	providers := make([]Provider, 0, len(registry.providers))
	for _, provider := range registry.providers {
		providers = append(providers, provider)
	}
	return providers
}

type BrowseResult struct {
	SchemaVersion string        `json:"schema_version"`
	Warehouse     string        `json:"warehouse"`
	Level         string        `json:"level"`
	Path          string        `json:"path,omitempty"`
	Children      []BrowseChild `json:"children"`
	Schema        []BrowseField `json:"schema,omitempty"`
}

type BrowseChild struct {
	ID           string `json:"id"`
	FriendlyName string `json:"friendly_name,omitempty"`
	Location     string `json:"location,omitempty"`
	Type         string `json:"type,omitempty"`
}

type BrowseField struct {
	Name        string        `json:"name"`
	Type        string        `json:"type"`
	Mode        string        `json:"mode,omitempty"`
	Description string        `json:"description"`
	Fields      []BrowseField `json:"fields,omitempty"`
}

type ConfigureResult struct {
	SchemaVersion string                 `json:"schema_version"`
	Warehouse     string                 `json:"warehouse"`
	Status        string                 `json:"warehouse_config"`
	Validations   []Validation           `json:"validations"`
	Diagnostics   []cliresult.Diagnostic `json:"diagnostics,omitempty"`
}

type DestroyResult struct {
	SchemaVersion string `json:"schema_version"`
	Warehouse     string `json:"warehouse"`
	Project       string `json:"project"`
	Dataset       string `json:"dataset"`
	Status        string `json:"status"`
	Message       string `json:"message"`
}

type Validation struct {
	ID      string `json:"id"`
	Field   string `json:"field,omitempty"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type TestResult struct {
	SchemaVersion string        `json:"schema_version"`
	Warehouse     string        `json:"warehouse"`
	Status        string        `json:"warehouse_access"`
	Checks        []AccessCheck `json:"checks"`
}

type AccessCheck struct {
	ID      string `json:"id"`
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

type QueryError struct {
	Diagnostics []cliresult.Diagnostic
	Actions     []cliresult.Action
}

func NewQueryError(id, field, message, suggestion string) QueryError {
	return NewQueryErrorWithActions(id, field, message, suggestion, nil)
}

func NewQueryErrorWithActions(id, field, message, suggestion string, actions []cliresult.Action) QueryError {
	return QueryError{
		Diagnostics: []cliresult.Diagnostic{{
			ID:         id,
			Field:      field,
			Message:    message,
			Suggestion: suggestion,
		}},
		Actions: actions,
	}
}

func (err QueryError) Error() string {
	messages := make([]string, 0, len(err.Diagnostics))
	for _, diagnostic := range err.Diagnostics {
		if diagnostic.Message != "" {
			messages = append(messages, diagnostic.Message)
		}
	}
	if len(messages) == 0 {
		return "warehouse query failed"
	}
	return strings.Join(messages, "; ")
}

func NewBrowseResult(warehouseType, level, path string, children []BrowseChild) BrowseResult {
	return BrowseResult{
		SchemaVersion: cliresult.SchemaVersion,
		Warehouse:     warehouseType,
		Level:         level,
		Path:          path,
		Children:      children,
	}
}

func NewConfigureResult(warehouseType string, validations []Validation, diagnostics []cliresult.Diagnostic) ConfigureResult {
	status := "valid"
	if len(diagnostics) > 0 {
		status = "invalid"
	}
	return ConfigureResult{
		SchemaVersion: cliresult.SchemaVersion,
		Warehouse:     warehouseType,
		Status:        status,
		Validations:   validations,
		Diagnostics:   diagnostics,
	}
}

func NewDestroyResult(warehouseType, projectID, datasetID, status, message string) DestroyResult {
	return DestroyResult{
		SchemaVersion: cliresult.SchemaVersion,
		Warehouse:     warehouseType,
		Project:       projectID,
		Dataset:       datasetID,
		Status:        status,
		Message:       message,
	}
}

func NewTestResult(warehouseType string, checks []AccessCheck) TestResult {
	status := "satisfied"
	for _, check := range checks {
		if !check.OK {
			status = "failed"
			break
		}
	}
	return TestResult{
		SchemaVersion: cliresult.SchemaVersion,
		Warehouse:     warehouseType,
		Status:        status,
		Checks:        checks,
	}
}

func AllChecksOK(checks []AccessCheck) bool {
	for _, check := range checks {
		if !check.OK {
			return false
		}
	}
	return len(checks) > 0
}
