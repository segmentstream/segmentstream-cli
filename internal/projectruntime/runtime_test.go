package projectruntime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/segmentstream/segmentstream-cli/internal/project"
)

func TestPrepareCreatesExpectedRuntimeFiles(t *testing.T) {
	root := t.TempDir()

	if err := Prepare(root, testConfig()); err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	for _, relative := range []string{
		"Dockerfile",
		"README.md",
		"docker-compose.yml",
		"dbt_project.yml",
		"profiles.yml",
		filepath.Join("dagster", "definitions.py"),
		filepath.Join("dbt", "models"),
		filepath.Join("dbt", "macros"),
	} {
		path := filepath.Join(root, RuntimeDirName, relative)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected generated path %s: %v", relative, err)
		}
	}

	profiles, err := os.ReadFile(filepath.Join(root, RuntimeDirName, "profiles.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(profiles), "project: example-project") {
		t.Fatalf("profiles.yml does not contain rendered project:\n%s", string(profiles))
	}
	if !strings.Contains(string(profiles), "dataset: segmentstream") {
		t.Fatalf("profiles.yml does not contain rendered dataset:\n%s", string(profiles))
	}
}

func TestPrepareRemovesStaleRuntimeFiles(t *testing.T) {
	root := t.TempDir()
	stale := filepath.Join(root, RuntimeDirName, "stale.txt")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Prepare(root, testConfig()); err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale file still exists or stat failed with unexpected error: %v", err)
	}
}

func TestValidateRuntimeDirRejectsUnexpectedPath(t *testing.T) {
	root := t.TempDir()
	err := validateRuntimeDir(root, filepath.Join(root, "outside"))
	if err == nil {
		t.Fatal("expected path safety error")
	}
	if !strings.Contains(err.Error(), "refusing to remove runtime directory") {
		t.Fatalf("error = %v, want path safety refusal", err)
	}
}

func testConfig() project.Config {
	return project.Config{
		Version: 1,
		Warehouse: project.Warehouse{
			Type:     "bigquery",
			Auth:     "production-bigquery",
			Project:  "example-project",
			Dataset:  "segmentstream",
			Location: "US",
		},
	}
}
