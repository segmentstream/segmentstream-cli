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

func TestParseConfigParsesIdentityKeys(t *testing.T) {
	config, err := ParseConfig([]byte(`version: 1
warehouse:
  type: bigquery
  auth: production-bigquery
  project: example-project
  dataset: segmentstream
identity:
  keys:
    - name: " user_id "
      tier: " deterministic "
      window_days: 180
      max_distinct_anonymous_ids: 1000
      scope: " project "
    - name: ip_address
      tier: probabilistic
      window_days: 3
      max_distinct_anonymous_ids: 100
      scope: source
`))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}
	if config.Identity == nil || len(config.Identity.Keys) != 2 {
		t.Fatalf("Identity = %+v, want two keys", config.Identity)
	}
	first := config.Identity.Keys[0]
	if first.Name != "user_id" || first.Tier != "deterministic" || first.WindowDays != 180 ||
		first.MaxDistinctAnonymousIDs != 1000 || first.Scope != "project" {
		t.Fatalf("first identity key = %+v, want normalized deterministic user_id", first)
	}
}

func TestParseConfigAllowsAbsentAndEmptyIdentityConfig(t *testing.T) {
	for _, data := range []string{
		`version: 1
warehouse:
  type: bigquery
  auth: production-bigquery
  project: example-project
  dataset: segmentstream
`,
		`version: 1
warehouse:
  type: bigquery
  auth: production-bigquery
  project: example-project
  dataset: segmentstream
identity:
  keys: []
`,
	} {
		config, err := ParseConfig([]byte(data))
		if err != nil {
			t.Fatalf("ParseConfig failed for %q: %v", data, err)
		}
		if config.Identity != nil {
			t.Fatalf("Identity = %+v, want nil for absent or empty identity config", config.Identity)
		}
	}
}

func TestParseConfigRejectsInvalidIdentityKeys(t *testing.T) {
	tests := []struct {
		name    string
		patch   string
		wantErr string
	}{
		{
			name: "missing name",
			patch: `    - tier: deterministic
      window_days: 180
      max_distinct_anonymous_ids: 100
      scope: project
`,
			wantErr: "missing required field identity.keys[0].name",
		},
		{
			name: "invalid tier",
			patch: `    - name: user_id
      tier: strong
      window_days: 180
      max_distinct_anonymous_ids: 100
      scope: project
`,
			wantErr: "identity.keys[0].tier must be deterministic or probabilistic",
		},
		{
			name: "non-positive window",
			patch: `    - name: user_id
      tier: deterministic
      window_days: 0
      max_distinct_anonymous_ids: 100
      scope: project
`,
			wantErr: "identity.keys[0].window_days must be a positive integer",
		},
		{
			name: "non-positive max",
			patch: `    - name: user_id
      tier: deterministic
      window_days: 180
      max_distinct_anonymous_ids: -1
      scope: project
`,
			wantErr: "identity.keys[0].max_distinct_anonymous_ids must be a positive integer",
		},
		{
			name: "invalid scope",
			patch: `    - name: user_id
      tier: deterministic
      window_days: 180
      max_distinct_anonymous_ids: 100
      scope: global
`,
			wantErr: "identity.keys[0].scope must be project or source",
		},
		{
			name: "duplicate name",
			patch: `    - name: user_id
      tier: deterministic
      window_days: 180
      max_distinct_anonymous_ids: 100
      scope: project
    - name: user_id
      tier: probabilistic
      window_days: 30
      max_distinct_anonymous_ids: 100
      scope: source
`,
			wantErr: `duplicate identity key "user_id"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseConfig([]byte(`version: 1
warehouse:
  type: bigquery
  auth: production-bigquery
  project: example-project
  dataset: segmentstream
identity:
  keys:
` + tt.patch))
			if err == nil {
				t.Fatal("expected identity validation error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
