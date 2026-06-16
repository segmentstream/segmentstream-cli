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

func TestParseConfigRejectsNonBigQueryWarehouse(t *testing.T) {
	_, err := ParseConfig([]byte(`version: 1
warehouse:
  type: snowflake
  auth: production-snowflake
  project: example-project
  dataset: segmentstream
`))
	if err == nil {
		t.Fatal("expected unsupported warehouse error")
	}
	if !strings.Contains(err.Error(), "only bigquery is supported") {
		t.Fatalf("error = %v, want bigquery-only error", err)
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
