package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/segmentstream/segmentstream-cli/cli/internal/project"
)

func TestSavePassingAndCheckValidMarker(t *testing.T) {
	root, source := writeSourcePackage(t)

	marker, markerPath, err := SavePassing(root, source, "2026-06-16", "2026-06-23", time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("SavePassing failed: %v", err)
	}
	if marker.SchemaVersion != SchemaVersion ||
		marker.Source != "ga4" ||
		marker.Contract.Type != "events" ||
		marker.Contract.SchemaVersion != 1 ||
		marker.StartDate != "2026-06-16" ||
		marker.EndExclusiveDate != "2026-06-23" ||
		marker.Fingerprint == "" {
		t.Fatalf("marker = %+v, want populated marker", marker)
	}
	if markerPath != filepath.Join(root, "sources", "ga4", MarkerDirName, MarkerFileName) {
		t.Fatalf("marker path = %q", markerPath)
	}

	status, err := Check(root, source)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !status.Valid || status.Reason != "" {
		t.Fatalf("status = %+v, want valid marker", status)
	}
}

func TestCheckReportsMissingAndStaleMarker(t *testing.T) {
	root, source := writeSourcePackage(t)

	status, err := Check(root, source)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if status.Valid || !strings.Contains(status.Reason, "not passed verification") {
		t.Fatalf("status = %+v, want missing marker", status)
	}

	if _, _, err := SavePassing(root, source, "2026-06-16", "2026-06-23", time.Now()); err != nil {
		t.Fatalf("SavePassing failed: %v", err)
	}
	appendFile(t, filepath.Join(root, "sources", "ga4", "models", "events.sql"), "\n-- changed\n")

	status, err = Check(root, source)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if status.Valid || !strings.Contains(status.Reason, "changed since verification") {
		t.Fatalf("status = %+v, want stale marker", status)
	}
}

func TestCheckReportsContractMismatch(t *testing.T) {
	root, source := writeSourcePackage(t)
	if _, _, err := SavePassing(root, source, "2026-06-16", "2026-06-23", time.Now()); err != nil {
		t.Fatalf("SavePassing failed: %v", err)
	}

	writeFile(t, filepath.Join(root, "sources", "ga4", "contract.yml"), `type: identity_keys
schema_version: 2
`)

	status, err := Check(root, source)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if status.Valid || !strings.Contains(status.Reason, "contract changed") {
		t.Fatalf("status = %+v, want contract mismatch", status)
	}
}

func TestCheckReportsUnsupportedIdentityKeysContractVersion(t *testing.T) {
	root, source := writeLegacyIdentityKeysSourcePackage(t)

	status, err := Check(root, source)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if status.Valid ||
		!strings.Contains(status.Reason, "schema_version 1 is unsupported") ||
		!strings.Contains(status.Reason, "expected schema_version 2") ||
		!strings.Contains(status.Reason, "Migration guide") ||
		!strings.Contains(status.Reason, "observed_at") ||
		!strings.Contains(status.Reason, "sources/crm/models/identity_keys.sql") ||
		!strings.Contains(status.Reason, "segmentstream source verify crm") {
		t.Fatalf("status = %+v, want unsupported identity_keys schema version", status)
	}
}

func TestCheckReportsSourceNameMismatch(t *testing.T) {
	root, source := writeSourcePackage(t)
	if _, _, err := SavePassing(root, source, "2026-06-16", "2026-06-23", time.Now()); err != nil {
		t.Fatalf("SavePassing failed: %v", err)
	}

	status, err := Check(root, project.Source{Name: "other", Path: "./sources/ga4"})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if status.Valid || !strings.Contains(status.Reason, "different source") {
		t.Fatalf("status = %+v, want source mismatch", status)
	}
}

func TestRequireTemplateTests(t *testing.T) {
	root, _ := writeSourcePackage(t)
	sourcePath := filepath.Join(root, "sources", "ga4")

	if err := RequireTemplateTests(sourcePath); err != nil {
		t.Fatalf("RequireTemplateTests failed: %v", err)
	}
	if err := os.Remove(filepath.Join(sourcePath, "tests", "verify_events_non_empty.sql")); err != nil {
		t.Fatal(err)
	}
	err := RequireTemplateTests(sourcePath)
	if err == nil || !strings.Contains(err.Error(), "verify_events_non_empty.sql") {
		t.Fatalf("error = %v, want missing test error", err)
	}
}

