package project

import (
	"strings"
	"testing"
)

func TestParseConfigAcceptsValidBigQueryConfig(t *testing.T) {
	config, err := ParseConfig([]byte(`version: 1
warehouse:
  type: bigquery
  auth: production-bigquery
  project: example-project
  dataset: segmentstream
  location: EU
`))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}

	if config.Version != 1 {
		t.Fatalf("Version = %d, want 1", config.Version)
	}
	if config.Warehouse.Type != "bigquery" {
		t.Fatalf("Warehouse.Type = %q, want bigquery", config.Warehouse.Type)
	}
	if config.Warehouse.Location != "EU" {
		t.Fatalf("Warehouse.Location = %q, want EU", config.Warehouse.Location)
	}
}

func TestParseConfigRejectsMissingVersion(t *testing.T) {
	_, err := ParseConfig([]byte(`warehouse:
  type: bigquery
  auth: production-bigquery
  project: example-project
  dataset: segmentstream
`))
	if err == nil {
		t.Fatal("expected missing version error")
	}
	if !strings.Contains(err.Error(), "missing required field version") {
		t.Fatalf("error = %v, want missing version", err)
	}
}

func TestParseConfigRejectsUnsupportedVersion(t *testing.T) {
	_, err := ParseConfig([]byte(`version: 2
warehouse:
  type: bigquery
  auth: production-bigquery
  project: example-project
  dataset: segmentstream
`))
	if err == nil {
		t.Fatal("expected unsupported version error")
	}
	if !strings.Contains(err.Error(), "unsupported version 2") {
		t.Fatalf("error = %v, want unsupported version", err)
	}
}

func TestParseConfigLeavesWarehouseTypeSupportToRegistry(t *testing.T) {
	config, err := ParseConfig([]byte(`version: 1
warehouse:
  type: snowflake
  auth: production-snowflake
  project: example-project
  dataset: segmentstream
`))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}
	if config.Warehouse.Type != "snowflake" {
		t.Fatalf("Warehouse.Type = %q, want snowflake", config.Warehouse.Type)
	}
}

func TestParseConfigParsesRequiresWithoutEnforcing(t *testing.T) {
	config, err := ParseConfig([]byte(`version: 1
requires:
  segmentstream: ">=99.0.0"
warehouse:
  type: bigquery
  auth: production-bigquery
  project: example-project
  dataset: segmentstream
`))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}
	if config.Requires.SegmentStream != ">=99.0.0" {
		t.Fatalf("Requires.SegmentStream = %q, want >=99.0.0", config.Requires.SegmentStream)
	}
}

func TestParseConfigDefaultsLocation(t *testing.T) {
	config, err := ParseConfig([]byte(`version: 1
warehouse:
  type: bigquery
  auth: production-bigquery
  project: example-project
  dataset: segmentstream
`))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}
	if config.Warehouse.Location != DefaultLocation {
		t.Fatalf("Warehouse.Location = %q, want %q", config.Warehouse.Location, DefaultLocation)
	}
}

func TestParseConfigRejectsPlaceholderProject(t *testing.T) {
	_, err := ParseConfig([]byte(`version: 1
warehouse:
  type: bigquery
  auth: production-bigquery
  project: your-gcp-project
  dataset: segmentstream
`))
	if err == nil {
		t.Fatal("expected placeholder project error")
	}
	if !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("error = %v, want placeholder message", err)
	}
}

func TestParseConfigLeavesDatasetRulesToWarehouseProvider(t *testing.T) {
	config, err := ParseConfig([]byte(`version: 1
warehouse:
  type: bigquery
  auth: production-bigquery
  project: example-project
  dataset: segmentstream-new
`))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}
	if config.Warehouse.Dataset != "segmentstream-new" {
		t.Fatalf("Warehouse.Dataset = %q, want segmentstream-new", config.Warehouse.Dataset)
	}
}

func TestParseConfigParsesSourcesWithoutDagsterValidation(t *testing.T) {
	config, err := ParseConfig([]byte(`version: 1
warehouse:
  type: bigquery
  auth: production-bigquery
  project: example-project
  dataset: segmentstream
sources:
  - name: ga4
    path: ./sources/ga4
`))
	if err != nil {
		t.Fatalf("ParseConfig should leave sources to Dagster, got error: %v", err)
	}
	if len(config.Sources) != 1 {
		t.Fatalf("Sources length = %d, want 1", len(config.Sources))
	}
	if config.Sources[0].Name != "ga4" || config.Sources[0].Path != "./sources/ga4" {
		t.Fatalf("Sources[0] = %+v, want ga4 source", config.Sources[0])
	}
}
