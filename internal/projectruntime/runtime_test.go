package projectruntime

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/segmentstream/segmentstream-cli/internal/project"
	"gopkg.in/yaml.v3"
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
		".env",
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
	if !strings.Contains(string(profiles), `project: "{{ env_var('SEGMENTSTREAM_BQ_PROJECT') }}"`) {
		t.Fatalf("profiles.yml does not contain BigQuery project env var:\n%s", string(profiles))
	}
	if strings.Contains(string(profiles), "example-project") {
		t.Fatalf("profiles.yml should be static, got rendered project:\n%s", string(profiles))
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

func TestPrepareWritesRuntimeEnvAndStaticComposeFile(t *testing.T) {
	root := t.TempDir()

	if err := Prepare(root, testConfig()); err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	compose, err := os.ReadFile(filepath.Join(root, RuntimeDirName, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(compose, &parsed); err != nil {
		t.Fatalf("docker-compose.yml is not valid YAML: %v\n%s", err, string(compose))
	}
	if !strings.Contains(string(compose), `source: "${SEGMENTSTREAM_HOST_HOME}"`) {
		t.Fatalf("docker-compose.yml does not use static host env var mount:\n%s", string(compose))
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	hostHome, err := filepath.Abs(filepath.Join(home, ".segmentstream"))
	if err != nil {
		t.Fatal(err)
	}
	env, err := os.ReadFile(filepath.Join(root, RuntimeDirName, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`SEGMENTSTREAM_HOST_HOME=` + strconv.Quote(filepath.ToSlash(hostHome)),
		`SEGMENTSTREAM_BQ_PROJECT="example-project"`,
		`SEGMENTSTREAM_BQ_DATASET="segmentstream"`,
		`SEGMENTSTREAM_BQ_LOCATION="US"`,
	} {
		if !strings.Contains(string(env), want) {
			t.Fatalf(".env does not contain %q:\n%s", want, string(env))
		}
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
