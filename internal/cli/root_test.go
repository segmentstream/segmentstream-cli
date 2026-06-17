package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"segmentstream ",
		"commit: ",
		"date: ",
		"os/arch: ",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("version output %q does not contain %q", got, want)
		}
	}
}

func TestInitCreatesProjectConfigGitignoreAndRuntime(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"init"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init command failed: %v", err)
	}

	assertFileExists(t, filepath.Join(root, "segmentstream.yml"))
	assertFileExists(t, filepath.Join(root, "README.md"))
	assertFileExists(t, filepath.Join(root, "AGENTS.md"))
	assertFileExists(t, filepath.Join(root, ".segmentstream", "docker-compose.yml"))
	assertFileExists(t, filepath.Join(root, ".segmentstream", "README.md"))
	assertFileExists(t, filepath.Join(root, ".segmentstream", "dbt_project.yml"))
	assertFileExists(t, filepath.Join(root, ".segmentstream", "dagster", "definitions.py"))

	gitignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gitignore), ".segmentstream/") {
		t.Fatalf(".gitignore = %q, want .segmentstream entry", string(gitignore))
	}
	if !strings.Contains(out.String(), "Created segmentstream.yml") {
		t.Fatalf("init output = %q, want config creation message", out.String())
	}
	if !strings.Contains(out.String(), "Created README.md") {
		t.Fatalf("init output = %q, want README creation message", out.String())
	}
	if !strings.Contains(out.String(), "Created AGENTS.md") {
		t.Fatalf("init output = %q, want agent guide creation message", out.String())
	}
	if !strings.Contains(out.String(), "Prepared SegmentStream project") {
		t.Fatalf("init output = %q, want project preparation message", out.String())
	}
}

func TestInitDoesNotOverwriteExistingConfig(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)

	config := `version: 1
warehouse:
  type: bigquery
  auth: existing-bigquery
  project: existing-project
  dataset: existing_dataset
`
	if err := os.WriteFile(filepath.Join(root, "segmentstream.yml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"init"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init command failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "segmentstream.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != config {
		t.Fatalf("segmentstream.yml was overwritten:\n%s", string(data))
	}

	env, err := os.ReadFile(filepath.Join(root, ".segmentstream", ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), `SEGMENTSTREAM_BQ_PROJECT="existing-project"`) {
		t.Fatalf(".env did not render existing config:\n%s", string(env))
	}
	if !strings.Contains(out.String(), "Using existing segmentstream.yml") {
		t.Fatalf("init output = %q, want existing config message", out.String())
	}
}

func TestInitDoesNotOverwriteExistingAgentGuide(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("custom agent guide\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"init"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init command failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "custom agent guide\n" {
		t.Fatalf("AGENTS.md was overwritten:\n%s", string(data))
	}
}

func TestInitDoesNotOverwriteExistingReadme(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("custom readme\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"init"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("init command failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "custom readme\n" {
		t.Fatalf("README.md was overwritten:\n%s", string(data))
	}
}

func TestPrepareCommandIsNotRegistered(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"prepare"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected prepare command to be unavailable")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("error = %v, want unknown command message", err)
	}
}

func TestSourceInitCreatesLocalSourceTemplate(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)
	writeValidConfig(t, root)

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"source", "init", "ga4"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("source init command failed: %v", err)
	}

	assertFileExists(t, filepath.Join(root, "sources", "ga4", "dbt_project.yml"))
	assertFileExists(t, filepath.Join(root, "sources", "ga4", "models", "exports", "events_ga4.sql"))
	assertFileExists(t, filepath.Join(root, "sources", "ga4", "models", "exports", "schema.yml"))
	assertFileExists(t, filepath.Join(root, "sources", "ga4", "models", "staging", "stg_events_ga4.sql"))
	assertFileExists(t, filepath.Join(root, "sources", "ga4", "models", "staging", "sources.yml"))

	for _, want := range []string{
		`Created source "ga4" at sources/ga4`,
		"sources:",
		"  - name: ga4",
		"    path: ./sources/ga4",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("source init output = %q, want %q", out.String(), want)
		}
	}
}

func TestSourceInitFailsWhenProjectIsMissing(t *testing.T) {
	root := t.TempDir()
	withWorkingDirectory(t, root)

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"source", "init", "ga4"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected source init to fail")
	}
	if !strings.Contains(err.Error(), "segmentstream.yml was not found") {
		t.Fatalf("error = %v, want missing config message", err)
	}
}

func TestAuthCommandIncludesBigQuery(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	cmd := NewRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"auth", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth help failed: %v", err)
	}
	if !strings.Contains(out.String(), "bigquery") {
		t.Fatalf("auth help %q does not include bigquery", out.String())
	}
}

func TestAuthBigQueryCommandRunsAuthenticator(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	authenticator := &fakeBigQueryAuthenticator{path: "/tmp/google.json"}

	cmd := newRootCommand(&out, &errOut, cliOptions{
		NewBigQueryAuthenticator: func(io.Writer, io.Writer) bigQueryAuthenticator {
			return authenticator
		},
	})
	cmd.SetArgs([]string{"auth", "bigquery"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth bigquery failed: %v", err)
	}
	if !authenticator.called {
		t.Fatal("authenticator was not called")
	}
}

type fakeBigQueryAuthenticator struct {
	called bool
	path   string
	err    error
}

func (authenticator *fakeBigQueryAuthenticator) AuthenticateBigQuery(context.Context) (string, error) {
	authenticator.called = true
	return authenticator.path, authenticator.err
}

func withWorkingDirectory(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %s: %v", path, err)
	}
}
