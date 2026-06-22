package warehouse

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/segmentstream/segmentstream-cli/internal/cliresult"
	"github.com/segmentstream/segmentstream-cli/internal/project"
)

type Connector interface {
	Type() string
	Browse(ctx context.Context, credentialPath string, path string) (BrowseResult, error)
	ValidateConfiguration(ctx context.Context, credentialPath string, config project.Warehouse, options ConfigureOptions) (ConfigureResult, error)
	Test(ctx context.Context, credentialPath string, config project.Warehouse) (TestResult, error)
	Query(ctx context.Context, credentialPath string, config project.Warehouse, options QueryOptions) ([]map[string]any, error)
}

type ConfigureOptions struct {
	CreateDataset bool
}

type QueryOptions struct {
	SQL                string
	MaxRows            int64
	Timeout            time.Duration
	MaximumBytesBilled int64
}

type Registry struct {
	connectors map[string]Connector
}

func NewRegistry(connectors ...Connector) Registry {
	registry := Registry{connectors: make(map[string]Connector, len(connectors))}
	for _, connector := range connectors {
		registry.connectors[connector.Type()] = connector
	}
	return registry
}

func (registry Registry) IsZero() bool {
	return registry.connectors == nil
}

func (registry Registry) Connector(warehouseType string) (Connector, error) {
	connector, ok := registry.connectors[warehouseType]
	if !ok {
		return nil, fmt.Errorf("unsupported warehouse.type %q", warehouseType)
	}
	return connector, nil
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
}

func NewQueryError(id, field, message, suggestion string) QueryError {
	return QueryError{
		Diagnostics: []cliresult.Diagnostic{{
			ID:         id,
			Field:      field,
			Message:    message,
			Suggestion: suggestion,
		}},
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