func TestRequireTemplateTestsUsesIdentityKeysContractModel(t *testing.T) {
	root, _ := writeIdentityKeysSourcePackage(t)
	sourcePath := filepath.Join(root, "sources", "crm")

	if err := RequireTemplateTests(sourcePath); err != nil {
		t.Fatalf("RequireTemplateTests failed: %v", err)
	}

	writeFile(t, filepath.Join(sourcePath, "tests", "verify_identity_keys_contract.sql"), "select 1 where false\n")
	err := RequireTemplateTests(sourcePath)
	if err == nil ||
		!strings.Contains(err.Error(), "verify_identity_keys_contract.sql") ||
		!strings.Contains(err.Error(), "segmentstream_source_verify") {
		t.Fatalf("error = %v, want missing tag error for identity_keys contract test", err)
	}
}

func writeSourcePackage(t *testing.T) (string, project.Source) {
	t.Helper()
	root := t.TempDir()
	sourcePath := filepath.Join(root, "sources", "ga4")
	writeFile(t, filepath.Join(sourcePath, "contract.yml"), `type: events
schema_version: 1
`)
	writeFile(t, filepath.Join(sourcePath, "dbt_project.yml"), `name: segmentstream_source_ga4
`)
	writeFile(t, filepath.Join(sourcePath, "models", "events.sql"), "select 1 as event_id\n")
	writeFile(t, filepath.Join(sourcePath, "tests", "verify_events_contract.sql"), "{{ config(tags=['segmentstream_source_verify']) }}\nselect 1 where false\n")
	writeFile(t, filepath.Join(sourcePath, "tests", "verify_events_non_empty.sql"), "{{ config(tags=['segmentstream_source_verify']) }}\nselect 1 where false\n")
	return root, project.Source{Name: "ga4", Path: "./sources/ga4"}
}

func writeIdentityKeysSourcePackage(t *testing.T) (string, project.Source) {
	t.Helper()
	root := t.TempDir()
	sourcePath := filepath.Join(root, "sources", "crm")
	writeFile(t, filepath.Join(sourcePath, "contract.yml"), `type: identity_keys
schema_version: 2
model:
  name: identity_keys
  partition: date
`)
	writeFile(t, filepath.Join(sourcePath, "dbt_project.yml"), `name: segmentstream_source_crm
`)
	writeFile(t, filepath.Join(sourcePath, "models", "identity_keys.sql"), "select current_date() as date\n")
	writeFile(t, filepath.Join(sourcePath, "tests", "verify_identity_keys_contract.sql"), "{{ config(tags=['segmentstream_source_verify']) }}\nselect 1 where false\n")
	writeFile(t, filepath.Join(sourcePath, "tests", "verify_identity_keys_non_empty.sql"), "{{ config(tags=['segmentstream_source_verify']) }}\nselect 1 where false\n")
	return root, project.Source{Name: "crm", Path: "./sources/crm"}
}

func writeLegacyIdentityKeysSourcePackage(t *testing.T) (string, project.Source) {
	t.Helper()
	root := t.TempDir()
	sourcePath := filepath.Join(root, "sources", "crm")
	writeFile(t, filepath.Join(sourcePath, "contract.yml"), `type: identity_keys
schema_version: 1
model:
  name: identity_keys
  partition: date
`)
	writeFile(t, filepath.Join(sourcePath, "dbt_project.yml"), `name: segmentstream_source_crm
`)
	writeFile(t, filepath.Join(sourcePath, "models", "identity_keys.sql"), "select current_date() as date\n")
	writeFile(t, filepath.Join(sourcePath, "tests", "verify_identity_keys_contract.sql"), "{{ config(tags=['segmentstream_source_verify']) }}\nselect 1 where false\n")
	writeFile(t, filepath.Join(sourcePath, "tests", "verify_identity_keys_non_empty.sql"), "{{ config(tags=['segmentstream_source_verify']) }}\nselect 1 where false\n")
	return root, project.Source{Name: "crm", Path: "./sources/crm"}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendFile(t *testing.T, path, content string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString(content); err != nil {
		t.Fatal(err)
	}
}
